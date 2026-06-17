package BrowserEnv

import (
	"regexp"
	"strings"
	"time"

	model "private_browser_client/Models/BrowserEnv"
)

var userIDRe = regexp.MustCompile(`^\d+$`)

// normalizeCreateRequest 统一清洗 create-browser-env 请求。
//
// 设计来源：
// - 请求体里的字段会直接决定 profile/binding/container 的落盘事实；
// - 用户已经明确 language 固定为 us-en，因此不能继续接受外部自由覆盖；
// - 代理 YAML 通过 Base64 传输，是为了避免长文本在前端工具里被截断或转义错。
func normalizeCreateRequest(param *model.CreateBrowserEnvRequest) (*model.CreateBrowserEnvRequest, error) {
	if param == nil {
		return nil, invalidError("请求参数不能为空")
	}
	param.UserID = strings.TrimSpace(param.UserID)
	param.RPAType = strings.ToLower(strings.TrimSpace(param.RPAType))
	param.Name = strings.TrimSpace(param.Name)
	param.Runtime.Image = strings.TrimSpace(param.Runtime.Image)
	param.Runtime.StartupURL = strings.TrimSpace(param.Runtime.StartupURL)
	param.Runtime.ShmSize = strings.TrimSpace(param.Runtime.ShmSize)
	param.Environment.Timezone = strings.TrimSpace(param.Environment.Timezone)
	param.Environment.Language = model.FixedLanguage
	param.Proxy.Type = strings.ToLower(strings.TrimSpace(param.Proxy.Type))

	if param.UserID == "" || !userIDRe.MatchString(param.UserID) {
		return nil, invalidError("userId 必须是非空数字字符串")
	}
	if _, ok := model.SupportedRPATypes[param.RPAType]; !ok {
		return nil, invalidError("rpaType 仅支持 tk/fb/ins/yt/x/other")
	}
	if param.Name == "" {
		return nil, invalidError("name 不能为空")
	}
	if param.Runtime.Image == "" {
		return nil, invalidError("runtime.image 不能为空")
	}
	if param.Runtime.StartupURL == "" {
		param.Runtime.StartupURL = model.DefaultStartupURL
	}
	if param.Runtime.ShmSize == "" {
		param.Runtime.ShmSize = model.DefaultShmSize
	}
	if param.Environment.Timezone == "" {
		return nil, invalidError("environment.timezone 不能为空")
	}
	if _, err := time.LoadLocation(param.Environment.Timezone); err != nil {
		return nil, invalidError("environment.timezone 不是有效 IANA 时区")
	}
	if param.Environment.Screen.Width <= 0 || param.Environment.Screen.Height <= 0 {
		return nil, invalidError("environment.screen.width 和 height 必须为正整数")
	}
	if param.Environment.Screen.Depth <= 0 {
		param.Environment.Screen.Depth = model.DefaultScreenDepth
	}
	if param.Proxy.Enabled == nil {
		return nil, invalidError("proxy.enabled 必填")
	}
	if *param.Proxy.Enabled {
		if param.Proxy.Type == "" {
			param.Proxy.Type = "clash"
		}
		if param.Proxy.Type != "clash" {
			return nil, invalidError("proxy.type 当前仅支持 clash")
		}
		config, err := decodeProxyConfigBase64(param.Proxy.ConfigBase64)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(config) == "" {
			return nil, invalidError("proxy.enabled=true 时 proxy.configBase64 不能为空")
		}
		param.Proxy.Config = config
		return param, nil
	}

	param.Proxy.Type = ""
	param.Proxy.ConfigBase64 = ""
	param.Proxy.Config = ""
	return param, nil
}
