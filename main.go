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
// 现在 root 不再依赖 `config-docker.yaml`，而是依赖 `go.mod` / 目录结构，
// 避免把 yaml 文件重新变成启动必需品。
func detectProjectRoot() (string, error) {
	if explicitRoot := os.Getenv("PRIVATE_BROWSER_CLIENT_PROJECT_ROOT"); explicitRoot != "" {
		if hasProjectMarker(explicitRoot) {
			return explicitRoot, nil
		}
		return "", fmt.Errorf("PRIVATE_BROWSER_CLIENT_PROJECT_ROOT is set but invalid: %s", explicitRoot)
	}

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
			if hasProjectMarker(current) {
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

// hasProjectMarker 判断目录是否是当前新 Client 的项目根目录。
//
// 这里改成认 `go.mod`，因为配置文件现在只是可选项。
// 后续如果还要扩展，更应该基于模块根目录，而不是继续回看 yaml。
func hasProjectMarker(root string) bool {
	root = filepath.Clean(root)

	goModPath := filepath.Join(root, "go.mod")
	if stat, statErr := os.Stat(goModPath); statErr == nil && !stat.IsDir() {
		return true
	}

	// 容器镜像里通常只复制运行所需的 `/app/docs`、`/app/public`、`/app/Settings`、`/app/data`
	// 和二进制本身，不会带源码树里的 go.mod。
	// 因此这里必须接受“运行时目录布局”作为根目录判定条件，否则容器永远无法启动。
	requiredDirs := []string{
		filepath.Join(root, "docs"),
		filepath.Join(root, "public"),
		filepath.Join(root, "data"),
	}
	for _, path := range requiredDirs {
		stat, statErr := os.Stat(path)
		if statErr != nil || !stat.IsDir() {
			return false
		}
	}
	return true
}
