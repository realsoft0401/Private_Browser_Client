# Private_Browser_Client

边缘服务，运行在单台设备上，只负责获取和管理本机 Docker / 浏览器运行环境。

当前已经明确：用户、Edge Client 列表、设备归属、设备编号、多节点调度等中心能力不再放在这里，后续应进入 `Private_Browser_Server`。

## 技术栈

- Go 1.22+
- Gin 1.10
- Viper 1.19（配置文件加载；dev/test/prod/docker 只表示读取哪份参数，不表示不同运行模式）
- SQLite（本机环境包索引与状态记录）
- 无前端、无桌面壳
- 当前阶段不使用 JWT；用户、节点和中控能力仍不放在边缘服务里

## 目录结构

```text
Private_Browser_Client/
├── main.go                              # 程序入口：找根目录 → 启动边缘服务
├── Settings/
│   ├── settings.go                      # 配置加载；运行口径统一为 production
│   ├── config-dev.yaml                  # 本地参数文件（运行模式仍是 production）
│   ├── config-test.yaml                 # 测试参数文件（运行模式仍是 production）
│   ├── config-prod.yaml                 # 默认参数文件（运行模式仍是 production）
│   └── config-docker.yaml               # 容器参数文件（运行模式仍是 production）
├── Infrastructures/
│   ├── Init.go                          # 服务启动总入口
│   └── SQLite/                          # 本机 SQLite 初始化与建表
├── Dao/
│   └── BrowserEnv/                      # 环境包创建索引的业务动作入口
├── Repository/
│   └── BrowserEnv/                      # browser_envs 表 SQL 访问
├── Routes/
│   └── Routes.go                        # Gin 路由注册
├── Models/
│   └── Edge/
│       └── edge.go                      # 本机设备与 Docker 状态模型
├── Service/
│   └── Edge/
│       ├── edge.go                      # 本机 Docker 2375 读取逻辑
│       └── http.go                      # Edge HTTP 处理器
├── Pkg/
│   └── HttpResponse/
│       ├── HttpResponse.go              # 统一响应封装
│       └── ResponseCode.go              # 统一状态码
├── docs/
│   └── openapi.yaml                     # Apifox 可导入的 OpenAPI 文档
└── .gitignore
```

## 职责边界

部署口径：

- 一台服务器部署一个 `Private_Browser_Client`。
- 多台服务器就部署多个 Client，各管各的本机 Docker 和环境包。
- “本机”是相对 Client 进程所在服务器而言，不是相对前端或 Server 调用方而言。
- Client 之间不互相发现、不互相调用、不维护其他服务器列表。
- 多服务器统一管理只能由 `Private_Browser_Server` 完成。
- Client 可以通过 UDP discovery/beacon 在独立内网里持续广播本机服务入口，帮助 Server 自动发现；这不代表 Client 维护Edge Client 列表或调用其它 Client。

`Private_Browser_Client` 只负责本机：

- 获取本机设备信息。
- 获取本机 Docker 状态。
- 后续管理本机 Docker 镜像、容器、浏览器实例。
- 后续向中心服务端上报心跳和状态。
- 对外只暴露 HTTP API，不把 SQLite、环境包目录或 `browser-data/profile` 当成可被 Server 直接读取的共享存储。
- 通过 UDP beacon 广播本机 Edge 服务入口和非敏感摘要，供 Server 自动发现和后续 HTTP 探测。

它不负责：

- 用户注册登录。
- JWT 鉴权。
- API Key、mTLS 等节点调用方鉴权。
- 校验 `userId` 对应用户是否存在、是否有权限、是否拥有某台服务器。
- 根据节点架构或商业策略选择浏览器运行镜像。
- 多Edge Client 列表。
- 设备归属关系。
- 设备编号。
- 多节点调度。

## 镜像职责

浏览器运行镜像选择归 `Private_Browser_Server` 或未来中心管理服务。

Server 负责：

- 探测节点架构并归一化为 `amd64`、`arm64`、`unknown`。
- 维护镜像策略和版本规则。
- 根据节点架构决定创建环境包时的 `runtime.image`。
- 在节点架构为 `unknown` 时阻止自动创建环境包。

Client 负责：

- 接收并保存环境包里的 `runtime.image`。
- 在本机 Docker 上执行 `/api/v1/edge/docker/pull-image`。
- run 时检查镜像是否存在，并用该镜像创建容器。
- 镜像不存在时返回明确错误，要求上游先拉取镜像。

维护原则：

- Client 不维护 ImagePolicy。
- Client 不根据本机架构自动替换镜像 tag。
- Client 不把 amd64/arm64 镜像互相猜测或兜底。
- `/api/v1/edge/docker/pull-image` 只是本机执行动作，拉哪个镜像由 Server 或受信管理方决定。
- 前端不应绕过 Server 直接决定商业运行镜像。

## userId 语义

`userId` 在 Client 中只是业务标识，没有权限含义。

它用于：

- 标识哪个平台用户在使用某个环境包。
- 生成环境包目录：`data/browser-envs/users/{userId}/{rpaType}/{envId}`。
- 生成 `envId` 和参与 `identityHash`。
- SQLite 列表过滤和后续中心服务聚合。

它不用于：

- Client 本机认证。
- Client 本机授权。
- 判断用户是否存在。
- 判断调用方是否有权使用该 `userId`。
- 判断用户拥有哪些服务器或设备。

维护原则：

- Client 可以校验 `userId` 格式和路径安全，但这只是数据卫生与文件安全校验，不是权限校验。
- 不要因为 Client 请求体里有 `userId`，就在 Client 中新增用户表、登录接口、JWT、RBAC 或设备归属判断。
- 平台后期应由客户中心或中心管理服务提供 API 登录；用户登录管理平台后，由管理平台统一管理所有 Client。
- Server 或上层平台访问环境包状态、下发动作和获取生命周期结果时，只能通过这些 API，不能直接读 SQLite、扫描环境包目录或 SSH 到节点翻登录态文件。
- 后期如需宿主机环境变量、服务日志或系统诊断数据，应新增 Client 受控诊断 API，做字段白名单、敏感值脱敏和访问留痕。

## 工具页面边界

Client 可以有内网工具页面，但不做业务前端。

允许保留：

- `/swagger`：接口文档和调试入口。
- `/openapi.yaml`：OpenAPI 原始文档。
- `/web-vnc.html?envId=...`：内网 WebVNC 工具页面。

不允许恢复：

- 旧 `public/index.html` 控制台。
- 旧 `public/app.js` 业务交互。
- 用户登录页。
- 节点管理页面。
- 环境包管理前端。
- 多节点 Dashboard。

维护原则：

- Swagger 和 WebVNC 是内网工具，不是产品业务入口。
- 真正业务前端应连接 `Private_Browser_Server` 或后续中心管理平台。
- Client 页面不承担用户体系、权限、节点归属、环境包调度和商业化交互。

## UDP 自动发现边界

Client 后期需要增加 UDP discovery/beacon 能力，用于在独立内网中让 Server 自动发现本机边缘服务。

职责边界：

