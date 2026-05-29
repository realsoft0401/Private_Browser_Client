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
| POST | `/api/v1/edge/docker/pull-image` | 拉取本机 Docker 镜像 |
| POST | `/api/v1/edge/docker/remove-image` | 删除本机 Docker 镜像 |
| POST | `/api/v1/edge/containers/:id/start` | 启动本机 Docker 容器 |
| POST | `/api/v1/edge/containers/:id/stop` | 停止本机 Docker 容器，请求体可为空 |
| POST | `/api/v1/edge/containers/:id/restart` | 重启本机 Docker 容器，请求体可为空 |
| GET | `/api/v1/edge/browser-envs` | 查询本机浏览器环境包索引列表，默认排除假删除 |
| POST | `/api/v1/edge/browser-envs` | 创建本地浏览器环境包文件，不启动 Docker |
| GET | `/api/v1/edge/browser-envs/:envId` | 查询单个环境包详情，不返回代理明文和指纹 raw |
| POST | `/api/v1/edge/browser-envs/:envId/run` | 按环境包创建或启动本机浏览器容器 |
| POST | `/api/v1/edge/browser-envs/:envId/stop` | 按环境包停止本机浏览器容器，并同步运行态 |
| PATCH | `/api/v1/edge/browser-envs/:envId/proxy` | 修改环境包代理配置，变更后需要重新启动容器 |
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
  -d '{"image":"alpine:latest"}'
```

删除镜像示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/docker/remove-image \
  -H 'Content-Type: application/json' \
  -d '{"image":"alpine:latest","force":false,"noPrune":false}'
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
      "image":"crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1"
    },
    "environment":{
      "timezone":"America/Toronto",
      "language":"en-US",
      "screen":{"width":1366,"height":768}
    },
    "proxy":{
      "enabled":true,
      "type":"clash-verge",
      "config":"mode: rule\nmixed-port: 7897\n"
    },
    "metadata":{"description":"TikTok browser env"}
  }'
```

该接口只写入 `data/browser-envs/users/{userId}/{rpaType}/{envId}` 环境包，不创建或启动 Docker 容器。
创建成功后会同步写入 `browser_envs` SQLite 索引表，用于后续列表、假删除、运行状态和监控状态查询。

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

停止浏览器环境包示例：

```bash
curl -X POST http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/stop \
  -H 'Content-Type: application/json' \
  -d '{"timeoutSeconds":10}'
```

`stop` 接口围绕 envId 停止容器，并回写 `container.json`、`manifest.lastRuntime` 和 SQLite `browser_envs` 运行态。
它不会删除容器、镜像或 `browser-data/profile` 登录态目录。

修改代理配置示例：

```bash
curl -X PATCH http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/proxy \
  -H 'Content-Type: application/json' \
  -d '{
    "enabled": true,
    "type": "clash-verge",
    "configBase64": "bW9kZTogcnVsZQptaXhlZC1wb3J0OiA3ODk3Cg=="
  }'
```

`configBase64` 是代理 YAML 原文的 Base64 编码，例如：

```bash
base64 -i clash.yaml | tr -d '\n'
```

代理配置不是热更新。只要配置实际发生变化，就必须通过重建容器让配置生效。
如果环境包正在 `running`，接口会自动 `forceRecreate` 重建容器，并返回 `restarted=true`，前端不需要再单独调用 `stop/run`。
如果环境包不是运行态，响应会返回 `restartRequired=true`，表示下一次 `run` 时生效。
该接口会重算 `binding.identityHash` 并递增 `binding.version`，但不会删除 `browser-data/profile`。

浏览器 VNC 示例：

```bash
curl 'http://127.0.0.1:3300/api/v1/edge/browser-envs/{envId}/vnc-info'
```

返回里的 `webVncUrl` 可以直接在浏览器打开。Mac 原生 VNC 客户端如果弹密码框，可以不用它，改用该浏览器页面。

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
