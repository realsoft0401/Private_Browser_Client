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

const maxImportPackageBytes = 20 << 30

// ImportBrowserEnvPackage 导入标准 tar.gz 环境包。
//
// 设计来源：
// - 用户把 import 放在 backup 之后，要求导入基于边缘服务真实生成的包格式，而不是另造一套 JSON 协议；
// - envId 属于账号环境身份，第一版导入默认保留原 envId；如果本机已存在同名环境包，直接拒绝；
// - envSequence、CDP/VNC 端口和容器运行态属于本机资源，导入时必须重新分配并重置。
//
// 职责边界：
// - 只接收 tar.gz 包，校验单根目录、manifest、checksums 和标准文件；
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
		EnvID:       imported.Manifest.EnvID,
		UserID:      imported.Manifest.UserID,
		RPAType:     imported.Manifest.RPAType,
		EnvSequence: imported.Manifest.EnvSequence,
		Ports:       imported.Profile.Ports,
		EnvPath:     imported.Index.EnvPath,
		Status:      model.BrowserEnvStatusCreated,
		ImportedAt:  imported.Now,
	}, nil
}

type preparedImportedPackage struct {
	Manifest model.ManifestFile
	Profile  model.ProfileFile
	Index    *model.BrowserEnvIndex
	Now      int64
}

// prepareImportedPackage 校验并改写 staging 包中的本机运行资源。
//
// 这里先在 staging 内完成端口、container、manifest 和 pending runtime 写回，
// 只有全部成功后才复制到正式目录，避免导入目录出现半成品。
func prepareImportedPackage(stagingEnvPath string) (*preparedImportedPackage, error) {
	var manifest model.ManifestFile
	if err := readJSONFile(filepath.Join(stagingEnvPath, "manifest.json"), &manifest); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 manifest 失败: %v", err))
	}
	if err := validateImportedManifest(stagingEnvPath, manifest); err != nil {
		return nil, err
	}
	if err := validatePackageChecksums(stagingEnvPath, manifest.Checksums); err != nil {
		return nil, err
	}

	var profile model.ProfileFile
	var binding model.BindingFile
	var container model.ContainerFile
	if err := readJSONFile(filepath.Join(stagingEnvPath, filepath.FromSlash(manifest.Paths.Profile)), &profile); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 profile 失败: %v", err))
	}
	if err := readJSONFile(filepath.Join(stagingEnvPath, filepath.FromSlash(manifest.Paths.Binding)), &binding); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 binding 失败: %v", err))
	}
	if err := readJSONFile(filepath.Join(stagingEnvPath, filepath.FromSlash(manifest.Paths.Container)), &container); err != nil {
		return nil, invalidError(fmt.Sprintf("读取 container 失败: %v", err))
	}
	if manifest.EnvID != profile.EnvID || manifest.EnvID != container.EnvID {
		return nil, invalidError("manifest/profile/container 的 envId 不一致")
	}
	if manifest.UserID != binding.Identity.UserID || manifest.RPAType != profile.RPAType {
		return nil, invalidError("manifest/profile/binding 的用户或类型不一致")
	}

	now := time.Now().Unix()
	envSequence, err := nextEnvSequence()
	if err != nil {
		return nil, internalError(err.Error())
	}
	ports := buildPorts(envSequence)
	relativeEnvPath := filepath.ToSlash(filepath.Join("data", "browser-envs", "users", manifest.UserID, manifest.RPAType, manifest.EnvID))

	manifest.EnvSequence = envSequence
	manifest.LastRuntime = model.ManifestLastRuntime{}
	manifest.PackageVersion = nil
	manifest.ExportedAt = nil
	manifest.ExportSource = nil
	manifest.ExportAction = ""
	manifest.Checksums = nil
	manifest.UpdatedAt = now
	profile.EnvSequence = envSequence
	profile.Ports = ports
	profile.Metadata.UpdatedAt = now
	container.ContainerName = "bv-" + strings.ReplaceAll(manifest.EnvID, "_", "-")
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
		"bv.envId":         manifest.EnvID,
		"bv.userId":        manifest.UserID,
		"bv.rpaType":       manifest.RPAType,
		"bv.schemaVersion": fmt.Sprintf("%d", model.SchemaVersion),
	}
	binding.RuntimeProtection.TimezoneStatus = "pending"
	binding.UpdatedAt = now

	if err = writePackageJSON(stagingEnvPath, manifest.Paths.Profile, profile); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(stagingEnvPath, manifest.Paths.Binding, binding); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writePackageJSON(stagingEnvPath, manifest.Paths.Container, container); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writeTimezoneProbePending(stagingEnvPath, manifest.Paths.ProxyRuntime); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writeJSONFile(filepath.Join(stagingEnvPath, "manifest.json"), manifest); err != nil {
		return nil, internalError(err.Error())
	}

	containerName := container.ContainerName
	return &preparedImportedPackage{
		Manifest: manifest,
		Profile:  profile,
		Now:      now,
		Index: &model.BrowserEnvIndex{
			EnvID:               manifest.EnvID,
			UserID:              manifest.UserID,
			RPAType:             manifest.RPAType,
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

func validateImportedManifest(envPath string, manifest model.ManifestFile) error {
	if manifest.SchemaVersion != model.SchemaVersion {
		return invalidError("不支持的环境包 schemaVersion")
	}
	if manifest.PackageVersion == nil || *manifest.PackageVersion != browserEnvPackageVersion {
		return invalidError("不支持的环境包 packageVersion")
	}
	if strings.TrimSpace(manifest.EnvID) == "" || strings.TrimSpace(manifest.UserID) == "" {
		return invalidError("manifest.envId/userId 不能为空")
	}
	if _, ok := model.SupportedRPATypes[manifest.RPAType]; !ok {
		return invalidError("manifest.rpaType 不支持")
	}
	if filepath.Base(envPath) != manifest.EnvID {
		return invalidError("tar 根目录必须等于 manifest.envId")
	}
	index := &model.BrowserEnvIndex{
		EnvID:   manifest.EnvID,
		UserID:  manifest.UserID,
		RPAType: manifest.RPAType,
	}
	if _, err := validateBackupSourcePackage(index, envPath); err != nil {
		return invalidError(err.Error())
	}
	if len(manifest.Checksums) == 0 {
		return invalidError("manifest.checksums 不能为空")
	}
	return nil
}

func validatePackageChecksums(envPath string, expected map[string]string) error {
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
	return nil
}

func extractImportTarGz(file multipart.File, targetDir string) error {
	limited := io.LimitReader(file, maxImportPackageBytes+1)
	gzipReader, err := gzip.NewReader(limited)
	if err != nil {
		return fmt.Errorf("导入文件不是有效 tar.gz: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("读取 tar 失败: %w", err)
		}
		if err = extractImportTarEntry(tarReader, header, targetDir); err != nil {
			return err
		}
	}
}

func extractImportTarEntry(reader *tar.Reader, header *tar.Header, targetDir string) error {
	if header == nil {
		return nil
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
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(targetPath, 0755)
	case tar.TypeReg, tar.TypeRegA:
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode).Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, reader)
		return err
	default:
		return nil
	}
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