- Client 只广播“本机 Edge 服务可连接”，不维护 Server 列表、不维护其它 Client 列表，也不主动调用其它 Client。
- UDP beacon 只用于发现服务入口，不承载业务动作，不返回环境包状态，不传用户、proxy 明文、fingerprint raw、Cookies、Local Storage、IndexedDB、Session Storage、Login Data 或备份包路径。
- UDP beacon 必须带平台识别字段，避免 Server 把内网里其它 UDP 报文抓进来。建议广播字段只包含非敏感摘要：discoveryMagic、protocolVersion、service、discoveryGroup、clientIp、hostname、baseUrl 或服务端口、clientVersion、startedAt、lastHeartbeatAt、capabilities。
- `discoveryMagic/service/discoveryGroup` 用来标识“这是当前私有浏览器平台、当前内网发现域的 beacon”；Server 发现不匹配时必须直接丢弃。
- Client 不再额外生成 `clientId`；在独立内网管理模式下，Client 的 IP 是 UDP discovery 的主要唯一识别来源，Server 结合 UDP 来源 IP、`baseUrl` 和 HTTP 探测结果进行去重。
- Client IP / `baseUrl` 只表示本机在独立内网中的接入地址；`clientId` 是 Server 落库后分配的中心身份，内部保存为 `edge_clients.id`。
- Client Edge 不生成、不保存、不要求 Edge API 请求携带 `clientId`；多 Client 身份、设备编号、权限和审计都归 Server 管理。
- 三层服务统一口径：商业设备唯一 ID 统一叫 `clientId`，由 Node Server 分配并维护，不是 Client Edge 自生成的 `device_unique_id`。
- Client 只负责暴露本机事实，例如 IP、baseUrl、hostname、os、arch、Docker 信息和健康检查。Node Server 探测确认后生成或绑定 `edge_clients.id`，并可在受控接口中下发或返回该 ID。
- 设备 IP/baseUrl 变化或设备重置时，Client 不自行创建新的商业设备 ID，也不修改中心 clientId 身份；Node Server 应标记 `ip_mismatch/identity_changed/manual_update_required`，由管理员确认后保持原 `edge_clients.id` 不变并更新接入地址。
- Client 侧只需确保本机 IP、`baseUrl`、端口和 hostname 能被 Server 探测到；如果 IP 变化，Client 不自行处理中心 clientId 身份，由 Server 标记 `identity_changed/ip_mismatch` 并提示管理员手动更新节点 IP。
- IP 不一致时，即使 Client 仍在线，Server 也不能自动覆盖原节点 IP 或创建新节点；管理员确认后，才把原 `clientId` 绑定到新的 clientIp/baseUrl。
- 管理员确认更新 IP 后，Server 保留原 `clientId`，只更新 `client_ip/base_url` 和健康摘要；历史任务、环境包聚合和审计记录仍绑定原 `clientId`。Client 不参与这些中心记录调整。
- IP 更新后，Server 会重新调用 Client `/health`、`/api/v1/edge/device-info` 和 Docker 探测确认设备事实；如果架构、Docker 环境、hostname 或环境包列表差异过大，Server 应继续要求管理员确认。
- beacon 端口、广播间隔、开关、网卡绑定地址应做成配置项，默认只在独立内网启用。
- Server 收到 beacon 后必须再通过 HTTP API 探测 `/health`、`/api/v1/edge/device-info` 或等价接口，确认服务可达和设备能力后才能登记节点。
- UDP 发现到的节点在 Server 侧只能先进入 `discovered`；`discovered` 不能创建环境包，也不能执行 run/stop/backup/restore/delete/import-package。
- Server 只有确认 UDP 报文属于当前平台和当前发现域、Client `/health` 可达、`/api/v1/edge/device-info` 可达、Docker 2375 可达、架构能归一化为 `amd64` 或 `arm64`、IP/baseUrl 没有不一致、没有 `identity_changed/stale/unhealthy/offline`，并且镜像策略可用后，才能把节点标记为 `verified`。
- Client 侧不决定自己是否 `verified`，只提供 beacon、健康检查和设备信息；最终 `discovered/verified/identity_changed` 状态由 Server 根据探测结果维护。
- Client `/health` 只返回本机视角 `healthy/unhealthy` 和 checks 明细，例如 API、SQLite、Docker、磁盘、配置文件和架构识别。`offline` 是 Server 访问不到 Client 后推导出来的状态，`stale` 是 Server 缓存不可信或短时无法确认时的状态，二者都不应该由 Client API 返回。
- Server 的心跳和探测阈值由 Server 配置控制，建议默认 `heartbeat_interval_seconds=15`、`stale_after_seconds=30`、`offline_after_seconds=90`、`failure_threshold=3`。Client 只负责按配置广播 beacon、响应 `/health` 和设备信息探测，不决定自己何时进入 `stale/offline`。
- 当 Server 从 `stale/offline` 恢复当前 Client 时，必须重新调用 `/health`、`/api/v1/edge/device-info` 和 Docker 探测；如果本机 checks 异常，Server 应标记 `unhealthy`，不应直接恢复 `healthy`。
- 如果未来进入共享内网或公网，UDP discovery 必须增加签名、预共享 token、mTLS 或等价节点鉴权；不能把当前内网明文 beacon 直接扩大到不可信网络。

## identityHash 语义

`identityHash` 只做环境包标识一致性摘要。

参与字段：

- `envId`
- `userId`
- `rpaType`

不参与字段：

- timezone、language、screen。
- proxy 配置和代理文件内容。
- browser-data 相对路径。
- 出口 IP、国家、地区、provider 结果、风险评分。
- envSequence、CDP/VNC 端口、containerId、containerName、Docker API、设备架构、节点 ID。

维护原则：

- `identityHash` 用来确认 `profile.json`、`binding.json`、目录名和 SQLite 是否仍指向同一个环境包。
- `binding.version`、`profile.updatedAt` 用来表达配置版本和更新时间。
- `runtimeProtection` 用来表达 timezone、代理出口、网络指纹验证状态和业务可用性结论。
- `proxyRuntime` 用来表达最近一次代理出口观测事实。
- 当前新项目不再生成或返回 `configHash`；配置变化通过 `binding.version`、`profile.updatedAt`、`runtimeProtection` 和 `proxyRuntime` 表达。
- timezone 或代理配置变化不能改变 `identityHash`，只能更新配置版本、更新时间和运行保护状态。
- 同一个用户可以有多个同平台环境，因此 `identityHash` 必须包含 `envId`，不能只用 `userId/rpaType`。

## runtimeProtection / proxyRuntime 语义

`proxyRuntime` 记观测事实，`runtimeProtection` 记业务结论。

`proxyRuntime` 适合记录：

- provider
- exitIp
- country / region
- timezone
- checkedAt
- success / failure
- error

`runtimeProtection` 适合记录：

- timezoneStatus
- riskStatus
- availabilityStatus
- lastVerifiedAt
- lastError

维护原则：

- `proxyRuntime` 回答“最近一次探测到了什么”。
- `runtimeProtection` 回答“这个环境现在能不能安全用”。
- 探测失败时，`proxyRuntime` 要保留失败细节，`runtimeProtection` 要明确标记 failed/timeout/pending/unavailable 或 highRisk。
- `proxyRuntime` 可以随每次探测更新；`runtimeProtection` 只表达当前可用性结论。
- 出口 IP、provider 原始结果、国家地区等观测事实不能进入 `identityHash`，也不能替代 `runtimeProtection` 的业务结论。

## 安全边界

`Private_Browser_Client` V1 明确采用内网管理模式，不在服务内实现鉴权。

部署前提：

- Client 运行在独立内网网段。
- Client 不直接暴露公网。
- 调用方是受信的中心服务、运维工具或本机管理进程。
- 用户认证、权限判断和对外访问控制由 `Private_Browser_Server` 或网络边界承担。

维护原则：

- 不要为了“补安全”把用户表、登录接口、JWT 或 RBAC 加回 Client。
- `/api/v1/edge/*` 可以操作 Docker、容器、环境包备份和删除，不能部署到不可信网络。
- 如果未来需要跨公网、跨客户网络、共享内网或调用方审计，再单独设计 Edge API Key、mTLS 或等价节点鉴权。
- 在引入节点鉴权之前，所有文档和部署脚本都应把 Client 描述为“内网受信边缘服务”，不能误导为公网安全 API。

## 调用链路

```text
main.go
  └→ detectProjectRoot()              // 从当前目录往上找 Settings/config-*.yaml
  └→ Infrastructures.Init(root)
       ├→ Settings.Init(root)         // 加载 config-{env}.yaml；env 只选择配置文件，mode 统一为 production
       ├→ SQLite.Init()               // 打开 data/private_browser_client.db 并建 browser_envs
       ├→ StartStatusSyncManager()    // 启动带哨兵的环境包状态同步任务
       ├→ ensurePortAvailable(port)   // 生产口径端口检查；占用时明确失败，不自动 kill
       ├→ Routes.Setup()              // 注册边缘服务路由
       ├→ http.Server.ListenAndServe  // 启动服务
       └→ waitForShutdownSignal()     // SIGINT/SIGTERM 优雅关闭
```

## 接口清单

### 服务自身

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/` | 服务信息 |
| GET | `/health` | 本机健康检查，只返回本机 `healthy/unhealthy` 和 checks 明细，不返回 Server 侧 `offline/stale` |
| GET | `/swagger` | Swagger UI 接口文档页面 |
| GET | `/openapi.yaml` | OpenAPI 原始 YAML |

### 边缘服务

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/edge/device-info` | 通过本机 Docker 2375 获取设备能力、Docker 版本、镜像数、容器数 |
| GET | `/api/v1/edge/docker/status` | 获取本机 Docker 可用性、镜像数量、容器数量 |
| GET | `/api/v1/edge/docker/images` | 获取本机 Docker 镜像列表 |
| GET | `/api/v1/edge/docker/containers` | 获取本项目相关 Docker 容器，只返回边缘服务容器和浏览器环境容器 |
| POST | `/api/v1/edge/docker/pull-image` | SSE 任务：拉取本机 Docker 镜像 |
| POST | `/api/v1/edge/docker/remove-image` | SSE 任务：删除本机 Docker 镜像 |
| POST | `/api/v1/edge/containers/:clientId/start` | SSE 任务：启动本机 Docker 容器 |
| POST | `/api/v1/edge/containers/:clientId/stop` | SSE 任务：停止本机 Docker 容器，请求体可为空 |
| POST | `/api/v1/edge/containers/:clientId/restart` | SSE 任务：重启本机 Docker 容器，请求体可为空 |
| GET | `/api/v1/edge/tasks/:taskId` | 查询 SSE 任务详情 |
| GET | `/api/v1/edge/tasks/:taskId/events` | SSE 事件流，订阅任务进度和最终结果 |
| GET | `/api/v1/edge/browser-envs` | 查询本机浏览器环境包索引列表，默认排除历史 deleted/归档记录 |
| POST | `/api/v1/edge/browser-envs` | 创建本地浏览器环境包文件，不启动 Docker |
| POST | `/api/v1/edge/browser-envs/import-package` | 上传标准 tar.gz 环境包并导入本机，保留 envId，重新分配本机端口 |
| GET | `/api/v1/edge/browser-envs/:envId` | 查询单个环境包详情，不返回代理明文和指纹 raw |
| POST | `/api/v1/edge/browser-envs/:envId/run` | SSE 任务：按环境包创建或启动本机浏览器容器 |
| POST | `/api/v1/edge/browser-envs/:envId/stop` | SSE 任务：按环境包停止本机浏览器容器，并同步运行态 |
| POST | `/api/v1/edge/browser-envs/:envId/backup` | 备份并清理环境包：生成 tar.gz 后删除本机容器和环境目录，SQLite 索引保留为 `backed_up` |
| POST | `/api/v1/edge/browser-envs/:envId/restore` | 从本机备份包恢复环境目录，恢复后可继续 run |
| POST | `/api/v1/edge/browser-envs/:envId/revalidate` | SSE 任务：管理员排查后重新校验 error 环境，只恢复到 created/stopped + runtimeProtection pending |
| DELETE | `/api/v1/edge/browser-envs/:envId` | SSE 任务：彻底删除环境包，删除配置目录、登录态目录、已停止容器和 SQLite 索引 |
| PATCH | `/api/v1/edge/browser-envs/:envId/proxy` | running 时返回 SSE 任务：修改环境包代理配置，变更后重建容器 |
| PATCH | `/api/v1/edge/browser-envs/:envId/proxy-mode` | running 时返回 SSE 任务：切换 Clash 规则/全局/直连模式并自动重建 |
| GET | `/api/v1/edge/browser-envs/:envId/cdp-test` | 基础 CDP 连通性诊断：测试 /json/version、target、WebSocket 和 Runtime.evaluate |
| GET | `/api/v1/edge/browser-envs/:envId/vnc-info` | 获取浏览器版 VNC 连接信息 |
| GET | `/api/v1/edge/browser-envs/:envId/vnc/ws` | noVNC WebSocket 到 VNC TCP 的代理通道 |
| GET | `/web-vnc.html?envId=...` | 独立浏览器 VNC 页面 |

