# Private_Browser_Client

边缘服务，运行在单台设备上，只负责获取和管理本机 Docker / 浏览器运行环境。

当前已经明确：用户、节点列表、设备归属、设备编号、多节点调度等中心能力不再放在这里，后续应进入 `Private_Browser_Server`。

## 技术栈

- Go 1.22+
- Gin 1.10
- Viper 1.19（多环境配置）
- SQLite（本机环境包索引与状态记录）
- 无前端、无桌面壳
- 当前阶段不使用 JWT；用户、节点和中控能力仍不放在边缘服务里

## 目录结构

```text
Private_Browser_Client/
├── main.go                              # 程序入口：找根目录 → 启动边缘服务
├── Settings/
│   ├── settings.go                      # 多环境配置加载
│   ├── config-dev.yaml                  # 开发环境（端口 3300）
│   ├── config-test.yaml                 # 测试环境（端口 3300）
│   └── config-prod.yaml                 # 生产环境（端口 3300）
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

`Private_Browser_Client` 只负责本机：

- 获取本机设备信息。
- 获取本机 Docker 状态。
- 后续管理本机 Docker 镜像、容器、浏览器实例。
- 后续向中心服务端上报心跳和状态。

它不负责：

- 用户注册登录。
- JWT 鉴权。
- 多节点列表。
- 设备归属关系。
- 设备编号。
- 多节点调度。

## 调用链路

```text
main.go
  └→ detectProjectRoot()              // 从当前目录往上找 Settings/config-dev.yaml
  └→ Infrastructures.Init(root)
       ├→ Settings.Init(root)         // 加载 config-{env}.yaml
       ├→ SQLite.Init()               // 打开 data/private_browser_client-{env}.db 并建 browser_envs
       ├→ StartStatusSyncManager()    // 启动带哨兵的环境包状态同步任务
       ├→ releaseOccupiedPort(port)   // 开发期清理占用端口的旧进程
       ├→ Routes.Setup()              // 注册边缘服务路由
       ├→ http.Server.ListenAndServe  // 启动服务
       └→ waitForShutdownSignal()     // SIGINT/SIGTERM 优雅关闭
```

## 接口清单

### 服务自身

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/` | 服务信息 |
| GET | `/health` | 健康检查，返回配置文件与 Docker API 地址 |
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
| POST | `/api/v1/edge/containers/:id/start` | SSE 任务：启动本机 Docker 容器 |
| POST | `/api/v1/edge/containers/:id/stop` | SSE 任务：停止本机 Docker 容器，请求体可为空 |
| POST | `/api/v1/edge/containers/:id/restart` | SSE 任务：重启本机 Docker 容器，请求体可为空 |
| GET | `/api/v1/edge/tasks/:taskId` | 查询 SSE 任务详情 |
| GET | `/api/v1/edge/tasks/:taskId/events` | SSE 事件流，订阅任务进度和最终结果 |
| GET | `/api/v1/edge/browser-envs` | 查询本机浏览器环境包索引列表，默认排除历史 deleted/归档记录 |
| POST | `/api/v1/edge/browser-envs` | 创建本地浏览器环境包文件，不启动 Docker |
| POST | `/api/v1/edge/browser-envs/import-package` | 上传标准 tar.gz 环境包并导入本机，保留 envId，重新分配本机端口 |
| GET | `/api/v1/edge/browser-envs/:envId` | 查询单个环境包详情，不返回代理明文和指纹 raw |
| POST | `/api/v1/edge/browser-envs/:envId/run` | SSE 任务：按环境包创建或启动本机浏览器容器 |
| POST | `/api/v1/edge/browser-envs/:envId/stop` | SSE 任务：按环境包停止本机浏览器容器，并同步运行态 |
| POST | `/api/v1/edge/browser-envs/:envId/backup-package` | 非 SSE：备份环境包为 tar.gz 下载流，不删除本机环境包 |
| POST | `/api/v1/edge/browser-envs/:envId/export-and-remove` | 非 SSE：导出环境包为 tar.gz 后删除本机源环境包和索引 |
| DELETE | `/api/v1/edge/browser-envs/:envId` | SSE 任务：彻底删除环境包，删除配置目录、登录态目录、已停止容器和 SQLite 索引 |
| PATCH | `/api/v1/edge/browser-envs/:envId/proxy` | running 时返回 SSE 任务：修改环境包代理配置，变更后重建容器 |
| PATCH | `/api/v1/edge/browser-envs/:envId/proxy-mode` | running 时返回 SSE 任务：切换 Clash 规则/全局/直连模式并自动重建 |
| GET | `/api/v1/edge/browser-envs/:envId/cdp-test` | 基础 CDP 连通性诊断：测试 /json/version、target、WebSocket 和 Runtime.evaluate |
| GET | `/api/v1/edge/browser-envs/:envId/vnc-info` | 获取浏览器版 VNC 连接信息 |
| GET | `/api/v1/edge/browser-envs/:envId/vnc/ws` | noVNC WebSocket 到 VNC TCP 的代理通道 |
| GET | `/web-vnc.html?envId=...` | 独立浏览器 VNC 页面 |

