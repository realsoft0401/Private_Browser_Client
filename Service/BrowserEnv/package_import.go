package BrowserEnv

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

const (
	// maxImportPackageBytes 限制上传 gzip 流大小。
	//
	// 环境包包含 browser-data/profile，商业场景可能较大；这里保留 20GiB 上限，
	// 但仍要求调用方通过受控内网传输，不能把 Edge API 暴露到公网。
	maxImportPackageBytes = 20 << 30
	// maxImportExtractedBytes 限制解压后总文件体积，避免 gzip bomb 打满 staging 磁盘。
	maxImportExtractedBytes = 25 << 30
	// maxImportPackageFileCount 限制 tar entry 数量，避免大量小文件拖垮导入和 checksum 计算。
	maxImportPackageFileCount = 200000
)

type importExtractStats struct {
	files int
	bytes int64
}

// ImportBrowserEnvPackage 导入标准 tar.gz 环境包。
//
// 设计来源：
// - 用户把 import 放在 backup 之后，要求导入基于边缘服务真实生成的包格式，而不是另造一套 JSON 协议；
// - envId 属于账号环境身份，第一版导入默认保留原 envId；如果本机已存在同名环境包，直接拒绝；
// - envSequence、CDP/VNC 端口和容器运行态属于本机资源，导入时必须重新分配并重置。
//
// 职责边界：
// - 只接收 tar.gz 包，校验单根目录、profile、checksums 和原子材料；
// - 只恢复环境包目录和 SQLite 索引，不启动 Docker，不拉镜像，不创建容器；
// - timezone 状态导入后标记 pending，下一次 run 会在浏览器容器内重新探测真实出口。
func (s *Service) ImportBrowserEnvPackage(file multipart.File, header *multipart.FileHeader) (*model.ImportBrowserEnvPackageResponse, error) {
	if file == nil || header == nil {
		return nil, invalidError("导入文件不能为空")
	}
	if header.Size <= 0 {
		return nil, invalidError("导入文件为空")
	}
	if header.Size > maxImportPackageBytes {
		return nil, invalidError("导入文件超过大小限制")
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	stagingRoot, err := os.MkdirTemp("", "private-browser-import-*")
	if err != nil {
		return nil, internalError(fmt.Sprintf("创建导入 staging 目录失败: %v", err))
	}
	defer os.RemoveAll(stagingRoot)

	if err = extractImportTarGz(file, stagingRoot); err != nil {
		return nil, invalidError(err.Error())
	}
	stagingEnvPath, err := findImportPackageRoot(stagingRoot)
	if err != nil {
		return nil, invalidError(err.Error())
	}

	imported, err := prepareImportedPackage(stagingEnvPath)
	if err != nil {
		return nil, err
	}
	targetEnvPath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(imported.Index.EnvPath))
	if err = ensureEnvPathAvailable(targetEnvPath); err != nil {
		return nil, err
	}

	handler := browserEnvDao.NewCreateModelHandler()
	if _, err = browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(context.Background(), imported.Index.EnvID); err == nil {
		return nil, conflictError("envId 已存在，不能重复导入")
	} else if !errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
		return nil, internalError(err.Error())
	}

	if err = copyDirectory(stagingEnvPath, targetEnvPath); err != nil {
		return nil, internalError(fmt.Sprintf("复制导入环境包失败: %v", err))
	}
	created := false
	defer cleanupPartialEnvPackage(targetEnvPath, &created)
	if err = handler.CreateBrowserEnvIndex(context.Background(), imported.Index); err != nil {
		if errors.Is(err, browserEnvDao.ErrDuplicateBrowserEnv) {
			return nil, conflictError("envId 已存在，不能重复导入")
		}
		return nil, internalError(err.Error())
	}

	created = true
	return &model.ImportBrowserEnvPackageResponse{
		EnvID:       imported.Profile.EnvID,
		UserID:      imported.Profile.UserID,
		RPAType:     imported.Profile.RPAType,
		EnvSequence: imported.Profile.EnvSequence,
		Ports:       imported.Profile.Ports,
		EnvPath:     imported.Index.EnvPath,
		Status:      model.BrowserEnvStatusCreated,
		ImportedAt:  imported.Now,
	}, nil
}

