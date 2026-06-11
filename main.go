package main

import (
	"fmt"
	"os"
	"path/filepath"

	"private_browser_client/Infrastructures"
)

// main 是当前 Private_Browser_Client 的纯后端入口。
//
// 这个入口的来历：
// 1. 这个项目原先尝试过 Wails 桌面壳，但用户已经明确要求暂时彻底放弃桌面客户端形态；
// 2. 现在这里只承担“启动 RESTful 服务”的职责，不再夹带前端构建、桌面窗口和混合运行逻辑；
// 3. 当前服务已经收紧为边缘服务，后续只在这里挂本机 Docker / 浏览器实例能力，不再塞入用户鉴权和多节点中控逻辑。
func main() {
	projectRoot, err := detectProjectRoot()
	if err != nil {
		fmt.Printf("detect project root failed, err:%v\n", err)
		os.Exit(1)
	}

	if err = Infrastructures.Init(projectRoot); err != nil {
		fmt.Printf("init infrastructure failed, err:%v\n", err)
		os.Exit(1)
	}
}

// detectProjectRoot 负责识别项目根目录。
//
// 这样设计的原因：
// - 用户要求保留自己熟悉的 Go 项目目录结构，而 `Settings`、`docs`、`data` 都依赖项目根目录做相对路径定位；
// - 早期桌面客户端入口通常默认“当前工作目录就是项目目录”，但服务端部署后这个假设并不稳定；
// - 所以后续维护时如果调整启动方式，优先扩展这里的查找策略，不要把大量相对路径散落回业务代码里。
func detectProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	candidates := []string{cwd}
	if exePath, exeErr := os.Executable(); exeErr == nil {
		candidates = append(candidates, filepath.Dir(exePath))
	}

	for _, start := range candidates {
		current := start
		for {
			if hasSettingsConfig(current) {
				return current, nil
			}

			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	return "", fmt.Errorf("project root not found")
}

// hasSettingsConfig 判断目录是否是当前边缘服务项目根目录。
//
// 设计来源：
// - 早期支持过 config-dev/test/prod/docker 多文件选择，导致 `go run .` 默认找 config-prod.yaml；
// - 用户已经确认 Edge Client 后期只保留 `config-docker.yaml`，所有运行都按生产口径；
// - 因此这里只识别唯一配置文件，避免项目启动逻辑再退回多环境模式。
func hasSettingsConfig(root string) bool {
	configPath := filepath.Join(root, "Settings", "config-docker.yaml")
	stat, statErr := os.Stat(configPath)
	return statErr == nil && !stat.IsDir()
}
