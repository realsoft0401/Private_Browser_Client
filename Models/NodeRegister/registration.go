package NodeRegister

// RegistrationState 表示 Node Server 当前返回给 Client 的中心登记结果。
//
// 设计来源：
//   - 用户要求 Node bind 成功后，要把唯一标识返回给 Client；
//   - 这个唯一标识仍然是 Node Server 生成的 clientId，不是 Client 自己计算；
//   - 最新第一阶段定案已经改成“Node bind 后反向 assign 给 Client”，
//     因此这个结构既用于接口返回，也用于本地 node-registration.json 留痕；
//   - 但它仍然不是中心真相源，不能反向覆盖 Node 中心事实。
type RegistrationState struct {
	ClientID          string `json:"clientId"`
	MainAccountID     string `json:"mainAccountId"`
	NodeServerBaseURL string `json:"nodeServerBaseUrl"`
	NodeName          string `json:"nodeName"`
	BaseURL           string `json:"baseUrl"`
	ClientIP          string `json:"clientIp"`
	DockerAPIURL      string `json:"dockerApiUrl"`
	Source            string `json:"source"`
	RegisteredAt      int64  `json:"registeredAt"`
	UpdatedAt         int64  `json:"updatedAt"`
}

// AssignRequest 描述 Node Server 下发给 Client 的中心绑定结果。
//
// 职责边界：
// - 这里只接收 Node 已经决定好的 clientId/accountId；
// - 不负责中心 bind 判定，不负责账号权限判断，也不负责生成 clientId；
// - 该结构的存在是为了把 Node -> Client assign 协议固定住，避免后续再把字段口径写散。
type AssignRequest struct {
	ClientID   string `json:"clientId"`
	AccountID  string `json:"accountId"`
	Source     string `json:"source"`
	AssignedAt int64  `json:"assignedAt"`
}

// AssignResult 是 assign 接口成功后的本地写入结果。
//
// 设计来源：
// - 你要求 Client 把 Node 下发结果落到 JSON，并且联调时能直接知道写到了哪里；
// - 因此成功响应除了 registration 外，还要显式带出 written/cachePath，方便快速排查。
type AssignResult struct {
	Written      bool               `json:"written"`
	CachePath    string             `json:"cachePath"`
	Registration *RegistrationState `json:"registration"`
}

// StatusView 是给本机 HTTP API 返回的中心登记视图。
//
// 它只回显当前这台 Client 对中心暴露的入口摘要，以及本次实时查询到的 Node 结果，
// 方便联调时快速确认“为什么 Node 那边看见的是这个 baseUrl/clientIp”。
//
// 这里允许带出 JSON 文件里的本地缓存结果，但必须明确它只是“上次登记留痕”，
// 不能把它误当成当前有效中心身份。
type StatusView struct {
	Enabled            bool               `json:"enabled"`
	ConfigReady        bool               `json:"configReady"`
	ConfigMessage      string             `json:"configMessage"`
	NodeName           string             `json:"nodeName"`
	BaseURL            string             `json:"baseUrl"`
	ClientIP           string             `json:"clientIp"`
	DockerAPIURL       string             `json:"dockerApiUrl"`
	ServerBaseURL      string             `json:"serverBaseUrl"`
	MainAccountID      string             `json:"mainAccountId"`
	Registered         bool               `json:"registered"`
	LookupStatus       string             `json:"lookupStatus"`
	LookupMessage      string             `json:"lookupMessage"`
	CacheStatus        string             `json:"cacheStatus"`
	CacheMessage       string             `json:"cacheMessage"`
	CachedRegistration *RegistrationState `json:"cachedRegistration,omitempty"`
	Registration       *RegistrationState `json:"registration,omitempty"`
}
