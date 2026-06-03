package BrowserEnv

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	model "private_browser_client/Models/BrowserEnv"
)

// buildPackageArchive 从源目录复制到 staging 并生成 tar.gz。
//
// 设计来源：
// - 当前公开流程已经收敛为 backup/restore，不再保留临时下载或导出删除接口；
// - 但备份、恢复和外部导入都需要同一套标准 tar.gz 包协议；
// - 因此底层打包 helper 保留为内部能力，只负责 staging、manifest 元信息、checksums 和 tar.gz。
//
// 职责边界：
// - 只处理 staging 副本，不能污染源环境包；
// - 不删除源目录、不更新 SQLite、不操作 Docker；
// - 调用方必须在业务动作完成后调用 result.Cleanup 清理临时目录。
func buildPackageArchive(index *model.BrowserEnvIndex, sourceEnvPath string, manifest model.ManifestFile, exportAction string, fileNameBuilder func(string, int64) string) (*PackageArchiveResult, error) {
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
	return &PackageArchiveResult{
		FilePath: archivePath,
		FileName: filepath.Base(archivePath),
		Cleanup:  cleanup,
	}, nil
}