## 生命周期入口

正常业务生命周期必须围绕环境包 `envId` 执行。

业务入口：

- `POST /api/v1/edge/browser-envs/:envId/run`
- `POST /api/v1/edge/browser-envs/:envId/stop`
- `POST /api/v1/edge/browser-envs/:envId/backup`
- `POST /api/v1/edge/browser-envs/:envId/restore`
- `POST /api/v1/edge/browser-envs/:envId/revalidate`
- `DELETE /api/v1/edge/browser-envs/:envId`

裸 Docker 容器接口只保留为内网运维诊断和异常兜底：

- `POST /api/v1/edge/containers/:clientId/start`
- `POST /api/v1/edge/containers/:clientId/stop`
- `POST /api/v1/edge/containers/:clientId/restart`

维护原则：

- Server、前端和自动化流程不应绕过环境包，直接把裸 `containerId` 当作业务主键。
- 裸容器接口只封装本机 Docker 动作，不读取环境包，不回写 `profile.json`、`container.json`、`binding.json`。
- 裸容器接口不保证 SQLite `browser_envs` 生命周期状态完整同步，状态最终应由环境包接口或后台状态同步收口。
- 备份、恢复、删除这类涉及 `browser-data/profile` 和环境包资产状态的动作，必须走 `browser-envs/:envId/*`。

## 资产动作语义

`backup`、`restore`、`import-package`、`revalidate`、`delete` 不能互相替代。

| 动作 | 语义 | 结果 |
|---|---|---|
| `POST /api/v1/edge/browser-envs/:envId/backup` | 归档并释放本机运行目录 | 生成 tar.gz，删除源环境包目录和已停止容器，SQLite 保留 `backed_up` |
| `POST /api/v1/edge/browser-envs/:envId/restore` | 从本机备份包恢复原环境 | 读取 SQLite `backup_path`，恢复环境包目录，恢复后可再次 run |
| `POST /api/v1/edge/browser-envs/import-package` | 上传外部标准包导入本机 | 保留 `envId`，重新分配本机序号和端口，不自动启动 |
| `POST /api/v1/edge/browser-envs/:envId/revalidate` | 异常环境重新校验 | 只校验原子材料、Docker 事实、端口和路径；不拉镜像、不启动容器，只恢复到 `created/stopped + pending` |
| `DELETE /api/v1/edge/browser-envs/:envId` | 彻底销毁环境资产 | 删除环境包目录、登录态目录、已停止容器和 SQLite 索引 |

维护原则：

- `backup` 不是普通“下载一份副本”，而是“归档资产并释放本机运行目录”；backup 后不能直接 run，必须先 restore。
- `restore` 只恢复本机已有备份包，不接收上传文件。
- `import-package` 只导入外部标准包，不读取本机 `backup_path`，也不自动启动容器。
- `revalidate` 只用于管理员排查后的重新准入，不替代 run/stop/backup/restore/delete，也不能把异常环境直接变成可用环境。
- `import-package`、`restore`、`revalidate` 都不自动拉镜像、不创建容器、不启动浏览器；`runtime.image` 缺失、非法或 image contract 不匹配时只能返回修复建议。
- `delete` 不是软删除，不保留可恢复资产；如果还想保留环境包，应先 backup，不要 delete。
- 当前商业运行口径下，环境包只能在同一台服务器恢复和运行；`import-package` 是受控导入能力，不代表 Server 可以把环境包自动跨服务器调度运行。
- 跨服务器导入可能改变宿主硬件指纹、CPU 架构、浏览器平台事实、镜像契约和网络环境，尤其 `amd64/x86_64` 与 `arm64/aarch64` 不能默认兼容。
- 后期如果要支持账号转移，必须先由 Server 或中心管理服务完成核心环境指纹比对；只有源/目标服务器被判定兼容时，才允许显式转移。
- 核心环境指纹比对至少应覆盖内部架构枚举、浏览器平台事实、image contract、Chromium 大版本、fingerprintEngineVersion、launchArgsVersion、WebRTC 策略、屏幕/语言/UA 兼容性、代理和网络指纹运行保护要求。
- Client 当前不负责做跨服务器兼容性判定；`import-package` 只校验包协议和本机导入规则，不代表账号转移风险已经通过。

导入规则：

- 导入必须保留 `envId/userId/rpaType`。
- 如果本机已存在相同 `envId`，直接返回冲突，不覆盖、不合并。
- 导入必须重新分配本机 `envSequence` 和 CDP/VNC 宿主端口。
- CDP/VNC 宿主端口必须结合当前服务器实际占用情况选择；如果默认规则端口已被正在运行的服务占用，应分配新的可用端口，不能和本机现有服务冲突。
- 导入必须重置 containerName/containerId/container_status/monitor_status/lastRuntime 等运行资源。
- 导入成功后不自动 run、不自动拉镜像、不自动创建容器。
- 下一次 run 必须重新执行当前服务器的网络指纹 / timezone / 代理出口验证。
- 端口和容器名重排不改变 `identityHash`。

## 登录态目录边界

`browser-data/profile` 是环境包核心资产，默认不能删除。

不能删除它的动作：

- `run`
- `stop`
- `forceRecreate`
- proxy update
- proxy-mode update
- image update
- timezone probe
- status sync
- container restart
- 为重建容器而删除旧 container

允许删除它的动作：

- `backup`：必须先生成并校验 tar.gz，成功后才释放源环境目录，SQLite 保留 `backed_up`。
- `DELETE /api/v1/edge/browser-envs/:envId`：彻底销毁环境资产，删除后不可恢复。

`clean-cache` 边界：

- 可以清理：Cache、Code Cache、GPUCache、ShaderCache、Crashpad、tmp、logs。
- 禁止清理：Cookies、Local Storage、IndexedDB、Session Storage、Login Data、Preferences、Extensions。

维护原则：

- 容器是运行态，可以删除和重建。
- `browser-data/profile` 是账号环境资产，不能随容器生命周期删除。
- 登录态真正载体是完整 Chromium profile，不是单独 Cookies 文件。
- 如果调用方想释放本机空间但保留资产，应使用 backup；只有确认彻底不要环境时才使用 delete。

## 状态字段语义

`browser_envs` 里的状态字段分工如下：

| 字段 | 含义 | 主要用途 |
|---|---|---|
| `status` | 环境包资产生命周期主状态 | 前端列表、Server 聚合、判断能否 run/backup/restore |
| `container_status` | Docker 容器事实快照 | Docker 调试、确认容器是否 running/exited/missing |
| `monitor_status` | 后台状态同步和监控健康摘要 | 判断同步任务是否正常、状态数据是否可信 |
| `last_error` | 最近一次动作或同步错误 | 排障说明 |

`status` 常见值：

- `created`：环境包已创建，尚未运行。
- `running`：环境包已通过业务流程启动成功。
- `stopped`：环境包已停止，可再次 run。
- `backed_up`：环境包已归档，本机运行目录已释放，必须 restore 后才能 run。
- `deleted`：历史兼容状态；当前 delete 更倾向物理删除目录和索引。
- `archived`：预留归档状态。
- `error`：最近一次生命周期动作失败。
- 如果环境包配置异常，`profile.json`、`binding.json`、`proxy/`、`fingerprint/`、`browser-data/profile` 等原子必需材料缺失、不可解析、关键字段非法或校验失败，或运行目录 missing 且不是受控 `backed_up`，即使节点健康，也不能继续执行普通 run/stop/backup/restore/import-package。
- `status=error` 不能靠手工改 SQLite 或下一次 `run` 隐式解除。管理员排查完成后必须调用 `revalidate` 或等价受控校验接口；校验通过后只恢复到 `created/stopped + runtimeProtection=pending`。
- 环境包异常时必须先完成受控诊断或配置修复，让配置重新通过校验后才能使用；Client 接口应把不可用原因写入 `last_error` 或诊断结果，不能静默继续生命周期动作。
- 配置修复只能通过 Client 受控接口执行，且只能修复索引摘要、缺失或过期的运行态字段、本机端口重新分配、container 运行摘要、非身份类配置格式问题，以及能从现有环境包文件一致推导出来的元数据。
- 配置修复不能改 `envId/userId/rpaType`、`identityHash`、`browser-data/profile` 登录态内容、proxy 明文来源、fingerprint raw、核心身份字段和 binding 身份字段，不能重建登录态或替换账号环境。修复后必须重新校验通过，才能恢复生命周期动作。

维护原则：

