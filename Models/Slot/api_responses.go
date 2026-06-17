package Slot

// VNCInfoResponse 是 slot 视角的 VNC/noVNC 连接信息。
//
// 设计来源：
// - 新模型已经明确 WebVNC 必须按 slot 视角访问；
// - 前端和 Node 后续都应该围绕 slot 查看当前浏览器画面，而不是继续拿 packageId/envId 当连接对象；
// - 当前响应只返回连接事实，不负责判断 package 业务是否成功。
type VNCInfoResponse struct {
	SlotID    string `json:"slotId"`
	VNCPort   int    `json:"vncPort"`
	VNCURL    string `json:"vncUrl"`
	WSURL     string `json:"wsUrl"`
	WebVNCURL string `json:"webVncUrl"`
	CDPPort   *int   `json:"cdpPort,omitempty"`
}

// CDPInfoResponse 是 slot 视角的 CDP 连接信息。
//
// 当前先只返回连接入口，不在这个接口里混入复杂诊断。
// 后续如果要加 `/cdp-test` 或 `/json/version` 代理诊断，再扩独立接口。
type CDPInfoResponse struct {
	SlotID     string  `json:"slotId"`
	CDPPort    int     `json:"cdpPort"`
	HTTPURL    string  `json:"httpUrl"`
	VersionURL string  `json:"versionUrl"`
	WSBaseURL  *string `json:"wsBaseUrl,omitempty"`
}
