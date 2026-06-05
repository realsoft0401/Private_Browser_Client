package BrowserEnv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// buildTextHash 计算普通文本字段的 sha256。
//
// 当前主要用于代理配置摘要或排障比较，不参与 identityHash。这里会统一 CRLF 和首尾空白，
// 避免同一份 Clash YAML 因换行格式不同产生不同 hash。
func buildTextHash(value string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// buildJSONHash 计算结构化对象的 sha256。
//
// identityHash 必须从结构化模型计算，而不是手工拼字符串。
// 当前 identityHash 只包含 envId/userId/rpaType，避免配置变化误伤环境身份。
func buildJSONHash(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