- `status` 管资产生命周期，不能被 Docker 快照随意覆盖。
- `container_status` 来自 Docker，例如 `unknown/created/running/exited/paused/restarting/removing/dead/missing`，只表示容器事实。
- `monitor_status` 回答“后台同步是否健康、状态是否可信”，不是浏览器运行状态。
- `backed_up` 时 `container_status=missing` 是正常结果，不是异常。
- 前端和 Server 展示用户可见主状态时优先使用 `status`。
- 运维排障时再结合 `container_status`、`monitor_status` 和 `last_error`。
- 对单个 Edge Client来说，Client 本地 `browser_envs` 是环境包资产事实源；Server 的 `server_browser_envs` 只是中心聚合缓存。Server 如果因为 Edge 失联、心跳超时或校验失败标记 `stale`，不代表 Client 本地环境包生命周期变成 `stale`。
- Server 在执行 `run/stop/backup/restore/delete/import-package` 前，应重新调用 Client API 校验当前状态，并以 Client 返回结果刷新中心缓存。
- Server 创建环境包时必须在中心侧明确指定目标 `clientId`；指定到当前这台 Client 的环境包，后续才会通过当前 Client 执行 run/stop/backup/restore/delete。
- Server 只有在该节点 `health_status=healthy` 且 `discovery_status=verified` 时，才应向当前 Client 下发创建和生命周期动作；`unhealthy/offline/stale/identity_changed/discovered` 都不能作为放行状态。
- 节点处于 `unhealthy` 时，Server 不应向当前 Client 下发任何环境包生命周期动作，包括 run、stop、backup、restore、delete、import-package；当前原则是节点不带病工作，先修复节点再操作环境包。
- V1 前期 Server 不应向 Client 下发批量生命周期动作；每个 run/stop/backup/restore/delete/import-package 都应对应一个明确环境包和一个独立任务。未来即使支持批量，也应由 Server 做容量评估、并发控制和逐个校验，Client 仍按单环境包动作执行。
- 当前没有定时自动生命周期调度需求，Client 不应实现定时 run、定时 stop、定时 backup、定时 delete 或无人值守自动恢复。Client 本机后台状态同步任务只同步 Docker 事实和运行态摘要，不能升级成生命周期调度器。
- 如果 Server 因节点状态不健康、架构 unknown、Docker 不可达或镜像策略不可用而拒绝动作，Client 不需要自行补做中心准入判断。
- Client 不感知也不校验 `clientId`，只处理发送到本机 Edge API 的环境包请求；Server 负责维护 `server_browser_envs.client_id` 与当前 Client IP/baseUrl 的绑定关系，对外统一叫 clientId。

## 连接地址语义

Client 返回的 CDP / VNC / WebVNC 地址都是独立内网中的 Edge 连接地址。

适用对象：

- `Private_Browser_Server`
- 内网管理端
- 运维工具
- 本机或同网段调试程序

字段语义：

- `cdpUrl`：Edge 内网可访问的浏览器 CDP 地址。
- `vncUrl`：Edge 内网可访问的原生 VNC 地址，主要用于排障。
- `vncWsUrl`：Edge 内网可访问的 noVNC WebSocket 代理地址。
- `webVncUrl`：Edge 内网可访问的浏览器版 VNC 页面。

维护原则：

- 这些地址用于 Server 或内网管理端监控、连接容器设备。
- 这些地址不是公网地址，也不是外部客户浏览器的最终访问地址。
- Client 不负责生成跨公网或跨客户网络可访问 URL。
- 如果后续外部客户前端需要访问浏览器画面，应由 Server、网关或反向代理重新包装地址。
- 在当前独立内网管理模式下，Server 可以直接使用和展示 Edge 返回的内网连接地址。

## 运行可用性

容器 `running` 不等于环境可用。

设计原则：

- 浏览器服务、代理出口、timezone 和网络指纹是一个原子环境。
- Docker 容器可以启动成功，但如果浏览器服务、CDP、代理出口、timezone 或网络指纹验证失败/超时，本次 `run` 必须失败。
- 这种情况下 `browser_envs.status` 应进入 `error` 或等价待排查状态；`container_status=running` 只作为 Docker 事实保留，供 VNC、日志和管理员诊断使用。
- 响应和详情必须保留 `timezoneStatus/timezoneError/availabilityStatus/last_error` 或等价运行保护结果，供 Server 和管理端提示风险。

展示规则：

- `container_status=running` 只表示 Docker 容器在运行。
- `status=running` 必须表示业务 `run` 已经完成容器、浏览器服务、代理出口、timezone 和网络指纹保护验证。
- `timezoneStatus=verified` 或等价网络指纹保护成功，才表示该环境具备正常业务使用前提。
- `timezoneStatus=failed/timeout/pending` 时，即使容器 running，也应进入 `error` 或等价待排查状态，并提示“网络指纹未确认，环境不可用或高风险，需要管理员排查”。
- 代理配置修改同样遵守这条原子性：配置落盘、配置版本更新、容器重建、timezone/代理出口验证都不是最终可用；只有运行态完成网络指纹确认后，环境才可标记为可用。`identityHash` 只做 `envId/userId/rpaType` 一致性摘要，不因代理配置变化而改变。

维护原则：

- 不要在 timezone/代理出口探测失败时静默沿用旧 timezone 当作成功。
- 不要让前端或 Server 只根据 `running` 判断环境可用。
- 后续如果新增 `usable/available/riskStatus` 字段，应把网络指纹保护结果纳入计算。
- 网络指纹失败属于业务不可用，不只是普通日志告警。

## 配置

```yaml
docker:
  api_url: http://127.0.0.1:2375
status_sync:
  enabled: true
  interval_seconds: 5
  watchdog_seconds: 15
  stale_seconds: 30
discovery:
  enabled: true
  broadcast_address: 255.255.255.255
  port: 43000
  interval_seconds: 5
  magic: PRIVATE_BROWSER_CLIENT_DISCOVERY
  protocol_version: 1
  group: default
  advertise_host: ""
  advertise_base_url: ""
```

`docker.api_url` 是边缘服务访问本机 Docker Engine 的地址。当前默认使用 Docker HTTP 2375。

Docker API 边界：

- `docker.api_url` 只用于本机或独立内网网段的 Docker 管理。
- Docker 2375 不能暴露公网或不可信网络。
- Client 不通过 SSH 管理远端 Docker，也不是通用 Docker 管理后台。
- Client 只操作当前服务器本机 Docker；多服务器统一管理由 Server 调度多个 Client 完成。
- Docker API 不可达、超时、权限不足或 Docker daemon 未开启时，相关接口必须明确失败并说明修复方向，不能静默降级或假装成功。

Server 访问 Client 的边界：

- Server 只能通过 Client HTTP API 获取环境包状态、创建环境包、启动、停止、备份、恢复、导入和删除。
- SQLite 是 Client 本机索引与运行摘要，不是 Server 可直接连接或挂载读取的数据库。
- `data/browser-envs`、备份包目录和 `browser-data/profile` 是 Edge 本地资产目录，不是 Server 的共享文件源。
- Server 不通过 SSH 到 Client 节点绕过 API 翻环境包文件、修配置、搬运环境包或读取登录态；需要排障、修复、重建索引时，应新增或调用 Client 受控 API。
- 后期如果确实需要采集宿主机环境变量、Docker/系统诊断、服务日志或部署状态，应作为单独的受控诊断能力设计；Client 负责本机采集、白名单过滤、敏感值脱敏，Server 只接收诊断结果。
- SSH 可以作为独立运维、部署或救援通道存在，但不能成为环境包业务数据源，也不能用于读取 SQLite、`browser-data/profile`、代理明文、指纹 raw 或登录态文件。
- 这样做是为了让环境包生命周期、SQLite 索引、登录态目录和 Docker 运行态仍由 Client 保持本地一致性，避免中心服务绕过边缘保护后形成双事实源。

`status_sync` 是浏览器环境包后台状态同步任务：Worker 每隔几秒按 Docker 真实容器状态刷新 `browser_envs` 运行态摘要，Watchdog 监控 Worker 心跳，异常退出或长时间无心跳时自动拉起。

职责边界：

- 可以读取 SQLite 索引、Docker 容器列表、Docker labels、containerId、containerName 和容器状态。
- 可以写 SQLite 的 `container_status`、`monitor_status`、`last_checked_at`、`last_error`、`last_started_at`、`last_stopped_at` 等运行态摘要。
- 可以对 `status` 做保守收口，但不能把 `backed_up/deleted/archived` 改回 `running/stopped`。
- Docker 容器 missing 或 Docker API 不可达时，只能记录事实和错误，不能删除环境包索引或目录。

禁止事项：

- 不自动启动、停止、删除或重建浏览器容器。
- 不删除、创建或重建 `browser-data/profile`。
- 不改写 `profile.json`、`binding.json`、`container.json`、`proxy/clash.yaml`、`fingerprint/*`。
- 不修改代理、指纹、端口、identityHash、binding.version、runtimeProtection 或 proxyRuntime。
- 不替代 run/stop/backup/restore/delete/proxy update 等生命周期接口。

`discovery` 是 UDP 自动发现广播配置。Client 会按间隔向独立内网广播一帧 JSON，包含 `discoveryMagic`、`protocolVersion`、`service`、`discoveryGroup`、`clientIp`、`baseUrl`、`hostname`、`version`、`startedAt`、`lastHeartbeatAt` 和 `capabilities`。这些字段只服务 Server 自动发现和 HTTP 二次探测，不是鉴权，也不是中心 clientId 身份。`advertise_host` 和 `advertise_base_url` 用于多网卡、容器网络或反向代理场景下手动指定 Server 应访问的内网地址。