type preparedImportedPackage struct {
	Profile   model.ProfileFile
	Container model.ContainerFile
	Index     *model.BrowserEnvIndex
	Now       int64
}

// prepareImportedPackage 校验并改写 staging 包中的本机运行资源。
//
// 这里先在 staging 内完成端口、container、profile 和 pending runtime 写回，
// 只有全部成功后才复制到正式目录，避免导入目录出现半成品。
func prepareImportedPackage(stagingEnvPath string) (*preparedImportedPackage, error) {
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

	binding := atomic.Binding
	container := atomic.Container
	if !atomic.HasContainerFile {
		container = model.ContainerFile{
			EnvID: profile.EnvID,
			Image: profile.Runtime.Image,
		}
	}
	if profile.EnvID != container.EnvID {
		return nil, invalidError("profile/container 的 envId 不一致")
	}
	if profile.UserID != binding.Identity.UserID || profile.RPAType != binding.Identity.RPAType {
		return nil, invalidError("profile/binding 的用户或类型不一致")
	}

	now := time.Now().Unix()
	envSequence, ports, err := nextAvailableEnvSequenceAndPorts()
	if err != nil {
		return nil, internalError(err.Error())
	}
	relativeEnvPath := filepath.ToSlash(filepath.Join("data", "browser-envs", "users", profile.UserID, profile.RPAType, profile.EnvID))

	profile.EnvSequence = envSequence
	profile.Ports = ports
	profile.LastRuntime = model.PackageLastRuntime{}
	profile.Package = model.ProfilePackageMetadata{}
	profile.Metadata.UpdatedAt = now
	container.ContainerName = "bv-" + strings.ReplaceAll(profile.EnvID, "_", "-")
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

	if err = writePackageJSON(stagingEnvPath, profile.Paths.Profile, profile); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(stagingEnvPath, profile.Paths.Binding, binding); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(stagingEnvPath, profile.Paths.Container, container); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writeTimezoneProbePending(stagingEnvPath, profile.Paths.ProxyRuntime); err != nil {
		return nil, internalError(err.Error())
	}

	containerName := container.ContainerName
	if err := ensureNoDockerConflictForAdmission(profile.EnvID, containerName); err != nil {
		return nil, err
	}
	return &preparedImportedPackage{
		Profile:   profile,
		Container: container,
		Now:       now,
		Index: &model.BrowserEnvIndex{
			EnvID:               profile.EnvID,
			UserID:              profile.UserID,
			RPAType:             profile.RPAType,
			Name:                profile.Name,
			EnvSequence:         envSequence,
			CDPPort:             ports.CDP,
			VNCPort:             ports.VNC,
			EnvPath:             relativeEnvPath,
			Status:              model.BrowserEnvStatusCreated,
			ContainerName:       &containerName,
			ContainerStatus:     model.BrowserEnvContainerStatusUnknown,
			MonitorStatus:       model.BrowserEnvMonitorStatusUnknown,
			FingerprintRestored: binding.Fingerprint.Restored,
			HasBrowserData:      true,
			CreatedAt:           now,
			UpdatedAt:           now,
		},
	}, nil
}

func validateImportedProfile(envPath string, profile model.ProfileFile) error {
	if profile.SchemaVersion != model.SchemaVersion {
		return invalidError("不支持的环境包 profile.schemaVersion")
	}
	if profile.Package.Version == nil || *profile.Package.Version != browserEnvPackageVersion {
		return invalidError("不支持的环境包 profile.package.version")
	}
	if strings.TrimSpace(profile.EnvID) == "" || strings.TrimSpace(profile.UserID) == "" {
		return invalidError("profile.envId/userId 不能为空")
	}
	if _, ok := model.SupportedRPATypes[profile.RPAType]; !ok {
		return invalidError("profile.rpaType 不支持")
	}
	if filepath.Base(envPath) != profile.EnvID {
		return invalidError("tar 根目录必须等于 profile.envId")
	}
	index := &model.BrowserEnvIndex{
		EnvID:   profile.EnvID,
		UserID:  profile.UserID,
		RPAType: profile.RPAType,
	}
	if _, err := validateBackupSourcePackage(index, envPath); err != nil {
		return invalidError(err.Error())
	}
	if len(profile.Package.Checksums) == 0 {
		return invalidError("profile.package.checksums 不能为空")
	}
	return nil
}

