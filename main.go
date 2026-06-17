package main

import (
	"fmt"
	"os"
	"path/filepath"

	"private_browser_client/Infrastructures"
)

// main 是新 Private_Browser_Client 的服务入口。
//
// 这次重建后，项目目录层次继续沿用 old，但业务模型已经切到新方案。
// 当前阶段先把最小可运行骨架接起来，让 README/Swagger/Health 这些正式入口先跑通，
// 避免新仓只有静态文件却没有真实服务入口。
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
// 新项目虽然重建了代码，但 `Settings`、`docs`、`public` 仍然是相对项目根目录组织。
// 因此这里继续保留“从当前目录或可执行文件位置反向找根目录”的做法，
// 避免后续把相对路径散落到各个服务和路由里。
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

// hasSettingsConfig 判断目录是否是当前新 Client 的项目根目录。
//
// 这里目前只认 `Settings/config-docker.yaml`，因为当前项目还没有恢复多环境加载逻辑。
// 后续如果重新引入正式配置加载器，应优先扩展这里，而不是把文件名判断复制到别处。
func hasSettingsConfig(root string) bool {
	configPath := filepath.Join(root, "Settings", "config-docker.yaml")
	stat, statErr := os.Stat(configPath)
	return statErr == nil && !stat.IsDir()
}