## 响应格式

```json
{
  "code": 1000,
  "message": "success",
  "data": {}
}
```

### 状态码

| code | 含义 |
|---|---|
| 1000 | 成功 |
| 1001 | 请求参数错误 |
| 1002 | 数据不存在 |
| 1003 | 数据状态冲突 |
| 1004 | Docker API 调用失败 |
| 1005 | 服务繁忙 |

## 运行

```bash
cd /Users/lining/Documents/Browser_virtualization/Private_Browser_Client
go run .

curl http://127.0.0.1:3300/health
curl http://127.0.0.1:3300/api/v1/edge/device-info
curl http://127.0.0.1:3300/api/v1/edge/docker/status
curl http://127.0.0.1:3300/api/v1/edge/docker/images
curl http://127.0.0.1:3300/api/v1/edge/docker/containers
```

拉取镜像示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/docker/pull-image \
  -H 'Content-Type: application/json' \
  -d '{"image":"crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-arm64"}'
```

删除镜像示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/docker/remove-image \
  -H 'Content-Type: application/json' \
  -d '{"image":"crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-arm64","force":false,"noPrune":false}'
```

容器生命周期示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/containers/{container_id}/start

curl -X POST http://127.0.0.1:3300/api/v1/edge/containers/{container_id}/stop \
  -H 'Content-Type: application/json' \
  -d '{"timeoutSeconds":10}'

curl -X POST http://127.0.0.1:3300/api/v1/edge/containers/{container_id}/restart \
  -H 'Content-Type: application/json' \
  -d '{"timeoutSeconds":10}'
```

创建浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs \
  -H 'Content-Type: application/json' \
  -d '{
    "userId":"318275706305908736",
    "rpaType":"tk",
    "name":"tk-browser-001",
    "runtime":{
      "image":"crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-arm64"
    },
    "environment":{
      "timezone":"America/Toronto",
      "language":"en-US",
      "screen":{"width":1366,"height":768}
    },
    "proxy":{
      "enabled":true,
      "type":"clash-verge",
      "mode":"rule",
      "configBase64":"bW9kZTogcnVsZQptaXhlZC1wb3J0OiA3ODk3Cg=="
    },
    "metadata":{"description":"TikTok browser env"}
  }'
```

该接口只写入 `data/browser-envs/users/{userId}/{rpaType}/{envId}` 环境包，不创建或启动 Docker 容器。
创建成功后会同步写入 `browser_envs` SQLite 索引表，用于后续列表、运行状态和监控状态查询。
创建阶段不会由 Go 边缘服务直接请求 IP 定位网站来最终确认 `timezone`。代理出口、DNS、TUN 和浏览器真实网络环境只在浏览器容器内成立，因此最终 `timezone` 必须在后续 `run` 阶段由容器内探测确认。创建请求里的 `environment.timezone` 只能作为初始值；后续容器内探测成功后，会以探测结果回写 `profile.environment.timezone`，并更新 `runtimeProtection/proxyRuntime`。`identityHash` 只做 `envId/userId/rpaType` 一致性摘要，不因 timezone 变化而改变。
创建时可以通过 `proxy.mode` 指定 Clash 顶层模式，支持 `rule/global/direct`；如果不传，则保留 `configBase64` 里原有的 `mode`。

查询浏览器环境包列表示例：

```bash
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs?page=1&pageSize=20'
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs?userId=318275706305908736&rpaType=tk&status=created'
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs?status=running'
```

当列表项 `status=running` 时，响应 item 会额外包含 `vncUrl`、`vncWsUrl`、`webVncUrl`，前端可以直接用 `webVncUrl` 打开浏览器 VNC 页面。

查询单个浏览器环境包详情示例：

```bash
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}'
```

详情接口会返回 `profile`、`binding`、`container`、`proxy` 摘要、`fingerprint` 摘要和一致性检查结果。
它不会返回 `proxy/clash.yaml` 明文，也不会返回 fingerprint raw；后续重新配置代理会使用独立修改接口。

启动浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/run \
  -H 'Content-Type: application/json' \
  -d '{"forceRecreate":false}'
```

`run` 接口只接受 envId 和 `forceRecreate`，镜像、端口、代理、指纹和浏览器数据挂载都从环境包文件读取。
如果镜像未提前拉取，会返回明确错误，调用方应先执行 `/api/v1/edge/docker/pull-image`。
后续 timezone 确认必须作为 `run` 生命周期的一部分执行：容器启动后，在浏览器容器内按顺序请求下面三个出口识别服务，只要任意一个返回可解析 `timezone` 即认为成功：

```text
1. https://ipwho.is
2. http://ip-api.com/json
3. https://ipapi.co/json/
```

成功后需要记录 provider、出口 IP、国家/地区、timezone 和 checkedAt，并把 timezone 回写到 profile，同时把 `runtimeProtection` 标记为可用。全部失败或超过探测预算时，应记录每个 provider 的失败原因，把 `proxy-runtime.status`、`runtimeProtection.timezoneStatus`、`riskStatus` 和 `availabilityStatus` 标记为失败/高风险/不可用；容器可以保持 running，但本次 run 必须明确失败，不能让调用方误以为环境可用。这个请求不能由 Go 边缘服务宿主机直连完成，也不能由前端代替完成。

代理启用时不能在容器刚启动后立刻取 timezone，因为 Clash/TUN/DNS 可能还没有完全接管，早期请求可能走直连出口。当前 run 流程会先等待容器内 Clash/Mihomo 进程出现并给代理链路一段初始化时间，再按 `proxy/clash.yaml` 顶层 `mode` 选择探测入口：

```text
mode: rule
  使用浏览器 CDP 页面访问 provider。
  页面导航后等待 10 秒再读取响应并关闭临时页面，确保域名规则、浏览器链路和页面网络行为参与判断。

mode: global / direct
  使用容器 shell 的 curl/wget 探测。
  curl 会读取 mixed-port，并显式使用 curl -x http://127.0.0.1:{mixed-port} 进入 Clash。
```

rule 模式不再把 curl 作为自动兜底；global/direct 模式也不走 CDP。这样 timezone 结果和当前 Clash 模式一一对应，避免排障时混淆“浏览器规则链路”和“容器命令行链路”。整个 timezone probe 有固定时间预算，避免外部 provider 或 CDP 长时间无响应导致接口 `socket hang up`。如果探测到的 timezone 和容器启动时的 `TZ` 不一致，后端会先写回 profile/binding，然后重建浏览器容器让新 `TZ` 生效；重建后的容器不再同步发起第二轮 provider/CDP 请求，避免接口等待时间翻倍。

如果 `proxy/clash.yaml` 启用了 `tun.enable=true`，Go 边缘服务会在创建浏览器容器前做 TUN 能力检查：

```text
检查 Edge Client 容器内是否能看到 /dev/net/tun
  不存在或不是设备：
    run 失败
    返回明确修复建议，不能静默降级
  存在：
    Docker create 自动追加 CapAdd: ["NET_ADMIN"]
    Docker create 自动挂载 /dev/net/tun:/dev/net/tun
```

因为系统环境指纹和网络环境指纹是原子关系，`tun.enable=true` 不能在运行时偷偷变成 `false`。本地 Mac / Docker Desktop 如果没有可用 TUN，应在提交代理配置前生成 `tun.enable=false` 的测试配置；商用 Linux 节点如果需要完整 TUN/DNS 保护，必须确保宿主机和 Edge Client 容器都能看到 `/dev/net/tun`。

这里要区分“CPU 架构”和“宿主机 TUN 能力”：

```text
Mac / Docker Desktop
  常见问题是 Docker Desktop VM 不一定暴露可挂载的 /dev/net/tun。
  即使浏览器镜像是 arm64，也可能不能跑完整 TUN。
  本地测试应使用 scripts/clash_tun_false_for_mac.sh 生成 tun.enable=false 配置。

Linux / Ubuntu x86_64 / amd64
  通常可以支持完整 TUN/DNS。
  前提是宿主机存在 /dev/net/tun，并且 Edge Client 容器启动时带：
    --cap-add NET_ADMIN --device /dev/net/tun:/dev/net/tun
  如果宿主机 /dev/net/tun 不存在，先执行 sudo modprobe tun。
```

配置生成脚本：

```bash
# Mac / Docker Desktop 本地界面测试：生成 tun.enable=false，不改原文件。
scripts/clash_tun_false_for_mac.sh \
  /Users/lining/Documents/analysis_ins/proxy/ClashVerge_1.yaml \
  .tmp/ClashVerge_1.mac.yaml

# x86 Linux 商用节点：生成 tun.enable=true，并提示检查 /dev/net/tun。
scripts/clash_tun_true_for_linux.sh \
  /Users/lining/Documents/analysis_ins/proxy/ClashVerge_1.yaml \
  .tmp/ClashVerge_1.linux.yaml
```

所以 Ubuntu amd64 商用节点要跑完整 TUN，不只需要宿主机有 `/dev/net/tun`，还要把这个设备和 `NET_ADMIN` 能力传给 Edge Client 容器；否则 Client 在创建浏览器容器前会明确失败，避免网络指纹被静默改写。

停止浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/stop \
  -H 'Content-Type: application/json' \
  -d '{"timeoutSeconds":10}'
```

`stop` 接口围绕 envId 停止容器，并回写 `container.json`、`profile.lastRuntime` 和 SQLite `browser_envs` 运行态。
它不会删除容器、镜像或 `browser-data/profile` 登录态目录。