func validatePackageChecksums(envPath string, expected map[string]string) error {
	if len(expected) == 0 {
		return invalidError("profile.package.checksums 不能为空")
	}
	actual, err := buildPackageChecksums(envPath)
	if err != nil {
		return invalidError(fmt.Sprintf("计算导入包 checksum 失败: %v", err))
	}
	for path, want := range expected {
		got, ok := actual[path]
		if !ok {
			return invalidError("导入包文件缺失: " + path)
		}
		if got != want {
			return invalidError("导入包 checksum 不匹配: " + path)
		}
	}
	for path := range actual {
		if _, ok := expected[path]; !ok {
			return invalidError("导入包存在未登记 checksum 的文件: " + path)
		}
	}
	return nil
}

// extractImportTarGz 解压上传环境包到 staging 目录。
//
// 商业导入入口不能对 tar 做宽松兼容：
// - 只允许普通文件和目录，symlink/hardlink/device/fifo 等特殊 entry 直接拒绝；
// - 同时限制解压后的文件数量和总字节数，避免 gzip bomb 或大量小文件打满磁盘；
// - 每个 entry 解析后再次校验目标路径仍在 staging 内，避免路径穿越。
func extractImportTarGz(file multipart.File, targetDir string) error {
	limited := io.LimitReader(file, maxImportPackageBytes+1)
	gzipReader, err := gzip.NewReader(limited)
	if err != nil {
		return fmt.Errorf("导入文件不是有效 tar.gz: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	stats := &importExtractStats{}
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("读取 tar 失败: %w", err)
		}
		if err = extractImportTarEntry(tarReader, header, targetDir, stats); err != nil {
			return err
		}
	}
}

func extractImportTarEntry(reader *tar.Reader, header *tar.Header, targetDir string, stats *importExtractStats) error {
	if header == nil {
		return nil
	}
	if stats == nil {
		stats = &importExtractStats{}
	}
	name := strings.TrimSpace(header.Name)
	if name == "" {
		return nil
	}
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || filepath.IsAbs(cleanName) {
		return fmt.Errorf("tar 包含非法路径: %s", name)
	}
	targetPath := filepath.Join(targetDir, cleanName)
	if err := ensureExtractTargetInDir(targetDir, targetPath); err != nil {
		return err
	}
	stats.files++
	if stats.files > maxImportPackageFileCount {
		return fmt.Errorf("导入包文件数量超过限制: %d", maxImportPackageFileCount)
	}
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(targetPath, 0755)
	case tar.TypeReg, tar.TypeRegA:
		if header.Size < 0 {
			return fmt.Errorf("tar 文件大小非法: %s", name)
		}
		if stats.bytes+header.Size > maxImportExtractedBytes {
			return fmt.Errorf("导入包解压后总大小超过限制: %d bytes", maxImportExtractedBytes)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode).Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		written, err := io.Copy(out, reader)
		if err != nil {
			return err
		}
		if written != header.Size {
			return fmt.Errorf("tar 文件大小与 header 不一致: %s", name)
		}
		stats.bytes += written
		return nil
	default:
		return fmt.Errorf("导入包包含不支持的 tar entry 类型: %s type=%d", name, header.Typeflag)
	}
}

func ensureExtractTargetInDir(root string, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("解析导入根目录失败: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("解析导入目标路径失败: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return fmt.Errorf("校验导入目标路径失败: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("tar 包含越界路径: %s", target)
	}
	return nil
}

func findImportPackageRoot(stagingRoot string) (string, error) {
	entries, err := os.ReadDir(stagingRoot)
	if err != nil {
		return "", fmt.Errorf("读取导入 staging 失败: %w", err)
	}
	roots := make([]string, 0, 1)
	for _, entry := range entries {
		if entry.IsDir() {
			roots = append(roots, filepath.Join(stagingRoot, entry.Name()))
		}
	}
	if len(roots) != 1 {
		return "", fmt.Errorf("tar 包必须只有一个环境包根目录")
	}
	return roots[0], nil
}