## 配置

```yaml
docker:
  api_url: http://127.0.0.1:2375
status_sync:
  enabled: true
  interval_seconds: 5
  watchdog_seconds: 15
  stale_seconds: 30
```

`docker.api_url` 是边缘服务访问本机 Docker Engine 的地址。当前默认使用 Docker HTTP 2375。

`status_sync` 是浏览器环境包后台状态同步任务：Worker 每隔几秒按 Docker 真实容器状态刷新 `browser_envs`，Watchdog 监控 Worker 心跳，异常退出或长时间无心跳时自动拉起。它只同步状态，不自动启动、删除或修改浏览器容器，也不会删除 `browser-data/profile` 登录态目录。

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
ENV=dev go run .

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
创建阶段不会由 Go 边缘服务直接请求 IP 定位网站来最终确认 `timezone`。代理出口、DNS、TUN 和浏览器真实网络环境只在浏览器容器内成立，因此最终 `timezone` 必须在后续 `run` 阶段由容器内探测确认。创建请求里的 `environment.timezone` 只能作为初始值；后续容器内探测成功后，会以探测结果回写 `profile.environment.timezone`、`binding.identity.timezone` 并重算 `identityHash/configHash`。
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

详情接口会返回 `manifest`、`profile`、`binding`、`container`、`proxy` 摘要、`fingerprint` 摘要和一致性检查结果。
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

成功后需要记录 provider、出口 IP、国家/地区、timezone 和 checkedAt，并把 timezone 回写到 profile/binding 后重算 hash。全部失败或超过探测预算时，应记录每个 provider 的失败原因，把 `proxy-runtime.status` 和响应里的 `timezoneStatus` 标记为 `failed`，但不阻塞容器启动/重建结果返回。这个请求不能由 Go 边缘服务宿主机直连完成，也不能由前端代替完成。

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
检查宿主机 /dev/net/tun
  不存在或不是设备：
    不阻断 run，不追加 NET_ADMIN 和设备挂载
    容器继续依赖 mixed-port + 浏览器代理链路
  存在：
    Docker create 自动追加 CapAdd: ["NET_ADMIN"]
    Docker create 自动挂载 /dev/net/tun:/dev/net/tun
```

因为代理配置模板通常都会带 `tun.enable=true`，缺少 `/dev/net/tun` 不能直接导致所有环境包无法启动。Mac / Docker Desktop 开发环境会自动降级；商用 Linux 节点如果需要完整 TUN/DNS 保护，应确保宿主机存在 `/dev/net/tun`，必要时执行 `sudo modprobe tun` 后再运行。

这里要区分“CPU 架构”和“宿主机 TUN 能力”：

```text
Mac / Docker Desktop
  常见问题是 Docker Desktop VM 不一定暴露可挂载的 /dev/net/tun。
  即使浏览器镜像是 arm64，也可能不能跑完整 TUN。
  当前后端会自动把注入容器的运行时代理配置临时降级为 tun.enable=false，走 mixed-port + 浏览器代理链路。