### 环境包备份、下载与恢复流程

最新流程里，核心能力重新定义为 `备份` 和 `下载`，不再把 `导出` 作为独立生命周期。原因是备份产物本身就是标准 `.tar.gz` 环境包，用户把这个包拿走时，它自然具备迁移能力；系统真正需要管理的是环境包是否还在本机可运行、是否已经只剩备份文件、以及下次 RPA 前如何恢复。

新的原则是：

```text
备份是状态变化动作：封存当前最新环境，释放运行资源。
下载不是状态变化动作：只是复制已有备份包给用户。
SQLite 索引不是运行目录索引，而是环境资产索引；备份后不能直接删除。
```

备份后的本机状态应变成：

```text
Docker 容器：删除
browser-envs 源环境目录：删除
备份 tar.gz：保留在受控备份目录
SQLite browser_envs 索引：保留，状态改为 backed_up/archived
```

SQLite 索引保留的原因很重要：前端仍然需要看到这个环境资产，用户也需要从列表里选择“恢复并运行”“下载备份包”“删除备份”。如果备份后直接删除索引，系统就失去了 envId、账号绑定、备份包路径、checksum、备份时间和后续恢复入口；用户下次执行 RPA 时只能手工上传文件，无法形成稳定的自动化生命周期。

但索引保留并不代表环境仍可直接运行。后续代码必须避免保留“假的可运行索引”：当环境目录已经删除时，`status` 不能仍是 `created/stopped/running`，而应转为 `backed_up` 或 `archived`。这个状态下前端不能直接显示“启动”，只能显示“恢复”“恢复并运行”“下载备份包”“删除备份”。

建议后续在索引里补充这些备份字段：

```text
status = backed_up / archived
backupPath
backupChecksum
backupSize
backupAt
backupVersion
lastRestoreAt
lastRunAt
```

备份流程：

```text
停止环境包
  -> 确认 Docker 容器不在 running
  -> 复制当前环境包到 staging
  -> 写入 backup metadata 和 checksums
  -> 生成 tar.gz
  -> 校验 tar.gz 可恢复
  -> 把 tar.gz 放入受控备份目录
  -> 删除关联的已停止 Docker 容器
  -> 删除 browser-envs 下源环境目录
  -> SQLite browser_envs 索引保留，状态改为 backed_up/archived
  -> 写入 backupPath、checksum、size、backupAt
```

项目仍处于开发期，不保留旧的临时下载流接口，避免“备份”和“下载”语义混在一起。代码统一到“备份包”模型：每次 RPA 执行结束后备份当前最新状态，只保留备份文件和 SQLite 资产索引；下一次执行 RPA 时先恢复环境包，再启动容器。

备份示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/backup
```

当前实现会把备份包放在 `data/browser-envs/users/{userId}/{rpaType}/{envId}-backup.tar.gz`，并在包校验成功后删除源环境包目录和关联的已停止 Docker 容器。它不会删除浏览器镜像，也不会删除 SQLite 资产索引；索引会改为 `backed_up`，并记录备份包位置和校验信息。如果环境包仍在运行中，接口会返回状态冲突，调用方应先执行 `stop`。

下载备份包不应该再叫导出。下载只是把已有备份文件复制给用户，不改变状态、不删除索引、不删除备份包、不影响后续恢复：

```text
backed_up/archived
  -> download backup package
  -> 状态仍然是 backed_up/archived
```

当前版本还没有公开 `backup/download` HTTP 接口。备份接口会把受控备份包路径、checksum、大小和备份时间写入 SQLite，下载能力应在后续 artifact/download 方案里补齐，不能恢复旧的“临时打包下载流”语义。

恢复示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/restore
```

`restore` 会从 SQLite 索引里的 `backupPath` 读取本机备份包，校验 checksum 后恢复 `browser-envs/{envId}` 目录，并把容器运行态重置为 `created`。它不会自动启动 Docker；恢复成功后会删除本机备份 tar 并清空 SQLite backup 字段，下一步再由调用方显式调用 `run`。

当前已落地接口：

```text
POST /api/v1/edge/browser-envs/{envId}/backup
POST /api/v1/edge/browser-envs/{envId}/restore
```

历史 `backup-package` 和 `export-and-remove` 接口语义已经被移除。后续如果要做下载，只能读取 `backup` 生成并登记过的备份包；如果要释放或删除备份资产，必须走独立删除动作，不能在下载接口里附带删除。

这里需要特别注意下载类接口的删除时机：下载失败不应影响备份状态。后续更稳的实现方向是 artifact 两阶段：

```text
POST backup task
  -> 后端生成并校验 artifact
  -> 删除容器和源环境目录
  -> SQLite 索引转为 backed_up/archived
  -> 返回 artifactUrl

GET artifactUrl
  -> 下载 tar.gz
  -> 不改变 backed_up/archived 状态
```

在 artifact 方案完成前，前端必须对备份做强提示：环境包包含登录态、代理凭据和指纹配置；备份后当前节点将不再保留可运行环境目录，后续需要使用时必须从备份恢复。

导入浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/import-package \
  -F "file=@{envId}-backup.tar.gz"
```

`import-package` 只接受本服务备份生成的标准 `.tar.gz` 包，或者同格式的外部备份包。
导入会校验单根目录、`profile.json`、`binding.json`、`proxy/`、`fingerprint/`、`browser-data/profile` 等原子必需材料和 checksums；默认保留原 `envId`，如果本机已存在同名环境包会拒绝覆盖。
导入到本机后会重新分配 `envSequence`、CDP/VNC 端口，并把容器运行态重置为 `created`；下一次 `run` 会重新在浏览器容器内探测 timezone。

重建 SQLite 索引示例：

```bash
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs-rebuild/candidates'

curl -X POST 'http://127.0.0.1:3300/api/v1/edge/browser-envs-rebuild/{envId}'
```

`rebuild-candidates` 只读扫描 `data/browser-envs/users` 下的环境包目录，返回 `created_atomic`、`verified_atomic` 或 `invalid`，不会写 SQLite，也不会修复文件。
`rebuild-index` 一次只处理一个 `envId`，只有原子材料完整、身份 hash 一致、路径合法、Docker 不存在同 envId 或同 containerName 冲突时才会写入 SQLite。该接口不启动 Docker、不拉镜像、不创建容器；端口会根据本机占用情况重新确认。

彻底删除浏览器环境包示例：

```bash
curl -X DELETE http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}
```

删除接口会物理删除环境包目录，包括 `profile.json`、`binding.json`、`proxy/`、`fingerprint/` 和 `browser-data/profile`，同时删除关联的已停止 Docker 容器并移除 SQLite `browser_envs` 索引记录。
它不会删除浏览器运行镜像，也不会自动停止正在运行的容器。
该操作无法通过 `rebuild-index` 找回，前端必须在调用前提示用户谨慎操作、删除后无法恢复。
如果环境包仍在运行中，接口会返回状态冲突，调用方应先执行 `stop`。

修改代理配置示例：

```bash
curl -X PATCH http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/proxy \
  -H 'Content-Type: application/json' \
  -d '{
    "enabled": true,
    "type": "clash-verge",
    "mode": "global",
    "configBase64": "bW9kZTogcnVsZQptaXhlZC1wb3J0OiA3ODk3Cg=="
  }'
```

`configBase64` 是代理 YAML 原文的 Base64 编码，例如：

```bash
base64 -i clash.yaml | tr -d '\n'
```

macOS 生成单行 Base64：

```bash
CONFIG_B64=$(base64 -i clash.yaml | tr -d '\n')
```

Linux 生成单行 Base64：

```bash
CONFIG_B64=$(base64 -w 0 clash.yaml)
```

完整调用示例：

```bash
CONFIG_B64=$(base64 -i clash.yaml | tr -d '\n')

curl -X PATCH http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/proxy \
  -H 'Content-Type: application/json' \
  -d "{\"enabled\":true,\"type\":\"clash-verge\",\"mode\":\"rule\",\"configBase64\":\"$CONFIG_B64\"}"
```

`configBase64` 必须来自一整份合法 YAML 原文。不要把两份 YAML 拼接在一起；例如 `- MATCH,relay` 后面又直接接 `mode: rule`，会导致代理配置语义错误。
Base64 长度通常比 YAML 原文更长，约等于 `4 * ceil(原文字节数 / 3)`，真实代理配置生成几 KB 到几十 KB 的单行字符串都正常。
PATCH 代理配置也可以通过 `mode` 同时切换 Clash 顶层模式。后端会先解码 `configBase64`，再把 `mode` 写入 YAML 顶层；如果只传 `mode` 不传 `configBase64`，则只修改现有 `proxy/clash.yaml` 的顶层 `mode`。
`PATCH proxy` 不负责修改 `profile.runtime.image`，前端也不应绕过 Server 直接传镜像字符串。镜像变更必须走 Server 或管理员受控镜像策略流程，并校验节点架构、`runtime.image` 和 image contract；不允许通过代理配置接口顺手切换镜像。

代理配置不是热更新。只要配置实际发生变化，就必须通过重建容器让配置生效。
如果环境包正在 `running`，接口会先完成配置落盘、递增 `binding.version` 并重置运行保护状态，然后立即返回 `restartQueued=true`；后端会在后台串行执行 `forceRecreate` 重建容器，前端不需要再单独调用 `stop/run`。
这样 rule 模式下 CDP/timezone provider 即使耗时较长，也不会拖断本次 PATCH 请求。
这里要特别注意：异步的是 `running` 环境的容器重建任务，不是 `rule` 模式本身。`rule/global/direct` 在 running 环境下都会快速返回并进入后台重建；区别只在后台 timezone probe 的入口不同，`rule` 走 CDP，`global/direct` 走容器内 curl/wget。
如果环境包不是运行态，响应会返回 `restartRequired=true`，表示下一次 `run` 时生效。
代理配置发生变化时，该接口只递增 `binding.version` 并把 `runtimeProtection` 重置为 pending；`identityHash` 只做 `envId/userId/rpaType` 一致性摘要，不因代理配置变化而改变。该接口不会删除 `browser-data/profile`。
代理变化后 timezone 也必须重新确认。规则如下：

```text
running 环境：
  PATCH proxy -> 写入新代理 -> 返回 restartQueued=true + taskId + eventsUrl -> 后台 forceRecreate -> 容器内多源 timezone probe。
  如果 timezone 成功，回写 timezone 和运行保护状态；如果超时或失败，在详情 proxy.runtime/runtimeProtection 里保留 failed/unavailable 记录。

