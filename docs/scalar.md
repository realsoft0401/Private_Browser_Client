# Scalar API Reference 方案

## 1. 当前结论

`Private_Browser_Client` 的 Scalar 页面已经收口为 Client 主服务内置能力。

它和 `/swagger` 一样，直接由 3300 服务提供，不再维护单独的 Scalar 展示服务。

## 2. 访问入口

正式推荐入口：

```text
http://127.0.0.1:3300/scalar
```

其中：

- `/scalar` 是正式路径。
- Scalar 只保留这一个入口，避免 API 文档页出现多个访问口径。
- 页面读取同一份 `/openapi.yaml`。
- 页面和真实 API 都在同一个 Client 服务内，不再出现文档端口和 API 端口分裂。

## 3. 事实源

当前只保留一份协议事实源：

- `docs/openapi.yaml`

当前只保留一份展示页面：

- `public/scalar.html`

维护原则：

- 不新增第二份 OpenAPI。
- 不新增独立 Scalar 展示服务。
- 不再维护单独的 Scalar 构建链路。
- Client 正式镜像必须继续复制 `docs` 和 `public`，确保 `/scalar`、`/openapi.yaml` 在容器里可用。

## 4. Client Libraries 展示原则

当前 `Scalar` 页面里只保留：

- Python
- Go
- Java
- PHP

这样做是为了和后续企业级 SDK 规划保持一致，不给出暂时不会维护的语言暗示。

## 5. 与 Swagger 的分工

- `/swagger`
  - 继续作为研发调试页和快速联调入口。
- `/scalar`
  - 作为更正式的 API Reference 展示入口。
- `docs/api/*.md`
  - 继续承担业务语义、状态机、SSE、失败收口和排障说明。

## 6. 验收标准

- `GET /swagger` 返回 200。
- `GET /scalar` 返回 200。
- `GET /openapi.yaml` 返回 200。
- 页面里的协议地址仍然是 `/openapi.yaml`。
- 仓库内不再保留独立 Scalar 展示服务的构建文件。
