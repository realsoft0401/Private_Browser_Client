package BrowserEnv

import (
	"regexp"
	"strings"
	"time"

	model "private_browser_client/Models/BrowserEnv"
)

var userIDRe = regexp.MustCompile(`^\d+$`)
var dockerImageRefRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]*$`)

const (
	defaultListPage     = 1
	defaultListPageSize = 20
	maxListPageSize     = 100
)

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

// normalizeRuntimeImage 清洗受控 runtime.image 修改入参。
//
// 这条校验只做最轻量的 Docker image 引用保护：
// - 必须非空；
// - 不能包含空白字符；
// - 不能携带 shell 特殊文本或 JSON 片段。
// 真正镜像是否存在、是否可拉取，仍由 pull-image/run 链路在 Docker 层给出明确错误。
func normalizeRuntimeImage(image string) (string, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", invalidError("runtime.image 不能为空")
	}
	if !dockerImageRefRe.MatchString(image) {
		return "", invalidError("runtime.image 格式非法，请传完整 Docker image 引用")
	}
	return image, nil
}

// normalizeListQuery 统一清洗 browser-env 列表查询条件。
//
// 设计来源：
// - 前端和 Node Server 都会直接使用这条接口做回读，如果分页和筛选口径不统一，后续统计会互相打架；
// - 默认不展示 deleted，是为了避免历史删除记录污染正常列表；
// - 这里只做参数清洗，不访问数据库、不读取任何环境包文件。
func normalizeListQuery(query model.ListBrowserEnvQuery) (model.ListBrowserEnvQuery, error) {
	query.UserID = strings.TrimSpace(query.UserID)
	query.RPAType = strings.ToLower(strings.TrimSpace(query.RPAType))
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))

	if query.UserID != "" && !userIDRe.MatchString(query.UserID) {
		return query, invalidError("userId 必须是数字字符串")
	}
	if query.RPAType != "" {
		if _, ok := model.SupportedRPATypes[query.RPAType]; !ok {
			return query, invalidError("rpaType 仅支持 tk/fb/ins/yt/x/other")
		}
	}
	if query.Status != "" && !isSupportedBrowserEnvStatus(query.Status) {
		return query, invalidError("status 仅支持 created/running/stopped/backed_up/deleted/error")
	}
	if query.Page <= 0 {
		query.Page = defaultListPage
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultListPageSize
	}
	if query.PageSize > maxListPageSize {
		query.PageSize = maxListPageSize
	}
	return query, nil
}

func isSupportedBrowserEnvStatus(status string) bool {
	switch status {
	case model.BrowserEnvStatusCreated,
		model.BrowserEnvStatusRunning,
		model.BrowserEnvStatusStopped,
		model.BrowserEnvStatusBackedUp,
		model.BrowserEnvStatusDeleted,
		model.BrowserEnvStatusError:
		return true
	default:
		return false
	}
}