非 running 环境：
  PATCH proxy -> 写入新代理 -> 标记下次 run 生效；代理变化时重新确认 timezone -> 返回 restartRequired=true。
```

### SSE 任务接口

下面这些接口已经改为 SSE 任务化：HTTP 请求只负责创建任务并立即返回 `taskId/eventsUrl`，真实执行结果通过 `/api/v1/edge/tasks/{taskId}/events` 推送。

```text
POST   /api/v1/edge/docker/pull-image
POST   /api/v1/edge/docker/remove-image
POST   /api/v1/edge/containers/:clientId/start
POST   /api/v1/edge/containers/:clientId/stop
POST   /api/v1/edge/containers/:clientId/restart
POST   /api/v1/edge/browser-envs/:envId/run
POST   /api/v1/edge/browser-envs/:envId/stop
DELETE /api/v1/edge/browser-envs/:envId
```

`PATCH /api/v1/edge/browser-envs/:envId/proxy` 和 `PATCH /api/v1/edge/browser-envs/:envId/proxy-mode` 是条件任务化：只有环境包正在 `running` 且配置实际变化时，才会返回 `restartQueued=true + taskId + eventsUrl`；非运行态只标记下次 run 生效。

Client 任务定位：

- Client task 是边缘节点本机短期执行观察，不做长期持久化。
- 任务数据保存在当前 Client 进程内，主要用于 SSE 实时进度、Docker pull 过程、run/stop/backup/delete 阶段和内网排障。
- Client 服务重启后历史任务不会恢复，也不作为用户审计、SLA、计费或跨节点聚合依据。
- Server 如果调用 Client 任务接口，应把 Edge `taskId` 绑定到 Server 自己的持久任务记录。
- 平台级任务事实以 `Private_Browser_Server` 的任务表为准。
- 如果 Client 重启、SSE 中断或 Edge `taskId` 查不到，Server 必须重新调用当前环境包状态接口校验事实后收敛 Server task。Server task 终态只有 `success/failed`：能确认动作完成才记 `success`，无法确认、状态冲突、Client 失联、配置异常或资产动作不可信时统一记 `failed` 并写清原因。
- Client 不负责恢复历史 task，也不负责判断平台级任务是否成功；backup/restore/delete/import-package 等资产动作不能因为 Edge task 丢失而被 Server 自动重放。
- 所有任务失败后都不自动重试，包括 run、stop、backup、restore、delete、import-package、proxy update、proxy-mode update 和 pull-image。失败后需要先修复节点、网络指纹、代理、镜像、端口或环境包配置，再由用户或管理员重新发起新任务。

备份、恢复、导入当前仍是文件类接口，不应按普通同步接口长期保留：

```text
POST /api/v1/edge/browser-envs/:envId/backup
POST /api/v1/edge/browser-envs/:envId/restore
POST /api/v1/edge/browser-envs/import-package
```

原因是备份生成包后会删除当前节点上的源环境包和已停止容器，但 SQLite 索引会保留为 `backed_up/archived` 资产状态。后续下载只应读取已有备份包，不改变状态；导入/恢复再把备份包还原成可运行环境。

后续实现应优先改成 artifact 任务模型：HTTP 请求只创建备份任务，SSE 负责报告打包、校验、删除容器、删除源环境目录、更新 SQLite 资产状态的进度，任务完成后返回 `artifactUrl` 或备份包下载地址。`import-package` 是上传恢复动作，可以先继续保持 multipart 同步接口；如果后续导入包变大或需要显示校验进度，再单独任务化。

任务化接口响应示例：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "task_1770000000000000000_12345",
    "taskType": "browser_env_run",
    "status": "queued",
    "resourceType": "browser_env",
    "resourceId": "318275706305908736_tk_318578131780767744",
    "eventsUrl": "http://127.0.0.1:3300/api/v1/edge/tasks/task_1770000000000000000_12345/events",
    "message": "浏览器环境启动任务已创建"
  }
}
```

前端拿到 `eventsUrl` 后打开 SSE：

```bash
curl -N "http://127.0.0.1:3300/api/v1/edge/tasks/{taskId}/events"
```

第一版事件会包含：

```text
queued    任务已创建
running   后台动作开始执行
progress  Docker pull 等动作的中间进度
heartbeat 长动作仍在执行
done      任务成功，最终结果在事件 data.result 中
error     任务失败，失败原因在 message 中
```

任务数据保存在当前 Client 进程内，主要用于实时观察和排障；服务重启后历史任务不会恢复。前端如果刷新页面，可以先请求 `GET /api/v1/edge/tasks/{taskId}` 读取当前进程内的任务摘要，再决定是否继续订阅 SSE。需要长期查询、审计和跨节点聚合的任务，应由 Server 任务表持久化。

容器内 timezone probe 的 provider 解析规则：

```text
ipwho.is:
  timezone 取 response.timezone.id

ip-api.com:
  timezone 取 response.timezone，且 response.status 必须是 success

ipapi.co:
  timezone 取 response.timezone
```

成功条件不是 HTTP 200，而是请求成功、JSON 可解析、timezone 非空并且看起来是 IANA timezone，例如 `America/Los_Angeles`。

切换代理模式示例：

```bash
curl -X PATCH http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/proxy-mode \
  -H 'Content-Type: application/json' \
  -d '{"mode":"global"}'
```

`proxy-mode` 独立接口继续保留，用于只切换模式、不提交整份代理配置的场景。它只修改 `proxy/clash.yaml` 顶层 `mode` 字段，支持：

```text
rule
global
direct
```

这个接口是代理配置修改能力，不是 `run` 参数。切换模式后会递增 `binding.version`，并把 timezone、risk 和 availability 标记为 pending。`identityHash` 不因代理模式变化而改变。环境包正在 `running` 时会返回 `restartQueued=true`，由后台 `forceRecreate` 让模式变化和 timezone 重新探测生效。

CDP 基础诊断示例：

```bash
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/cdp-test'
```

这个接口只测试 CDP 自身是否可用，不访问 timezone provider，也不判断代理出口。成功时 `data.ok=true`，并返回 `/json/version` 的浏览器信息、WebSocket 地址和 `Runtime.evaluate` 的结果；失败时 `data.ok=false`，`stage/error` 会指出卡在 `http_version`、`create_target`、`websocket`、`runtime_enable` 或 `runtime_evaluate` 哪一步。

### CDP 命令接口规划

当前已经落地的是 `cdp-test` 诊断接口；通用 CDP 命令接口尚未实现。后续 TikTok 发视频这类 RPA 会涉及文件上传、DOM 操作和等待页面状态，确实需要一条受控执行通道，因此该接口应排在 TikTok 业务自动化之前落地，而不是等到业务接口内部临时拼接 CDP。

统一 CDP 命令接口让 RPA 流程通过边缘服务下发受控命令，而不是让前端直接连接浏览器 CDP 端口。这个方向可以简化前端调用，也方便后端统一记录日志、控制超时、处理容器化部署下的 CDP 地址和 Host 头问题。

建议接口方向：

```text
POST /api/v1/edge/browser-envs/{envId}/cdp/command
```

第一版定位应是“受控命令接口”，不是裸 CDP 透传。端口必须从环境包索引读取，`envId` 必须处于 running 状态，命令需要白名单，参数需要校验，超时和返回体大小都要有上限。`Runtime.evaluate` 这类能力可以先只给内部调试或后端内置动作使用，不建议默认允许业务前端提交任意 JS。

不建议第一版开放下面这些高风险命令到通用命令接口：

```text
Browser.close
Target.closeTarget
Target.createBrowserContext
Target.disposeBrowserContext
Browser.setDownloadBehavior
Page.setDownloadBehavior
Network.setCookie
Network.deleteCookies
Storage.clearDataForOrigin
Storage.clearCookies
IndexedDB.deleteDatabase
Browser.grantPermissions
Browser.resetPermissions
Emulation.setTimezoneOverride
Emulation.setLocaleOverride
Emulation.setDeviceMetricsOverride
Network.setUserAgentOverride
Fetch.enable
```

原因不是这些命令技术上不能用，而是它们会绕过环境包边界：

```text
生命周期应该由 run/stop/delete 管，不应由 Browser.close 或 Target.closeTarget 绕过。
登录态应该由 browser-data/profile 管，不应由 Network.setCookie 或 Storage.clearDataForOrigin 随意污染或清空。
下载目录应该固定在环境包受控目录，不应由 Browser.setDownloadBehavior 传任意路径。
timezone/language/screen/UA 应该来自 profile 和 fingerprint/runtime-config，不应由 CDP 临时 override 后让环境包事实失真。
Fetch/Network 拦截会改变真实请求链路，可能和 proxy/clash.yaml、指纹和风控判断产生冲突。
Runtime.evaluate 等价于浏览器内远程执行脚本，必须区分内部调试和普通业务动作。
```