Linux / Ubuntu x86_64 / amd64
  通常可以支持完整 TUN/DNS。
  前提是宿主机存在 /dev/net/tun，并且 Docker create 带 NET_ADMIN + /dev/net/tun 设备挂载。
  如果 /dev/net/tun 不存在，先在宿主机执行 sudo modprobe tun。
```

所以 Ubuntu amd64 商用节点正常不会遇到 Mac 这种 Docker Desktop TUN 限制；后端检测到 `/dev/net/tun` 后会自动保留 `tun.enable=true` 并追加容器权限。只有检测不到 TUN 设备时，才会在运行时注入配置里临时关闭 TUN；磁盘上的 `proxy/clash.yaml` 不会被改写，环境包迁移到 Linux TUN 节点后仍可恢复完整 TUN 行为。

停止浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/stop \
  -H 'Content-Type: application/json' \
  -d '{"timeoutSeconds":10}'
```

`stop` 接口围绕 envId 停止容器，并回写 `container.json`、`manifest.lastRuntime` 和 SQLite `browser_envs` 运行态。
它不会删除容器、镜像或 `browser-data/profile` 登录态目录。

备份浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/backup-package \
  -o {envId}-backup.tar.gz
```

备份接口会把环境包复制到 staging，补充导出元信息和 checksums 后生成 `.tar.gz` 下载流。
它不会删除源环境包目录、Docker 容器、浏览器镜像或 SQLite 索引。
如果环境包仍在运行中，接口会返回状态冲突，调用方应先执行 `stop`。

导出并移除浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/export-and-remove \
  -o {envId}-export.tar.gz
```

`export-and-remove` 使用和 `backup-package` 相同的 `.tar.gz` 包协议，但语义是迁移：下载包生成成功后，会删除关联的已停止 Docker 容器、源环境包目录和 SQLite 索引。
它不会自动停止 running 容器，也不会删除浏览器镜像。
如果环境包仍在运行中，接口会返回状态冲突，调用方应先执行 `stop`。

导入浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/import-package \
  -F "file=@{envId}-export.tar.gz"
```

`import-package` 只接受本服务 `backup-package` 或 `export-and-remove` 生成的标准 `.tar.gz` 包。
导入会校验单根目录、`manifest.json`、标准文件和 checksums；默认保留原 `envId`，如果本机已存在同名环境包会拒绝覆盖。
导入到本机后会重新分配 `envSequence`、CDP/VNC 端口，并把容器运行态重置为 `created`；下一次 `run` 会重新在浏览器容器内探测 timezone。

彻底删除浏览器环境包示例：

```bash
curl -X DELETE http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}
```

删除接口会物理删除环境包目录，包括 `manifest.json`、`profile.json`、`binding.json`、`proxy/`、`fingerprint/` 和 `browser-data/profile`，同时删除关联的已停止 Docker 容器并移除 SQLite `browser_envs` 索引记录。
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
    "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-arm64",
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
  -d "{\"enabled\":true,\"type\":\"clash-verge\",\"image\":\"crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-arm64\",\"mode\":\"rule\",\"configBase64\":\"$CONFIG_B64\"}"
```

