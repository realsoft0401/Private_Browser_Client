package BrowserEnv

import (
	"regexp"
	"strings"
	"time"

	model "private_browser_client/Models/BrowserEnv"
)

var userIDRe = regexp.MustCompile(`^\d+$`)

const (
	defaultListPage     = 1
	defaultListPageSize = 20
	maxListPageSize     = 100
)

// normalizeCreateRequest 校验并补齐创建环境包请求。
//
// 设计来源：
// - 请求参数会直接写入 profile/binding，决定后续 run、迁移、指纹恢复的事实来源；
// - 因此默认值必须在落盘前统一补齐，不能分散到写文件或 Docker 阶段。
//
// 职责边界：
// - 这里只做请求清洗、默认值和业务参数校验；
// - 不生成 envId，不扫描 envSequence，不写文件。
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
	param.Environment.Language = strings.TrimSpace(param.Environment.Language)
	param.Proxy.Type = strings.TrimSpace(param.Proxy.Type)
	param.Metadata.Source = strings.TrimSpace(param.Metadata.Source)
	param.Metadata.Description = strings.TrimSpace(param.Metadata.Description)

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
	if param.Environment.Language == "" {
		return nil, invalidError("environment.language 不能为空")
	}
	if param.Environment.Screen.Width <= 0 || param.Environment.Screen.Height <= 0 {
		return nil, invalidError("screen.width / screen.height 必须为正整数")
	}
	if param.Environment.Screen.Depth <= 0 {
		param.Environment.Screen.Depth = model.DefaultScreenDepth
	}
	if param.Proxy.Enabled == nil {
		return nil, invalidError("proxy.enabled 必填")
	}
	if *param.Proxy.Enabled {
		if param.Proxy.Type == "" {
			param.Proxy.Type = "clash-verge"
		}
		if param.Proxy.Type != "clash-verge" {
			return nil, invalidError("proxy.type 第一版仅支持 clash-verge")
		}
		if strings.TrimSpace(param.Proxy.Config) == "" {
			return nil, invalidError("proxy.enabled=true 时 proxy.config 不能为空")
		}
	} else {
		param.Proxy.Type = ""
		param.Proxy.Config = ""
	}
	if param.Metadata.Source == "" {
		param.Metadata.Source = "api"
	}
	if param.Fingerprint != nil && param.Fingerprint.Backup != nil &&
		param.Fingerprint.Backup.Available && param.Fingerprint.Backup.Fingerprint == nil {
		return nil, invalidError("fingerprint.backup.available=true 时 fingerprint 不能为空")
	}
	return param, nil
}

// normalizeListQuery 校验并补齐环境包列表查询条件。
//
// 设计来源：
// - 列表接口直接面向前端分页和筛选，如果不统一 page/pageSize/status 规则，后续统计和列表很容易口径不一致；
// - 默认不显示 deleted，是用户确认的假删除策略，查看回收站必须显式传 status=deleted。
//
// 职责边界：
// - 这里只做查询参数清洗、分页兜底和枚举校验；
// - 不访问数据库，不判断环境包目录是否存在，不读取 profile 文件。
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
		return query, invalidError("status 仅支持 created/running/stopped/deleted/archived/error")
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

// isSupportedBrowserEnvStatus 统一判断 browser_envs.status 枚举。
//
// 状态枚举会同时影响查询、软删除、run/stop 状态刷新；
// 单独抽出来是为了后续新增状态时只改一个入口，避免接口层和数据库层各写一份判断。
func isSupportedBrowserEnvStatus(status string) bool {
	switch status {
	case model.BrowserEnvStatusCreated,
		model.BrowserEnvStatusRunning,
		model.BrowserEnvStatusStopped,
		model.BrowserEnvStatusDeleted,
		model.BrowserEnvStatusArchived,
		model.BrowserEnvStatusError:
		return true
	default:
		return false
	}
}
