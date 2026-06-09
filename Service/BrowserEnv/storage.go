package BrowserEnv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	model "private_browser_client/Models/BrowserEnv"
)

// envPackageFiles 聚合一次创建环境包需要写入的文件内容。
//
// 这个结构只在 Service 内部使用，目的是让主流程不用直接关心每个 JSON 文件的写入细节。
type envPackageFiles struct {
	Profile       model.ProfileFile
	Binding       model.BindingFile
	Container     model.ContainerFile
	Snapshot      model.FingerprintSnapshotFile
	Backup        model.FingerprintBackupFile
	RuntimeConfig any
	ProxyRuntime  model.ProxyRuntimeFile
	ProxyConfig   string
}

// createEnvDirectories 创建环境包目录骨架。
//
// 目录必须围绕“一个环境包可整体打包迁移”组织，profile、binding、指纹、
// 代理、browser-data 不能散落到项目其他位置。
func createEnvDirectories(envPath string) error {
	dirs := []string{
		envPath,
		filepath.Join(envPath, "fingerprint"),
		filepath.Join(envPath, "proxy"),
		filepath.Join(envPath, "browser-data", "profile"),
		filepath.Join(envPath, "logs"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}
	}
	return nil
}

// writeEnvPackageFiles 写入完整环境包文件。
//
// 职责边界：
// - 只负责把已经构建好的文件模型落盘；
// - 不计算 hash，不修改业务字段；
// - proxy/clash.yaml 保持文本原文写入，其他结构统一 JSON 缩进输出。
func writeEnvPackageFiles(envPath string, paths model.PackagePaths, files envPackageFiles) error {
	writeTasks := []struct {
		relativePath string
		value        any
	}{
		{paths.Profile, files.Profile},
		{paths.Binding, files.Binding},
		{paths.Container, files.Container},
		{paths.FingerprintSnapshot, files.Snapshot},
		{paths.FingerprintBackup, files.Backup},
		{paths.FingerprintRuntimeConfig, files.RuntimeConfig},
		{paths.ProxyRuntime, files.ProxyRuntime},
	}
	for _, task := range writeTasks {
		if err := writeJSONFile(filepath.Join(envPath, filepath.FromSlash(task.relativePath)), task.value); err != nil {
			return err
		}
	}
	return writeTextFile(filepath.Join(envPath, filepath.FromSlash(paths.ProxyConfig)), files.ProxyConfig)
}

func writeJSONFile(path string, value any) error {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 序列化失败 %s: %w", path, err)
	}
	bytes = append(bytes, '\n')
	return writeTextFile(path, string(bytes))
}

func writeTextFile(path string, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建文件目录失败 %s: %w", filepath.Dir(path), err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败 %s: %w", path, err)
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()
	if _, err = temp.Write([]byte(value)); err != nil {
		_ = temp.Close()
		return fmt.Errorf("写临时文件失败 %s: %w", tempName, err)
	}
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("同步临时文件失败 %s: %w", tempName, err)
	}
	if err = temp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败 %s: %w", tempName, err)
	}
	if err = os.Chmod(tempName, 0644); err != nil {
		return fmt.Errorf("设置临时文件权限失败 %s: %w", tempName, err)
	}
	if err = os.Rename(tempName, path); err != nil {
		return fmt.Errorf("原子替换文件失败 %s: %w", path, err)
	}
	cleanup = false
	return nil
}