`configBase64` 必须来自一整份合法 YAML 原文。不要把两份 YAML 拼接在一起；例如 `- MATCH,relay` 后面又直接接 `mode: rule`，会导致代理配置语义错误。
Base64 长度通常比 YAML 原文更长，约等于 `4 * ceil(原文字节数 / 3)`，真实代理配置生成几 KB 到几十 KB 的单行字符串都正常。
PATCH 代理配置也可以通过 `mode` 同时切换 Clash 顶层模式。后端会先解码 `configBase64`，再把 `mode` 写入 YAML 顶层；如果只传 `mode` 不传 `configBase64`，则只修改现有 `proxy/clash.yaml` 的顶层 `mode`。
同一个接口还可以传 `image` 修改 `profile.runtime.image`，例如切换到当前测试统一使用的 `crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-arm64`。只改 `image` 不会改变 `binding.identityHash`；但如果环境包正在 `running`，仍然会后台重建容器，让新镜像生效。

代理配置不是热更新。只要配置实际发生变化，就必须通过重建容器让配置生效。
如果环境包正在 `running`，接口会先完成配置落盘和 hash 更新，然后立即返回 `restartQueued=true`；后端会在后台串行执行 `forceRecreate` 重建容器，前端不需要再单独调用 `stop/run`。
这样 rule 模式下 CDP/timezone provider 即使耗时较长，也不会拖断本次 PATCH 请求。
这里要特别注意：异步的是 `running` 环境的容器重建任务，不是 `rule` 模式本身。`rule/global/direct` 在 running 环境下都会快速返回并进入后台重建；区别只在后台 timezone probe 的入口不同，`rule` 走 CDP，`global/direct` 走容器内 curl/wget。
如果环境包不是运行态，响应会返回 `restartRequired=true`，表示下一次 `run` 时生效。
代理配置发生变化时，该接口会重算 `binding.identityHash` 并递增 `binding.version`；仅修改 `image` 时不改变环境身份。两种情况都不会删除 `browser-data/profile`。
代理变化后 timezone 也必须重新确认。规则如下：

```text
running 环境：
  PATCH proxy -> 写入新代理/新镜像 -> 返回 restartQueued=true + taskId + eventsUrl -> 后台 forceRecreate -> 容器内多源 timezone probe。
  如果 timezone 成功，回写 timezone/hash；如果超时或失败，在详情 proxy.runtime 里保留 failed 记录。

非 running 环境：
  PATCH proxy -> 写入新代理/新镜像 -> 标记下次 run 生效；代理变化时重新确认 timezone -> 返回 restartRequired=true。
```

### SSE 任务接口

下面这些接口已经改为 SSE 任务化：HTTP 请求只负责创建任务并立即返回 `taskId/eventsUrl`，真实执行结果通过 `/api/v1/edge/tasks/{taskId}/events` 推送。

```text
POST   /api/v1/edge/docker/pull-image
POST   /api/v1/edge/docker/remove-image
POST   /api/v1/edge/containers/:id/start
POST   /api/v1/edge/containers/:id/stop
POST   /api/v1/edge/containers/:id/restart
POST   /api/v1/edge/browser-envs/:envId/run
POST   /api/v1/edge/browser-envs/:envId/stop
DELETE /api/v1/edge/browser-envs/:envId
```

`PATCH /api/v1/edge/browser-envs/:envId/proxy` 和 `PATCH /api/v1/edge/browser-envs/:envId/proxy-mode` 是条件任务化：只有环境包正在 `running` 且配置实际变化时，才会返回 `restartQueued=true + taskId + eventsUrl`；非运行态只标记下次 run 生效。

备份、导出、导入暂时不是 SSE：

```text
POST /api/v1/edge/browser-envs/:envId/backup-package
POST /api/v1/edge/browser-envs/:envId/export-and-remove
POST /api/v1/edge/browser-envs/import-package
```

原因是备份/导出当前直接返回 `application/gzip` 文件流，贸然改成 SSE 会改变下载语义。后续如果要任务化文件类接口，应先设计 `artifactUrl` 或临时下载凭据，再由 SSE 只负责报告打包进度。

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

任务数据保存在当前 Client 进程内，主要用于实时观察和排障；服务重启后历史任务不会恢复。前端如果刷新页面，可以先请求 `GET /api/v1/edge/tasks/{taskId}` 读取当前进程内的任务摘要，再决定是否继续订阅 SSE。

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

