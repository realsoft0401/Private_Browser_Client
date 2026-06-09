package BrowserEnv

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

const browserEnvPackageVersion = 1

// PackageArchiveResult 是环境包 tar.gz 生成后的内部结果。
//
// 当前不再暴露旧的临时打包下载接口；这个结果只作为 Service 内部打包 helper 的返回值。
// 调用方必须在归档复制或发送完成后调用 Cleanup，避免临时 tar.gz 和 staging 目录长期留在本机。
type PackageArchiveResult struct {
	FilePath string
	FileName string
	Cleanup  func()
}

// ensureDockerNotRunningForPackage 用 Docker 实时状态兜底确认环境包没有运行。
//
// SQLite 状态可能因为进程重启或后台同步延迟而过期；备份 browser-data/profile 前必须确认
// 关联容器不是 running。Docker API 不可用时状态不可证明，第一版直接拒绝备份。
func ensureDockerNotRunningForPackage(index *model.BrowserEnvIndex) error {
	containers, err := edgeService.NewEdgeService().GetDockerContainers()
	if err != nil {
		return remoteError(err.Error())
	}
	for _, container := range containers {
		if !isContainerMatchedBrowserEnv(index, container) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(container.State), model.BrowserEnvStatusRunning) {
			return conflictError("Docker 容器仍在运行，请先停止后再备份")
		}
	}
	return nil
}

// isContainerMatchedBrowserEnv 判断 Docker 容器是否属于当前环境包。
//
// 优先使用 label 中的 envId；旧数据可能只有 containerId/containerName，因此保留兼容匹配。
func isContainerMatchedBrowserEnv(index *model.BrowserEnvIndex, container edgeModel.DockerContainer) bool {
	if index == nil {
		return false
	}
	if strings.TrimSpace(container.EnvID) == index.EnvID {
		return true
	}
	if container.Labels != nil && strings.TrimSpace(container.Labels["bv.envId"]) == index.EnvID {
		return true
	}
	if index.ContainerID != nil && strings.TrimSpace(*index.ContainerID) != "" && strings.HasPrefix(container.ID, strings.TrimSpace(*index.ContainerID)) {
		return true
	}
	if index.ContainerName != nil {
		expected := strings.TrimPrefix(strings.TrimSpace(*index.ContainerName), "/")
		for _, name := range container.Names {
			if strings.TrimPrefix(strings.TrimSpace(name), "/") == expected {
				return true
			}
		}
	}
	return false
}

// validateBackupSourcePackage 校验备份源环境包的标准文件。
//
// 这里不读取代理明文或指纹 raw 到响应，只确认包结构足以作为后续 import-package 的输入。
func validateBackupSourcePackage(index *model.BrowserEnvIndex, envPath string) (model.ProfileFile, error) {
	pkg, err := loadAndValidateAtomicPackage(envPath)
	if err != nil {
		return model.ProfileFile{}, err
	}
	profile := pkg.Profile
	if profile.EnvID != index.EnvID || profile.UserID != index.UserID || profile.RPAType != index.RPAType {
		return profile, fmt.Errorf("profile 与 browser_envs 索引不一致")
	}
	return profile, nil
}

func requirePackageFile(envPath string, relativePath string) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("环境包文件缺失 %s: %w", relativePath, err)
	}
	if stat.IsDir() {
		return fmt.Errorf("环境包路径不是文件: %s", relativePath)
	}
	return nil
}

func requirePackageDir(envPath string, relativePath string) error {
	path, err := safePackagePath(envPath, relativePath)
	if err != nil {
		return err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("环境包目录缺失 %s: %w", relativePath, err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("环境包路径不是目录: %s", relativePath)
	}
	return nil
}

// writeExportProfile 只修改 staging profile，写入包协议元信息和文件校验和。
//
// 源环境包不能被污染；exportAction 不参与 identityHash，只用于后续导入审计。
func writeExportProfile(stagingEnvPath string, profile model.ProfileFile, exportAction string) error {
	now := time.Now().Unix()
	packageVersion := browserEnvPackageVersion
	profile.Package.Version = &packageVersion
	profile.Package.ExportedAt = &now
	profile.Package.ExportSource = &model.PackageExportSource{
		Type:           "edge",
		Env:            SettingsEnv(),
		ServiceVersion: SettingsVersion(),
	}
	profile.Package.ExportAction = exportAction
	profile.Package.Checksums = nil
	profile.Metadata.UpdatedAt = now

	checksums, err := buildPackageChecksums(stagingEnvPath)
	if err != nil {
		return err
	}
	profile.Package.Checksums = checksums
	return writeJSONFile(filepath.Join(stagingEnvPath, "profile.json"), profile)
}

// buildPackageChecksums 计算 staging 包内文件 sha256。
//
// profile.json 自身不参与 checksums，避免“profile 记录自己的 hash”造成循环依赖。
func buildPackageChecksums(envPath string) (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(envPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relativePath, err := filepath.Rel(envPath, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if relativePath == "profile.json" {
			return nil
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		result[relativePath] = sum
		return nil
	})
	return result, err
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

// copyDirectory 把源环境包复制到 staging。
//
// 备份必须基于 staging 副本写包元信息，不能污染源目录；restore/import 也复用这个
// 文件复制能力，所以这里只做文件系统复制，不承载任何业务状态变化。
func copyDirectory(source string, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(target, relativePath)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, targetPath, info.Mode().Perm())
	})
}

func copyFile(source string, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// createTarGzFromDirectory 将 staging 环境包打成 tar.gz。
//
// archiveRootName 固定为 envId，保证 tar 根目录不是散文件，导入时可以先校验根目录。
func createTarGzFromDirectory(sourceDir string, archiveRootName string, targetArchive string) error {
	out, err := os.Create(targetArchive)
	if err != nil {
		return err
	}
	defer out.Close()
	gzipWriter := gzip.NewWriter(out)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	paths := make([]string, 0)
	if err = filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		if path == sourceDir {
			continue
		}
		if err = addPathToTar(tarWriter, sourceDir, archiveRootName, path); err != nil {
			return err
		}
	}
	return nil
}

func addPathToTar(writer *tar.Writer, sourceDir string, archiveRootName string, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return nil
	}
	relativePath, err := filepath.Rel(sourceDir, path)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(filepath.Join(archiveRootName, relativePath))
	if info.IsDir() {
		header.Name += "/"
	}
	if err = writer.WriteHeader(header); err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

func buildBackupArchiveFileName(envID string, timestamp int64) string {
	safeEnvID := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(envID)
	return fmt.Sprintf("%s-backup-%d.tar.gz", safeEnvID, timestamp)
}

func SettingsEnv() string {
	mode := strings.TrimSpace(Settings.Conf.Mode)
	if mode == "" {
		return "production"
	}
	return mode
}

func SettingsVersion() string {
	return strings.TrimSpace(Settings.Conf.Version)
}