后续更稳的拆法是：

```text
Level 1：安全原子动作
  navigate、getTitle、screenshot、click、type、wait、受控 evaluate。

Level 2：专门业务接口
  cookie import/export、下载管理、缓存清理、timezone/language/fingerprint 修改。

Level 3：内部调试接口
  原始 CDP command 或任意 Runtime.evaluate，仅限受控环境，并记录审计日志。
```

核心原则：CDP 命令接口可以作为统一执行入口，但不能绕过环境包的身份、登录态、代理、指纹、timezone 和 Docker 生命周期管理。后期根据 RPA 流程推进，再逐步把高风险能力做成专门 API。

浏览器 VNC 示例：

```bash
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/vnc-info'
```

返回里的 `webVncUrl` 可以直接在浏览器打开。Mac 原生 VNC 客户端如果弹密码框，可以不用它，改用该浏览器页面。

VNC 端口映射是宿主 `910x` 到容器内固定 `5900`。`profile.ports.vnc` 和 SQLite `vnc_port` 保存的是宿主发布端口；浏览器镜像里的 `x11vnc` 仍监听 `VNC_PORT=5900`，不同环境包之间的隔离由 Docker `PortBindings` 完成。

容器化部署 Private_Browser_Client 时，VNC / CDP 不能在服务内部固定访问 `127.0.0.1`。浏览器容器的 `810x/910x` 端口是发布在 Docker 宿主机上的；服务容器里的 `127.0.0.1` 只代表服务容器自己。当前实现会根据 `Settings/config-docker.yaml` 里的 `docker.api_url` 自动选择发布端口访问主机，例如 `http://host.docker.internal:2375` 会让 noVNC 代理和 rule 模式 timezone CDP 探测访问 `host.docker.internal:910x/810x`。如果这里配错，典型现象是：

```text
连接 VNC TCP 失败: dial tcp 127.0.0.1:910x: connect: connection refused
cdp create target failed: dial tcp 127.0.0.1:810x: connect: connection refused
```

## Apifox

OpenAPI 文件：`docs/openapi.yaml`

导入方式：Apifox → 导入 → OpenAPI / Swagger → 指向文件。

服务启动后也可以直接打开：

```text
http://127.0.0.1:3300/swagger
```

`/swagger` 页面会优先加载本地 `/vendor/swagger-ui` 静态资源；当前镜像默认不内置该目录，会自动回退到 CDN。

## Docker

### 构建镜像

在 `Private_Browser_Client` 项目根目录执行：

```bash
docker build -t private-browser-client:local .
```

如果你要打 `linux/amd64` 镜像并推到阿里云 ACR，推荐这样构建和打 tag：

```bash
docker buildx build --platform linux/amd64 --load -t private-browser-client:amd64-20260609 .
docker tag private-browser-client:amd64-20260609 registry.cn-hangzhou.aliyuncs.com/<namespace>/private-browser-client:amd64-20260609
docker push registry.cn-hangzhou.aliyuncs.com/<namespace>/private-browser-client:amd64-20260609
```

当前商业部署口径不要求导出镜像 tar；后续由管理员 tag 后推 ACR，节点侧按镜像地址拉取。

### 运行容器

`data/` 必须挂载到宿主机目录。

原因：

- SQLite 数据库会写到 `/app/data/private_browser_client.db`。
- 浏览器环境包后续会写到 `/app/data/browser-envs/...`。
- 这些都是边缘服务运行态数据，不应该打进镜像，也不应该跟随容器删除。

先创建宿主机数据目录：

```bash
mkdir -p "$(pwd)/data"
```

Mac / Docker Desktop 运行示例：

```bash
IMAGE=private-browser-client:local \
DATA_DIR="$(pwd)/data" \
scripts/docker_run_mac.sh
```

Mac 脚本不会挂 `/dev/net/tun`，适合本地接口和界面 smoke。需要测试代理时，Clash 配置应使用 `tun.enable=false`。

Linux / x86_64 / amd64 完整 TUN 运行示例：

```bash
IMAGE=crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge_server:0.1.8-amd64 \
DATA_DIR=/Business/data \
scripts/docker_run_linux_tun.sh
```

Linux 脚本会固定增加：

```bash
--add-host=host.docker.internal:host-gateway
--cap-add NET_ADMIN
--device /dev/net/tun:/dev/net/tun
```

容器默认使用 `Settings/config-docker.yaml`，其中 Docker API 地址是：

```text
http://host.docker.internal:2375
```

#### Linux / x86_64 / UDP 自动发现部署脚本

如果该 Client 需要被 Node Server 通过 UDP discovery 自动发现，推荐在独立内网服务器上使用 `host` 网络部署 Client。

设计原因：

- Docker `bridge` 网络下，Client 容器向 `255.255.255.255:43000` 发 UDP 广播时，广播不一定能穿到宿主机所在局域网。
- `host` 网络下，Client 的 UDP beacon 会直接从宿主机网络栈发出，更适合 Node Server 自动发现。
- `host` 网络下不再需要 `-p 3300:3300`，但配置里的 Docker API 应改成 `http://127.0.0.1:2375`。

路径约定：

```text
宿主机数据目录: /Business/data
宿主机 host 网络配置: /Business/data/config-docker-host.yaml
容器配置挂载位置: /app/Settings/config-docker.yaml
容器数据挂载位置: /app/data
```

先在宿主机写入 host 网络专用配置：

```bash
mkdir -p /Business/data

cat > /Business/data/config-docker-host.yaml <<'EOF'
name: private-browser-client
mode: production
version: 0.1.8
server:
  host: 0.0.0.0
  port: 3300
  read_timeout_seconds: 15
  write_timeout_seconds: 15
docker:
  api_url: http://127.0.0.1:2375
status_sync:
  enabled: true
  interval_seconds: 5
  watchdog_seconds: 15
  stale_seconds: 30
discovery:
  enabled: true
  broadcast_address: 255.255.255.255
  port: 43000
  interval_seconds: 5
  magic: PRIVATE_BROWSER_CLIENT_DISCOVERY
  protocol_version: 1
  group: default
  advertise_host: "192.168.10.119"
  advertise_base_url: "http://192.168.10.119:3300"
EOF
```

再重新部署 Client 容器：

```bash
IMAGE=crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge_server:0.1.8-amd64
CONTAINER_NAME=private-browser-client
DATA_DIR=/Business/data

docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

docker run -d \
  --name "$CONTAINER_NAME" \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  --restart unless-stopped \
  --network host \
  -v "${DATA_DIR}:/app/data" \
  -v "${DATA_DIR}/config-docker-host.yaml:/app/Settings/config-docker.yaml:ro" \
  --cap-add NET_ADMIN \
  --device /dev/net/tun:/dev/net/tun \
  "$IMAGE"
```

验证：

```bash
curl http://192.168.10.119:3300/health

docker inspect private-browser-client \
  --format 'name={{.Name}} network={{.HostConfig.NetworkMode}} binds={{.HostConfig.Binds}}'
```

期望健康检查里看到：

```text
dockerApi: http://127.0.0.1:2375
status: healthy
```

在 Node Server 上验证 UDP discovery：

```bash
curl http://127.0.0.1:3400/api/v1/edge-clients/discovered \
  -H 'X-Main-Account-Id: 906090001' \
  -H 'X-Platform-User-Id: user_1780995561009325000_000001' \
  -H 'X-Platform-Username: user_906090001' \
  -H 'X-Platform-Role: owner'
```

期望出现：

```text
sourceIp: 192.168.10.119
baseUrl: http://192.168.10.119:3300
service: private-browser-client
```

如果必须继续使用 Docker `bridge` 网络，则不要依赖 `255.255.255.255` 广播穿透局域网。应把 `discovery.broadcast_address` 改成 Node Server 的内网 IP，让 Client 走 UDP 单播；或者由管理员继续手动添加 Client 节点。

如果不用脚本，节点已经拉取了 amd64 镜像，等价命令是：

```bash
docker run -d \
  --name private-browser-client \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  --restart unless-stopped \
  -p 3300:3300 \
  -v "$(pwd)/data:/app/data" \
  --add-host=host.docker.internal:host-gateway \
  --cap-add NET_ADMIN \
  --device /dev/net/tun:/dev/net/tun \
  registry.cn-hangzhou.aliyuncs.com/<namespace>/private-browser-client:amd64-20260609
```

验证容器：

```bash
curl http://127.0.0.1:3300/health
curl http://127.0.0.1:3300/openapi.yaml
```

浏览器打开：

```text
http://127.0.0.1:3300/swagger
```

停止和删除容器：

```bash
docker stop private-browser-client
docker rm private-browser-client
```

注意：删除容器不会删除宿主机 `$(pwd)/data`，因此 SQLite 数据库和环境包仍然保留。

## 已清理的旧职责

这些能力已经从 `Private_Browser_Client` 源码中移除，后续应进入 `Private_Browser_Server`：

- `/api/v1/auth/*`
- `/api/v1/edge-clients/*`
- 用户模型、用户 Dao、用户 Repository
- 节点中控模型、节点 Dao、节点 Service
- JWT、密码哈希、雪花 ID
- SQLite AutoMigrate 入口

##  备忘录

- 发现dockerfile没有装VNC 和 CDP 还需要检查其他插件，结合老的Private_Browser_Control 里面的dockerfile 文件还缺少什么文件