这个接口是代理配置修改能力，不是 `run` 参数。切换模式后会重算代理配置 hash 和 `binding.identityHash`，递增 `binding.version`，并把 timezone 标记为 pending。环境包正在 `running` 时会返回 `restartQueued=true`，由后台 `forceRecreate` 让模式变化和 timezone 重新探测生效。

CDP 基础诊断示例：

```bash
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/cdp-test'
```

这个接口只测试 CDP 自身是否可用，不访问 timezone provider，也不判断代理出口。成功时 `data.ok=true`，并返回 `/json/version` 的浏览器信息、WebSocket 地址和 `Runtime.evaluate` 的结果；失败时 `data.ok=false`，`stage/error` 会指出卡在 `http_version`、`create_target`、`websocket`、`runtime_enable` 或 `runtime_evaluate` 哪一步。

### CDP 命令接口规划

后续可以考虑新增统一的 CDP 命令接口，让 RPA 流程通过边缘服务下发受控命令，而不是让前端直接连接浏览器 CDP 端口。这个方向可以简化前端调用，也方便后端统一记录日志、控制超时、处理容器化部署下的 CDP 地址和 Host 头问题。

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
docker build -t private-browser-client:dev .
```

如果你要打 `linux/amd64` 镜像包，推荐这样构建并导出：

```bash
docker buildx build --platform linux/amd64 --load -t private-browser-client:amd64-20260530 .
docker save -o dist/private-browser-client-amd64-20260530.tar private-browser-client:amd64-20260530
```

加载镜像包：

```bash
docker load -i dist/private-browser-client-amd64-20260530.tar
```

### 运行容器

`data/` 必须挂载到宿主机目录。

原因：

- SQLite 数据库会写到 `/app/data/private_browser_client-docker.db`。
- 浏览器环境包后续会写到 `/app/data/browser-envs/...`。
- 这些都是边缘服务运行态数据，不应该打进镜像，也不应该跟随容器删除。

先创建宿主机数据目录：

```bash
mkdir -p "$(pwd)/data"
```

Mac / Docker Desktop 运行示例：

```bash
docker run --rm \
  --name private-browser-client \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  -p 3300:3300 \
  -v "$(pwd)/data:/app/data" \
  private-browser-client:dev
```

后台运行示例：

```bash
docker run -d \
  --name private-browser-client \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  --restart unless-stopped \
  -p 3300:3300 \
  -v "$(pwd)/data:/app/data" \
  private-browser-client:dev
```

Linux 如果容器需要访问宿主机 Docker 2375，可增加：

```bash
--add-host=host.docker.internal:host-gateway
```

Linux 完整示例：

```bash
docker run -d \
  --name private-browser-client \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  --restart unless-stopped \
  -p 3300:3300 \
  -v "$(pwd)/data:/app/data" \
  --add-host=host.docker.internal:host-gateway \
  private-browser-client:dev
```

容器默认使用 `Settings/config-docker.yaml`，其中 Docker API 地址是：

```text
http://host.docker.internal:2375
```

如果你是先 `docker load` 了 amd64 镜像包，再直接跑容器，命令可以这样：

```bash
docker run -d \
  --name private-browser-client \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  --restart unless-stopped \
  -p 3300:3300 \
  -v "$(pwd)/data:/app/data" \
  --add-host=host.docker.internal:host-gateway \
  private-browser-client:amd64-20260530
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
- `/api/v1/nodes/*`
- 用户模型、用户 Dao、用户 Repository
- 节点中控模型、节点 Dao、节点 Service
- JWT、密码哈希、雪花 ID
- SQLite AutoMigrate 入口

##  备忘录

- 发现dockerfile没有装VNC 和 CDP 还需要检查其他插件，结合老的Private_Browser_Control 里面的dockerfile 文件还缺少什么文件
