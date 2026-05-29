package BrowserEnv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"private_browser_client/Settings"
)

// nextEnvSequence 扫描本机环境包，生成下一个本机运行序号。
//
// 设计来源：
// - 用户要求 envSequence 由边缘服务本地 +1 生成，用户和服务端都不需要传；
// - cdp/vnc 端口依赖 envSequence 形成可辨识规则；
// - envSequence 不属于账号环境身份，导入迁移时如果端口冲突可以重新分配。
//
// 第一版直接扫描 profile.json，避免先引入 index 文件带来一致性问题。
// 后续环境数量很大时，可以再加只读索引加速，但必须保留扫描兜底。
func nextEnvSequence() (int, error) {
	root := filepath.Join(Settings.Conf.ProjectRoot, "data", "browser-envs")
	maxSequence := 0
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return 1, nil
	} else if err != nil {
		return 0, fmt.Errorf("读取 browser-envs 根目录失败: %w", err)
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "profile.json" {
			return nil
		}
		var payload struct {
			EnvSequence int `json:"envSequence"`
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		if payload.EnvSequence > maxSequence {
			maxSequence = payload.EnvSequence
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("扫描 envSequence 失败: %w", err)
	}
	return maxSequence + 1, nil
}
