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
- `node-registration/*` 是当前 Node bind 成功后把中心唯一设备身份写回 Client 的正式配套能力，但它不参与 UDP discovery，也不承担业务放行判断

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

当前也额外提供一条更正式的 API Reference 展示尝试：

- `public/scalar.html`
- `/scalar`

访问方式和 `/swagger` 一样，直接走 Client 主服务：

```text
http://127.0.0.1:3300/scalar
```

其中 `/scalar` 是唯一正式入口，避免 API 文档页出现多个访问口径。

当前 `Scalar` 展示页的 Client Libraries 口径只保留：

- Python
- Go
- Java
- PHP

## Client 镜像构建

当前仓库已经补齐正式 `Client` 业务镜像 Dockerfile，统一构建入口脚本是：

```bash
cd /Users/lining/Documents/Browser_virtualization/Private_Browser_Client
./scripts/build-client-image.sh
```

## 运行方式

现在 Client 不再依赖外部挂载 `config-docker.yaml`。先启动 Client，让它在局域网里发 UDP beacon；Node 发现后再 bind 并回写本地 JSON。

示例：

```bash
docker run -d \
  --name private-browser-client \
  --restart always \
  --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /Business/data:/app/data \
  crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge_server:0.1.10-amd64
```

如果后面 Node 已经发现并完成 bind，再由 Node 回写本地 `node-registration.json`；这一步不需要在 Client 启动命令里预先塞 Node 地址。

默认行为：

- 默认构建 `linux/amd64`
- 默认镜像名 `private-browser-client:latest`
- 默认使用 `--load` 装回本机 Docker
- 默认使用国内镜像入口、Debian 源和 Go 代理

常用示例：

```bash
cd /Users/lining/Documents/Browser_virtualization/Private_Browser_Client
./scripts/build-client-image.sh --platform linux/amd64 --image private-browser-client --tag amd64
./scripts/build-client-image.sh --platform linux/arm64 --image private-browser-client --tag arm64
./scripts/build-client-image.sh --platform linux/amd64 --image repo/private-browser-client --tag 0.2.0 --push
```

如果后续海外 CI 或客户环境直连官方源更稳定，可覆盖：

```bash
DOCKERHUB_MIRROR=docker.io \
DEBIAN_MIRROR=deb.debian.org \
GOPROXY=https://proxy.golang.org,direct \
GOSUMDB=sum.golang.org \
./scripts/build-client-image.sh --platform linux/amd64 --image private-browser-client --tag amd64
```

这里必须注意：

- `3300` 是 `Private_Browser_Client` 的真实 API 服务地址
- `/swagger` 和 `/scalar` 都挂在同一个 Client 服务内
- 后续不再维护独立 Scalar 展示服务，避免文档服务地址和真实 API 地址再次混淆

## WebVNC 边界

新的 `WebVNC` 不再围绕 `package/envId`，而是围绕 `slot`。

也就是后续入口口径应按下面这类方式统一：

- `/web-vnc.html?slot=slot001`

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

## 镜像版本边界

当前必须区分两类镜像：

- `slot runtime` 基础镜像
  - 由 `slot_runtime.image` 提供默认值
  - 只影响后续新建 slot 或显式 `reinit-slot`
- `browser-env` 正式运行镜像
  - 由环境包自己的 `runtime.image` 决定
  - 才是某个账号环境真正运行时使用的镜像事实

当前正式规则：

- 修改默认 `slot_runtime.image`，不会自动升级已有 slot
- 已存在 slot 必须保留自己的当前实际 `runtimeImage`
- 老 slot 想从 `1.1` 升到 `1.2`，必须走显式重初始化
- 重初始化成功后仍是原 slot 恢复 `waiting`，不是生成新 slot
- browser-env 的正式运行镜像也必须显式修改，不能被 slot 默认值偷偷带着变化

cd /Users/lining/Documents/Browser_virtualization/Private_Browser_Client
DEBIAN_MIRROR=deb.debian.org \
./scripts/build-client-image.sh \
  --platform linux/amd64 \
  --image crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge_server \
  --tag 0.1.10-amd64 \
  --push


docker run -d \
  --name private-browser-client \
  --restart always \
  --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /Business/Settings:/app/Settings \
  -v /Business/data:/app/data \
  crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge_server:0.1.10-amd64


  
