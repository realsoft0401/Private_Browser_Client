package BrowserEnv

import (
	"strings"

	model "private_browser_client/Models/BrowserEnv"
)

// edgeBrowserContainerName 统一生成浏览器环境运行容器名。
//
// 设计来源：
// - 用户在 2026-06-11 明确要求把旧的 `bv-<envId>` 命名收紧成
//   `private-browser-edge-<userId>-<rpaType>-<snowflakeId>` 这类可读前缀；
// - project.md 已明确 envId 格式是 `userId_rpaType_snowflakeId`，因此这里显式按三段拆解，
//   而不是把整个 envId 直接做字符串替换，避免后续维护者把末段误认成 identityHash；
// - create/import/rebuild-index/revalidate/restore 都必须复用同一规则，不能再各写各的名字。
//
// 职责边界：
// - 这里只负责生成浏览器环境容器名，不负责 Edge service 自己的容器名；
// - 优先使用 `userId/rpaType/snowflakeId` 三段稳定拼接；如果遇到历史异常 envId，再退回 `_ -> -` 兜底；
// - 如果后续容器名规则再调整，必须优先改这里，并同步更新文档和排障脚本。
func edgeBrowserContainerName(envID string) string {
	trimmed := strings.TrimSpace(envID)
	parts := strings.Split(trimmed, "_")
	if len(parts) == 3 {
		return "private-browser-edge-" + parts[0] + "-" + parts[1] + "-" + parts[2]
	}
	return "private-browser-edge-" + strings.ReplaceAll(trimmed, "_", "-")
}

func containerNameForProfile(profile model.ProfileFile) string {
	return edgeBrowserContainerName(profile.EnvID)
}
