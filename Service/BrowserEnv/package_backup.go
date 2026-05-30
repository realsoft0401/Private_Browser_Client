package BrowserEnv

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeModel "private_browser_client/Models/Edge"
	edgeService "private_browser_client/Service/Edge"
	"private_browser_client/Settings"
)

const browserEnvPackageVersion = 1

// BackupBrowserEnvPackageResult 是备份包生成后的内部结果。
//
// HTTP 层只负责把 FilePath 作为下载流返回；Cleanup 必须在响应完成后调用，
// 避免临时 tar.gz 和 staging 目录长期留在本机。
type BackupBrowserEnvPackageResult struct {
	FilePath string
	FileName string
	Cleanup  func()
}

// BackupBrowserEnvPackage 生成环境包 .tar.gz 备份。
//
// 设计来源：
// - 用户确认在实现 import-package 前必须先有标准备份包，避免导入一个只存在于文档里的理论格式；
// - 备份动作只生成包，不删除源目录、SQLite 索引、Docker 容器或浏览器镜像；
// - 运行中备份会产生半一致 browser-data/profile，因此第一版必须拒绝 running 状态。
//
// 职责边界：
// - 负责校验索引、运行态、环境包路径和标准文件；
// - 负责复制到 staging，只修改 staging 里的 manifest 导出元信息；
// - 负责生成 checksums 和 tar.gz 文件；
// - 不修改源环境包，不改变 identityHash，不参与导入落库。
func (s *Service) BackupBrowserEnvPackage(envID string) (*BackupBrowserEnvPackageResult, error) {
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
	if index.Status == model.BrowserEnvStatusDeleted || index.Status == model.BrowserEnvStatusArchived {
		return nil, conflictError("环境包已删除或归档，不能备份")
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
	manifest, err := validateBackupSourcePackage(index, sourceEnvPath)
	if err != nil {
		return nil, internalError(err.Error())
	}

	stagingRoot, err := os.MkdirTemp("", "private-browser-backup-*")
	if err != nil {
		return nil, internalError(fmt.Sprintf("创建备份 staging 目录失败: %v", err))
	}
	cleanupPaths := []string{stagingRoot}
	cleanup := func() {
		for _, path := range cleanupPaths {
			_ = os.RemoveAll(path)
		}
	}
	completed := false
	defer func() {
		if !completed {
			cleanup()
		}
	}()

	stagingEnvPath := filepath.Join(stagingRoot, index.EnvID)
	if err = copyDirectory(sourceEnvPath, stagingEnvPath); err != nil {
		return nil, internalError(err.Error())
	}
	if err = writeExportManifest(stagingEnvPath, manifest, "backup-package"); err != nil {
		return nil, internalError(err.Error())
	}

	archivePath := filepath.Join(stagingRoot, buildBackupArchiveFileName(index.EnvID, time.Now().Unix()))
	if err = createTarGzFromDirectory(stagingEnvPath, index.EnvID, archivePath); err != nil {
		return nil, internalError(err.Error())
	}
	stat, err := os.Stat(archivePath)
	if err != nil {
		return nil, internalError(fmt.Sprintf("读取备份包失败: %v", err))
	}
	if stat.Size() <= 0 {
		return nil, internalError("备份包为空")
	}

	cleanupPaths = append(cleanupPaths, archivePath)
	completed = true
	return &BackupBrowserEnvPackageResult{
		FilePath: archivePath,
		FileName: filepath.Base(archivePath),
		Cleanup:  cleanup,
	}, nil
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
func validateBackupSourcePackage(index *model.BrowserEnvIndex, envPath string) (model.ManifestFile, error) {
	var manifest model.ManifestFile
	if err := readJSONFile(filepath.Join(envPath, "manifest.json"), &manifest); err != nil {
		return manifest, err
	}
	if manifest.EnvID != index.EnvID || manifest.UserID != index.UserID || manifest.RPAType != index.RPAType {
		return manifest, fmt.Errorf("manifest 与 browser_envs 索引不一致")
	}

	requiredFiles := []string{
		manifest.Paths.Profile,
		manifest.Paths.Binding,
		manifest.Paths.Container,
		manifest.Paths.FingerprintSnapshot,
		manifest.Paths.FingerprintBackup,
		manifest.Paths.FingerprintRuntimeConfig,
		manifest.Paths.ProxyRuntime,
	}
	if manifest.Paths.ProxyConfig != "" {
		requiredFiles = append(requiredFiles, manifest.Paths.ProxyConfig)
	}
	for _, relativePath := range requiredFiles {
		if err := requirePackageFile(envPath, relativePath); err != nil {
			return manifest, err
		}
	}
	requiredDirs := []string{
		manifest.Paths.BrowserData,
		manifest.Paths.Logs,
		"proxy",
		"fingerprint",
	}
	for _, relativePath := range requiredDirs {
		if err := requirePackageDir(envPath, relativePath); err != nil {
			return manifest, err
		}
	}
	return manifest, nil
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

// writeExportManifest 只修改 staging manifest，写入导出协议元信息和文件校验和。
//
// 源环境包不能被污染；exportAction 也不参与 identityHash，只用于后续导入审计。
func writeExportManifest(stagingEnvPath string, manifest model.ManifestFile, exportAction string) error {
	now := time.Now().Unix()
	packageVersion := browserEnvPackageVersion
	manifest.PackageVersion = &packageVersion
	manifest.ExportedAt = &now
	manifest.ExportSource = &model.ManifestExportSource{
		Type:           "edge",
		Env:            SettingsEnv(),
		ServiceVersion: SettingsVersion(),
	}
	manifest.ExportAction = exportAction
	manifest.Checksums = nil

	checksums, err := buildPackageChecksums(stagingEnvPath)
	if err != nil {
		return err
	}
	manifest.Checksums = checksums
	return writeJSONFile(filepath.Join(stagingEnvPath, "manifest.json"), manifest)
}

// buildPackageChecksums 计算 staging 包内文件 sha256。
//
// manifest.json 自身不参与 checksums，避免“manifest 记录自己的 hash”造成循环依赖。
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
		if relativePath == "manifest.json" {
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
// 备份和迁移导出都必须基于 staging 副本写导出元信息，不能污染源目录。
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
	return strings.TrimSpace(Settings.Conf.Env)
}

func SettingsVersion() string {
	return strings.TrimSpace(Settings.Conf.Version)
}
