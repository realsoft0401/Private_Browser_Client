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
)

// ExportAndRemoveBrowserEnvPackage 导出环境包后从本机移除源环境。
//
// 设计来源：
// - 用户明确区分 backup 和 export：backup 只是复制，export 是“迁移走并从本机删除”；
// - 环境包包含 browser-data/profile 登录态，导出时必须拒绝 running，避免打包半写入状态；
// - 导出成功后要删除已停止容器、物理目录和 SQLite 索引，不能留下会被 rebuild-index 恢复的源包。
//
// 职责边界：
// - 复用备份包协议生成 tar.gz 下载包，但 manifest.exportAction 写 export-and-remove；
// - 不自动停止运行中容器，不删除 Docker 镜像；
// - 只有 archive 已成功生成后才进入删除流程，避免导出失败时误删源数据。
func (s *Service) ExportAndRemoveBrowserEnvPackage(envID string) (*BackupBrowserEnvPackageResult, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}

	runEnvMu.Lock()
	defer runEnvMu.Unlock()

	handler := browserEnvDao.NewDeleteModelHandler()
	index, err := handler.GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	if index.Status == model.BrowserEnvStatusDeleted || index.Status == model.BrowserEnvStatusArchived {
		return nil, conflictError("环境包已删除或归档，不能导出")
	}
	if index.Status == model.BrowserEnvStatusRunning || index.ContainerStatus == model.BrowserEnvStatusRunning {
		return nil, conflictError("环境包正在运行，请先停止后再导出")
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

	result, err := buildPackageArchive(index, sourceEnvPath, manifest, "export-and-remove", buildExportArchiveFileName)
	if err != nil {
		return nil, err
	}
	completed := false
	defer func() {
		if !completed && result != nil && result.Cleanup != nil {
			result.Cleanup()
		}
	}()

	if err = removeStoppedContainerForDelete(index); err != nil {
		return nil, err
	}
	if err = os.RemoveAll(sourceEnvPath); err != nil {
		return nil, internalError(fmt.Sprintf("删除源环境包目录失败: %v", err))
	}
	if err = handler.DeleteBrowserEnvIndex(context.Background(), index.EnvID); err != nil {
		return nil, internalError(err.Error())
	}

	completed = true
	return result, nil
}

func buildExportArchiveFileName(envID string, timestamp int64) string {
	safeEnvID := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(envID)
	return fmt.Sprintf("%s-export-%d.tar.gz", safeEnvID, timestamp)
}

// buildPackageArchive 从源目录复制到 staging 并生成 tar.gz。
//
// backup 和 export-and-remove 都走这个 helper，保证两类下载包只有 exportAction 不同，
// 不会出现“备份包能导入、导出包不能导入”的协议分裂。
func buildPackageArchive(index *model.BrowserEnvIndex, sourceEnvPath string, manifest model.ManifestFile, exportAction string, fileNameBuilder func(string, int64) string) (*BackupBrowserEnvPackageResult, error) {
	stagingRoot, err := os.MkdirTemp("", "private-browser-package-*")
	if err != nil {
		return nil, internalError(fmt.Sprintf("创建环境包 staging 目录失败: %v", err))
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
	if err = writeExportManifest(stagingEnvPath, manifest, exportAction); err != nil {
		return nil, internalError(err.Error())
	}

	archivePath := filepath.Join(stagingRoot, fileNameBuilder(index.EnvID, time.Now().Unix()))
	if err = createTarGzFromDirectory(stagingEnvPath, index.EnvID, archivePath); err != nil {
		return nil, internalError(err.Error())
	}
	stat, err := os.Stat(archivePath)
	if err != nil {
		return nil, internalError(fmt.Sprintf("读取环境包归档失败: %v", err))
	}
	if stat.Size() <= 0 {
		return nil, internalError("环境包归档为空")
	}

	cleanupPaths = append(cleanupPaths, archivePath)
	completed = true
	return &BackupBrowserEnvPackageResult{
		FilePath: archivePath,
		FileName: filepath.Base(archivePath),
		Cleanup:  cleanup,
	}, nil
}
