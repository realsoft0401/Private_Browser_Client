# Private_Browser_Client

新的 `Private_Browser_Client` 从这个目录重新开始。

当前阶段只保留干净项目骨架，不继承 old 业务实现。

## 当前原则

- 目录层次完全沿用 old 项目
- 业务模型按新的 `browser-env / slot / runtime relation` 重建
- `browser-env` 是正式业务资产主线，`slot` 只是本机运行承载位
- old 代码已单独冻结到 `Private_Browser_Client_Old`
- 新项目从第一天起保留 `Swagger / OpenAPI` 能力骨架

## 职责边界

`Private_Browser_Client` 只负责本机边缘能力：

- 获取本机设备信息
- 获取本机 Docker 状态
- 管理本机 Docker / 浏览器运行环境
- 通过 HTTP API 暴露本机能力
- 通过 UDP beacon 在独立内网广播本机服务入口和非敏感摘要

它不负责：

- 用户注册登录
- JWT、API Key、mTLS 等鉴权
- 多节点调度
- 设备归属
- 设备编号
- 多 Client 列表
- 中心平台权限判断
- 中心 `clientId` 身份真相维护

这里再特别收口一次：

- Client 不生成 `clientId`
- Client 不以 `clientId` 作为本机正式 API 输入
- `clientId` 是 `Private_Browser_Server` 的中心身份字段，不是 Edge 本机资源标识
- 后续如果保留 `node-registration` 相关接口，也只作为过渡期联调/排障留痕能力，不作为正式业务主链路

## UDP 自动发现边界

Client 后期需要支持 UDP discovery / beacon，用于在独立内网中让 Server 自动发现本机服务。

边界要求：

- 只广播服务入口，不承载业务动作
- 不返回环境包状态
- 不传 proxy 明文、fingerprint raw、Cookies、Local Storage、IndexedDB、Session Storage、Login Data 或备份包路径
- 只广播非敏感摘要，例如 `discoveryMagic`、`protocolVersion`、`service`、`discoveryGroup`、`clientIp`、`hostname`、`baseUrl`、`clientVersion`、`startedAt`、`lastHeartbeatAt`、`capabilities`
- `discoveryMagic/service/discoveryGroup` 用来识别当前私有浏览器平台和当前内网发现域
- Client 不维护其它 Client 列表，不主动调用其它 Client

## 安全边界

当前版本采用内网受信边缘服务模式：

- Client 不直接暴露公网
- 调用方是受信中心服务、运维工具或本机管理进程
- 用户认证、权限判断和对外访问控制由 `Private_Browser_Server` 或网络边界承担
- 未来如果进入公网或跨客户网络，再单独设计节点鉴权

## Swagger

新项目从第一天起保留 `Swagger / OpenAPI` 骨架：

- `docs/openapi.yaml`
- `public/swagger.html`

## WebVNC 边界

新的 `WebVNC` 不再围绕 `package/envId`，而是围绕 `slot`。

也就是后续入口口径应按下面这类方式统一：

- `/web-vnc.html?slot=1`
- `/web-vnc.html?slot=2`

它表达的是：

- 当前查看的是哪个 slot 上的 WebVNC 连接入口
- 不是某个包天然绑定的固定浏览器
- 包运行到哪个 slot，就通过哪个 slot 的 WebVNC 查看

但这里的 `slot` 只是运行承载视角，不是产品主叙事：

- 正式业务入口仍然是 `browser-envs/*`
- `slot` 只是 Client 本机资源层
- 不应让前端、平台或后续对外文档把本项目理解成“slot 管理平台”

维护原则：

- `slot=waiting` 时应提示当前没有运行实例
- `slot=loading/releasing` 时不能伪装成稳定可连接态
- `web-vnc.html?slot=...` 页面可访问，只说明页面入口、`vnc-info` 和 `ws` 路由存在
- 是否能看到真实桌面画面，还取决于当前 `slot runtime` 镜像内部是否真的提供 VNC 服务和浏览器桌面
- 如果当前 `slot runtime` 只是占位容器，例如 `alpine + sleep infinity`，则页面仍可访问，但不会出现真实浏览器画面
- 不再继续沿用 old 的 `web-vnc.html?envId=...` 视角
