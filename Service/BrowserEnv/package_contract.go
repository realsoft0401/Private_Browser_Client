package BrowserEnv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Settings"
)

// loadPackageProfileFromIndex 是新环境包契约的统一入口。
//
// 设计来源：
//   - 用户在 2026-06-09 明确要求 profile.json 才是环境包的唯一详细配置文档、
//     环境资产身份证和 SQLite 重建入口；
//   - 旧 manifest.json 会制造两份主配置，数据库丢失后也无法可靠重构配置服务；
//   - 因此所有生命周期动作必须先读取 profile.json，再通过 profile.paths 读取 binding、
//     container、proxy、fingerprint 和 browser-data。
//
// 职责边界：
// - 只根据 SQLite 索引定位本机环境包目录并读取 profile.json；
// - 只校验 profile 与索引的核心身份一致性；
// - 不创建缺失目录、不补默认路径、不读取登录态内容，缺少原子材料应由上层校验失败。
func loadPackageProfileFromIndex(index *model.BrowserEnvIndex) (string, model.ProfileFile, error) {
	if index == nil {
		return "", model.ProfileFile{}, fmt.Errorf("环境包索引不能为空")
	}
	if Settings.Conf.ProjectRoot == "" {
		return "", model.ProfileFile{}, fmt.Errorf("project root 不能为空")
	}
	absoluteEnvPath := filepath.Join(Settings.Conf.ProjectRoot, filepath.FromSlash(index.EnvPath))
	if stat, err := os.Stat(absoluteEnvPath); err != nil {
		return "", model.ProfileFile{}, fmt.Errorf("环境包目录不存在: %w", err)
	} else if !stat.IsDir() {
		return "", model.ProfileFile{}, fmt.Errorf("环境包路径不是目录")
	}

	var profile model.ProfileFile
	if err := readJSONFile(filepath.Join(absoluteEnvPath, "profile.json"), &profile); err != nil {
		return "", model.ProfileFile{}, fmt.Errorf("读取 profile.json 失败: %w", err)
	}
	if profile.EnvID != index.EnvID || profile.UserID != index.UserID || profile.RPAType != index.RPAType {
		return "", model.ProfileFile{}, fmt.Errorf("profile 与 browser_envs 索引不一致")
	}
	if profile.SchemaVersion != model.SchemaVersion {
		return "", model.ProfileFile{}, fmt.Errorf("profile.schemaVersion 不支持")
	}
	if err := validateProfilePackagePaths(profile.Paths); err != nil {
		return "", model.ProfileFile{}, err
	}
	return absoluteEnvPath, profile, nil
}

// validateProfilePackagePaths 校验 profile.paths 是否足以作为环境包入口。
//
// 这里不自动补默认值，是因为用户明确要求缺少关键文件或关键字段时视为伪造/不可信数据；
// 如果导入包里没有这些路径，后续 run/import/rebuild 都不能进入系统。
func validateProfilePackagePaths(paths model.PackagePaths) error {
	required := map[string]string{
		"paths.profile":                  paths.Profile,
		"paths.binding":                  paths.Binding,
		"paths.container":                paths.Container,
		"paths.browserData":              paths.BrowserData,
		"paths.fingerprintSnapshot":      paths.FingerprintSnapshot,
		"paths.fingerprintBackup":        paths.FingerprintBackup,
		"paths.fingerprintRuntimeConfig": paths.FingerprintRuntimeConfig,
		"paths.proxyConfig":              paths.ProxyConfig,
		"paths.proxyRuntime":             paths.ProxyRuntime,
		"paths.logs":                     paths.Logs,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("profile.%s 不能为空", name)
		}
	}
	return nil
}
