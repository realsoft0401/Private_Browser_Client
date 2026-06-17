package BrowserEnv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	model "private_browser_client/Models/BrowserEnv"
)

// envPackageFiles 聚合一次创建要落盘的全部文件。
type envPackageFiles struct {
	Profile      model.ProfileFile
	Binding      model.BindingFile
	Container    model.ContainerFile
	Snapshot     model.FingerprintSnapshotFile
	Backup       model.FingerprintBackupFile
	ProxyRuntime model.ProxyRuntimeFile
	ProxyConfig  string
}

// createEnvDirectories 创建正式 browser-env 目录骨架。
//
// 目录结构必须围绕“一个环境包是一套原子资产”组织，
// 避免 profile、proxy、browser-data 散落到别处后无法 backup/restore。
func createEnvDirectories(envPath string) error {
	dirs := []string{
		envPath,
		filepath.Join(envPath, "fingerprint"),
		filepath.Join(envPath, "proxy"),
		filepath.Join(envPath, "browser-data", "profile"),
		filepath.Join(envPath, "logs"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}
	}
	return nil
}

func writeEnvPackageFiles(envPath string, paths model.PackagePaths, files envPackageFiles) error {
	writeTasks := []struct {
		relativePath string
		value        any
	}{
		{relativePath: paths.Profile, value: files.Profile},
		{relativePath: paths.Binding, value: files.Binding},
		{relativePath: paths.Container, value: files.Container},
		{relativePath: paths.FingerprintSnapshot, value: files.Snapshot},
		{relativePath: paths.FingerprintBackup, value: files.Backup},
		{relativePath: paths.ProxyRuntime, value: files.ProxyRuntime},
	}
	for _, task := range writeTasks {
		if err := writeJSONFile(filepath.Join(envPath, filepath.FromSlash(task.relativePath)), task.value); err != nil {
			return err
		}
	}
	return writeTextFile(filepath.Join(envPath, filepath.FromSlash(paths.ProxyConfig)), files.ProxyConfig)
}

func writeJSONFile(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 序列化失败 %s: %w", path, err)
	}
	body = append(body, '\n')
	return writeTextFile(path, string(body))
}

// writeTextFile 用原子替换方式落盘关键文件。
//
// 这样做是为了避免 create 过程中程序中断时留下半写入文件，
// 后续 revalidate/rebuild-index 看到半截 JSON 会很难收口。
func writeTextFile(path string, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
	if _, err = temp.WriteString(value); err != nil {
		_ = temp.Close()
		return fmt.Errorf("写入临时文件失败 %s: %w", tempName, err)
	}
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("同步临时文件失败 %s: %w", tempName, err)
	}
	if err = temp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败 %s: %w", tempName, err)
	}
	if err = os.Chmod(tempName, 0o644); err != nil {
		return fmt.Errorf("设置临时文件权限失败 %s: %w", tempName, err)
	}
	if err = os.Rename(tempName, path); err != nil {
		return fmt.Errorf("原子替换文件失败 %s: %w", path, err)
	}
	cleanup = false
	return nil
}
