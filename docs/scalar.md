# Scalar API Reference 方案

## 1. 目标

给当前 `Private_Browser_Client` 增加一个比 `/swagger` 更正式的 API Reference 展示入口，并支持单独打成文档容器。

当前这条链路只服务展示：

- 协议事实源仍然是 `docs/openapi.yaml`
- `Scalar` 只负责页面渲染
- 不新增第二份 OpenAPI，不新增第二份路由协议真相

## 2. 当前新增内容

- 本机页面入口：
  - `GET /scalar`
  - `GET /scalar/`
- 静态页面：
  - `public/scalar.html`
- 独立文档容器：
  - `Dockerfile.scalar`
  - `docker/scalar/nginx.conf`

额外展示口径：

- 当前页面只保留 4 类 `Client Libraries`
  - `Python`
  - `Go`
  - `Java`
  - `PHP`
- 这里不再展示其它语言占位，避免后期页面和正式 SDK 规划不一致

## 3. 两种使用方式

### 方式 A：直接复用当前 Client 服务

前提：

- `Private_Browser_Client` 服务本身已经跑在 `127.0.0.1:3300`

访问：

```text
http://127.0.0.1:3300/scalar
```

这时：

- 页面由 Client 自己返回
- 协议从 `http://127.0.0.1:3300/openapi.yaml` 读取
- 页面里默认展示和调试的 API 服务地址也应是 `http://127.0.0.1:3300`

### 方式 B：单独打成 Scalar 文档容器

构建：

```bash
docker build -f Dockerfile.scalar -t private-browser-client-scalar:latest .
```

运行：

```bash
docker run --rm -p 13300:8080 private-browser-client-scalar:latest
```

访问：

```text
http://127.0.0.1:13300/
```

这时：

- 这是一个独立文档容器
- 不依赖 Client 主服务进程
- 但展示内容仍然来自同仓库里的 `docs/openapi.yaml`
- 文档页虽然跑在 `13300`，正式 API 目标地址仍应是 `http://127.0.0.1:3300`

## 3.1 关键口径

这里必须明确区分两件事：

- 文档页面地址
- 实际 API 服务地址

示例：

- `http://127.0.0.1:3300/scalar`
  - 表示页面和 API 都在 Client 服务里
- `http://127.0.0.1:13300/`
  - 只表示独立文档容器页面地址
  - 不表示 API 也在 `13300`

当前默认正式 API 地址应始终理解为：

```text
http://127.0.0.1:3300
```

## 3.2 Client Libraries 展示原则

当前 `Scalar` 页面里已经把语言范围主动收窄为：

- Python
- Go
- Java
- PHP

这样做不是临时视觉调整，而是后续企业级 SDK 规划的展示收口：

- 页面先按这四种语言展示
- 后续自动生成 SDK 也优先只围绕这四种语言建设
- 不再让页面给出一堆未来不会维护的杂语言暗示

## 4. 当前边界

- 这是展示层，不是 SDK
- 这是文档容器，不是 Client 业务运行容器
- 它不会替代 `/swagger`
- 它不会替代 `docs/api/*.md`

建议角色分工：

- `/swagger`
  - 继续作为调试页和快速联调入口
- `/scalar`
  - 作为更正式的 API Reference 展示入口
- `docs/api/*.md`
  - 继续承担业务语义、状态机、SSE、失败收口和排障说明

## 5. 后续建议

如果后期确认要正式切到 `Scalar`，建议继续做这几件事：

- 给 `Client` 和 `Server` 分别做独立 Scalar 文档容器
- 补一个统一文档首页，区分：
  - Client API
  - Server API
  - Platform API
- 再决定是否继续往 SDK 生成链路扩展
