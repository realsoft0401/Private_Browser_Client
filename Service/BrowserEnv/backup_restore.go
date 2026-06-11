package BrowserEnv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

// BackupBrowserEnv 把可运行环境封存为本机备份资产。
//
// 设计来源：
// - 用户重新定义“备份 + 下载”：备份是状态变化动作，下载只是取走已有备份包；
// - RPA 每次执行后应把当前最新 browser-data/profile 打成包，然后删除容器和源环境目录；
// - SQLite 索引不能删除，而要记录 backupPath/checksum/size，让前端还能展示、恢复和下载。
//
// 职责边界：
// - 只处理本机已有环境包，不接收外部上传文件；
// - 拒绝 running 状态，不自动 stop，避免打包半写入的登录态；
// - 备份包固定放在 data/browser-envs/users/{userId}/{rpaType}/ 下，不允许外部传路径；
// - 不删除浏览器镜像，不删除 SQLite 索引。
func (s *Service) BackupBrowserEnv(envID string) (*model.BackupBrowserEnvResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	handler := browserEnvDao.NewRuntimeModelHandler()
	index, err := handler.GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	if index.Status == model.BrowserEnvStatusBackedUp || index.Status == model.BrowserEnvStatusArchived {
		return nil, conflictError("环境包已经是备份状态，请先恢复后再重新备份")
	}
	if index.Status == model.BrowserEnvStatusDeleted {
		return nil, conflictError("环境包已删除，不能备份")
	}
	if index.Status == model.BrowserEnvStatusError {
		return nil, conflictError("环境包处于 error，不能备份可能不完整或伪造的资产；请先修复并调用 revalidate")
	}
	if index.Status == model.BrowserEnvStatusRunning || index.ContainerStatus == model.BrowserEnvStatusRunning {
		return nil, conflictError("环境包正在运行，请先停止后再备份")
	}
	if err = ensureDockerNotRunningForPackage(index); err != nil {
		return nil, err
	}

	sourceEnvPath, err := resolveManagedEnvPath(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	profile, err := validateBackupSourcePackage(index, sourceEnvPath)
	if err != nil {
		return nil, internalError(err.Error())
	}

	backupAbs, backupRel, err := managedBackupArchivePath(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	if err = os.MkdirAll(filepath.Dir(backupAbs), 0755); err != nil {
		return nil, internalError(fmt.Sprintf("创建备份目录失败: %v", err))
	}

	result, err := buildPackageArchive(index, sourceEnvPath, profile, "backup", buildBackupArchiveFileName)
	if err != nil {
		return nil, err
	}
	defer result.Cleanup()

	tempBackup := backupAbs + ".tmp"
	_ = os.Remove(tempBackup)
	if err = copyFile(result.FilePath, tempBackup, 0644); err != nil {
		_ = os.Remove(tempBackup)
		return nil, internalError(fmt.Sprintf("写入备份包失败: %v", err))
	}
	if err = os.Rename(tempBackup, backupAbs); err != nil {
		_ = os.Remove(tempBackup)
		return nil, internalError(fmt.Sprintf("替换备份包失败: %v", err))
	}

	backupAt := time.Now().Unix()
	backupChecksum, err := fileSHA256(backupAbs)
	if err != nil {
		return nil, internalError(fmt.Sprintf("计算备份包 checksum 失败: %v", err))
	}
	stat, err := os.Stat(backupAbs)
	if err != nil {
		return nil, internalError(fmt.Sprintf("读取备份包失败: %v", err))
	}
	backupSize := stat.Size()
	backupVersion := browserEnvPackageVersion

	if err = removeStoppedContainerForDelete(index); err != nil {
		return nil, err
	}
	if err = os.RemoveAll(sourceEnvPath); err != nil {
		return nil, internalError(fmt.Sprintf("删除源环境包目录失败: %v", err))
	}

	if err = handler.UpdateBrowserEnvBackupState(context.Background(), &model.BrowserEnvBackupStateUpdate{
		EnvID:           index.EnvID,
		Status:          model.BrowserEnvStatusBackedUp,
		ContainerID:     nil,
		ContainerStatus: model.BrowserEnvContainerStatusUnknown,
		MonitorStatus:   model.BrowserEnvMonitorStatusUnknown,
		LastError:       nil,
		HasBrowserData:  false,
		BackupPath:      &backupRel,
		BackupChecksum:  &backupChecksum,
		BackupSize:      &backupSize,
		BackupAt:        &backupAt,
		BackupVersion:   &backupVersion,
		LastRestoredAt:  index.LastRestoredAt,
		UpdatedAt:       backupAt,
	}); err != nil {
		return nil, internalError(err.Error())
	}

	return &model.BackupBrowserEnvResponse{
		EnvID:          index.EnvID,
		UserID:         index.UserID,
		RPAType:        index.RPAType,
		Status:         model.BrowserEnvStatusBackedUp,
		BackupPath:     backupRel,
		BackupChecksum: backupChecksum,
		BackupSize:     backupSize,
		BackupAt:       backupAt,
		Message:        "环境包已备份，容器和源环境目录已删除，SQLite 索引保留为备份资产",
	}, nil
}

// RestoreBrowserEnv 从本机备份包恢复环境目录。
//
// 设计来源：
// - 用户希望 RPA 执行前从备份包恢复，执行后再备份释放资源；
// - restore 依赖 SQLite 中保存的 backupPath，而不是让前端重新上传同一份文件；
// - 恢复后只得到 created 状态，不自动 run，避免把恢复和执行流程混在一起。
func (s *Service) RestoreBrowserEnv(envID string) (*model.RestoreBrowserEnvResponse, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	handler := browserEnvDao.NewRuntimeModelHandler()
	index, err := handler.GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	if index.Status != model.BrowserEnvStatusBackedUp && index.Status != model.BrowserEnvStatusArchived {
		return nil, conflictError("环境包不是备份状态，不能恢复")
	}

	backupAbs, err := resolveManagedBackupPath(index)
	if err != nil {
		return nil, err
	}
	if err = verifyBackupArchiveFile(index, backupAbs); err != nil {
		return nil, err
	}

	targetEnvPath, err := resolveManagedEnvPath(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	if err = ensureEnvPathAvailable(targetEnvPath); err != nil {
		return nil, err
	}

	stagingRoot, err := os.MkdirTemp("", "private-browser-restore-*")
	if err != nil {
		return nil, internalError(fmt.Sprintf("创建恢复 staging 目录失败: %v", err))
	}
	defer os.RemoveAll(stagingRoot)

	file, err := os.Open(backupAbs)
	if err != nil {
		return nil, internalError(fmt.Sprintf("打开备份包失败: %v", err))
	}
	defer file.Close()
	if err = extractImportTarGz(file, stagingRoot); err != nil {
		return nil, invalidError(err.Error())
	}
	stagingEnvPath, err := findImportPackageRoot(stagingRoot)
	if err != nil {
		return nil, invalidError(err.Error())
	}
	envSequence, ports, err := restoreRuntimePorts(index)
	if err != nil {
		return nil, internalError(err.Error())
	}
	restored, err := prepareRestoredPackage(stagingEnvPath, index, envSequence, ports)
	if err != nil {
		return nil, err
	}
	if err = ensureNoDockerConflictForAdmission(index.EnvID, restored.Container.ContainerName); err != nil {
		return nil, err
	}

	if err = copyDirectory(stagingEnvPath, targetEnvPath); err != nil {
		return nil, internalError(fmt.Sprintf("复制恢复环境包失败: %v", err))
	}
	created := false
	defer cleanupPartialEnvPackage(targetEnvPath, &created)

	restoredAt := restored.Now
	deletePendingPath, err := moveBackupArchiveToDeletePending(backupAbs, restoredAt)
	if err != nil {
		return nil, internalError(err.Error())
	}
	backupMoved := true
	defer func() {
		if !created && backupMoved {
			_ = os.Rename(deletePendingPath, backupAbs)
		}
	}()
	if err = handler.UpdateBrowserEnvBackupState(context.Background(), &model.BrowserEnvBackupStateUpdate{
		EnvID:           index.EnvID,
		Status:          model.BrowserEnvStatusCreated,
		EnvSequence:     &envSequence,
		CDPPort:         &ports.CDP,
		VNCPort:         &ports.VNC,
		ContainerID:     nil,
		ContainerStatus: model.BrowserEnvContainerStatusUnknown,
		MonitorStatus:   model.BrowserEnvMonitorStatusUnknown,
		LastError:       nil,
		HasBrowserData:  true,
		BackupPath:      nil,
		BackupChecksum:  nil,
		BackupSize:      nil,
		BackupAt:        nil,
		BackupVersion:   nil,
		LastRestoredAt:  &restoredAt,
		UpdatedAt:       restoredAt,
	}); err != nil {
		return nil, internalError(err.Error())
	}
	created = true
	if err = os.Remove(deletePendingPath); err != nil && !os.IsNotExist(err) {
		return nil, internalError(fmt.Sprintf("恢复已完成，但删除备份临时文件失败: %v", err))
	}
	backupMoved = false
	return &model.RestoreBrowserEnvResponse{
		EnvID:      index.EnvID,
		UserID:     index.UserID,
		RPAType:    index.RPAType,
		Status:     model.BrowserEnvStatusCreated,
		Ports:      restored.Profile.Ports,
		EnvPath:    index.EnvPath,
		RestoredAt: restoredAt,
		Message:    "环境包已从备份恢复为可运行目录，下一步可调用 run",
	}, nil
}

type restoredPackage struct {
	Profile   model.ProfileFile
	Container model.ContainerFile
	Now       int64
}

// prepareRestoredPackage 校验备份包并改写成本机可运行状态。
//
// restore 和 import 的区别是：restore 面向本机已有资产索引，必须沿用当前 envSequence/CDP/VNC，
// 不能像外部导入那样新建索引或重新分配 envId。
func prepareRestoredPackage(stagingEnvPath string, index *model.BrowserEnvIndex, envSequence int, ports model.BrowserEnvPorts) (*restoredPackage, error) {
	atomic, err := loadAndValidateAtomicPackage(stagingEnvPath)
	if err != nil {
		return nil, err
	}
	profile := atomic.Profile
	if err := validateImportedProfile(stagingEnvPath, profile); err != nil {
		return nil, err
	}
	if err := validatePackageChecksums(stagingEnvPath, profile.Package.Checksums); err != nil {
		return nil, err
	}
	if profile.EnvID != index.EnvID || profile.UserID != index.UserID || profile.RPAType != index.RPAType {
		return nil, invalidError("备份包与 SQLite 环境资产索引不一致")
	}

	binding := atomic.Binding
	container := atomic.Container
	if !atomic.HasContainerFile {
		container = model.ContainerFile{
			EnvID: profile.EnvID,
			Image: profile.Runtime.Image,
		}
	}

	now := time.Now().Unix()
	containerName := edgeBrowserContainerName(index.EnvID)
	if index.ContainerName != nil && strings.TrimSpace(*index.ContainerName) != "" {
		containerName = strings.TrimSpace(*index.ContainerName)
	}

	profile.EnvSequence = envSequence
	profile.Ports = ports
	profile.LastRuntime = model.PackageLastRuntime{}
	profile.Package = model.ProfilePackageMetadata{}
	profile.Metadata.UpdatedAt = now
	container.ContainerName = containerName
	container.ContainerID = nil
	container.Status = model.BrowserEnvStatusCreated
	container.Ports = ports
	container.Docker.APIURL = Settings.Conf.DockerConfig.APIURL
	container.Docker.DeviceArch = nil
	container.StartedAt = nil
	container.StoppedAt = nil
	container.UpdatedAt = now
	container.Labels = map[string]string{
		"bv.project":       "private-browser-client",
		"bv.role":          "browser-env",
		"bv.envId":         profile.EnvID,
		"bv.userId":        profile.UserID,
		"bv.rpaType":       profile.RPAType,
		"bv.schemaVersion": fmt.Sprintf("%d", model.SchemaVersion),
	}
	binding.RuntimeProtection.TimezoneStatus = "pending"
	binding.UpdatedAt = now

	if err := writePackageJSON(stagingEnvPath, profile.Paths.Profile, profile); err != nil {
		return nil, internalError(err.Error())
	}
	if err := writePackageJSON(stagingEnvPath, profile.Paths.Binding, binding); err != nil {
		return nil, internalError(err.Error())
	}
	if err := writePackageJSON(stagingEnvPath, profile.Paths.Container, container); err != nil {
		return nil, internalError(err.Error())
	}
	if err := writeTimezoneProbePending(stagingEnvPath, profile.Paths.ProxyRuntime); err != nil {
		return nil, internalError(err.Error())
	}

	return &restoredPackage{
		Profile:   profile,
		Container: container,
		Now:       now,
	}, nil
}

// moveBackupArchiveToDeletePending 先把备份 tar 改名为待删除文件，再更新 SQLite。
//
// 设计来源：
// - 用户确认 restore 成功后应删除备份包，避免恢复和紧急导入的语义分裂；
// - 但 SQLite 更新失败时不能丢失唯一备份资产，所以这里先 rename，DB 失败时由 defer 尝试改回；
// - rename 在同目录内是原子动作，比直接删除更容易处理失败回滚。
func moveBackupArchiveToDeletePending(backupAbs string, restoredAt int64) (string, error) {
	pendingPath := fmt.Sprintf("%s.restored-%d.delete-pending", backupAbs, restoredAt)
	_ = os.Remove(pendingPath)
	if err := os.Rename(backupAbs, pendingPath); err != nil {
		return "", fmt.Errorf("移动备份包到待删除状态失败: %w", err)
	}
	return pendingPath, nil
}

// managedBackupArchivePath 生成本机备份包固定路径。
//
// 用户要求备份文件放在当前环境分组目录 data/browser-envs/users/{userId}/{rpaType}/ 下；
// 文件名固定为 {envId}-backup.tar.gz，表示“当前最新备份”，下一次备份会原子替换。
func managedBackupArchivePath(index *model.BrowserEnvIndex) (string, string, error) {
	if index == nil {
		return "", "", fmt.Errorf("环境包索引不能为空")
	}
	fileName := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(index.EnvID) + "-backup.tar.gz"
	relativePath := filepath.ToSlash(filepath.Join("data", "browser-envs", "users", index.UserID, index.RPAType, fileName))
	absolutePath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(relativePath))
	if err := ensureBackupPathInUserRPA(index, absolutePath); err != nil {
		return "", "", err
	}
	return absolutePath, relativePath, nil
}

// resolveManagedBackupPath 从 SQLite 索引解析备份包路径。
//
// 路径必须仍落在 data/browser-envs/users/{userId}/{rpaType}/ 下，防止被手工改库后读取任意文件。
func resolveManagedBackupPath(index *model.BrowserEnvIndex) (string, error) {
	if index == nil {
		return "", internalError("环境包索引不能为空")
	}
	if index.BackupPath == nil || strings.TrimSpace(*index.BackupPath) == "" {
		return "", conflictError("环境包没有可恢复的备份包")
	}
	backupPath := strings.TrimSpace(*index.BackupPath)
	if filepath.IsAbs(backupPath) {
		return "", invalidError("backupPath 必须是项目内相对路径")
	}
	absolutePath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(backupPath))
	if err := ensureBackupPathInUserRPA(index, absolutePath); err != nil {
		return "", internalError(err.Error())
	}
	return absolutePath, nil
}

func ensureBackupPathInUserRPA(index *model.BrowserEnvIndex, absolutePath string) error {
	baseAbs, err := filepath.Abs(filepath.Join(Settings.Conf.ProjectRoot, "data", "browser-envs", "users", index.UserID, index.RPAType))
	if err != nil {
		return fmt.Errorf("解析备份根目录失败: %w", err)
	}
	targetAbs, err := filepath.Abs(absolutePath)
	if err != nil {
		return fmt.Errorf("解析备份包路径失败: %w", err)
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return fmt.Errorf("校验备份包路径失败: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("backupPath 不在当前用户和 rpaType 的受控目录内")
	}
	if !strings.HasSuffix(filepath.Base(targetAbs), ".tar.gz") {
		return fmt.Errorf("backupPath 必须指向 tar.gz 文件")
	}
	return nil
}

// verifyBackupArchiveFile 在恢复前确认备份包仍然存在且 checksum 匹配。
//
// 备份包是恢复登录态和指纹事实的唯一来源，不能只依赖文件名存在；
// 如果 checksum 不一致，宁可拒绝恢复，也不能把被替换或损坏的包恢复成可运行环境。
func verifyBackupArchiveFile(index *model.BrowserEnvIndex, backupAbs string) error {
	stat, err := os.Stat(backupAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return notFoundError("备份包不存在")
		}
		return internalError(fmt.Sprintf("读取备份包失败: %v", err))
	}
	if stat.IsDir() || stat.Size() <= 0 {
		return invalidError("备份包不是有效文件")
	}
	if index.BackupChecksum == nil || strings.TrimSpace(*index.BackupChecksum) == "" {
		return nil
	}
	actual, err := fileSHA256(backupAbs)
	if err != nil {
		return internalError(fmt.Sprintf("计算备份包 checksum 失败: %v", err))
	}
	if actual != strings.TrimSpace(*index.BackupChecksum) {
		return conflictError("备份包 checksum 不匹配，拒绝恢复")
	}
	return nil
}
