# Browser Env 全链路测试总览

## 1. 文档目标

这份文档不再承载单条详细执行步骤，而是作为 `browser-env` 测试总览，帮助区分两条正式测试路线。

当前必须拆成两种测试：

- 路线 A：正常配置文件创建流
- 路线 B：原始 `tgz` 包导入流

原因：

- 两条路线的入口不同
- 关键验证点不同
- `restore` 与 `import-package` 不适合再混成一条串行步骤

## 2. 两条路线的区别

### 2.1 路线 A：正常配置文件创建流

入口：

- `POST /api/v1/edge/browser-envs`

重点验证：

- 本机生成 `profile.json / binding.json / container.json`
- 本机生成 SQLite `browser_envs`
- create 后 run / stop / patch proxy / backup / restore / revalidate / delete 的完整延续性

适合场景：

- 新建一个全新环境
- 验证“从零创建”的真实落盘逻辑

详细文档：

- [browser-env-e2e-create-flow.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/browser-env-e2e-create-flow.md)

### 2.2 路线 B：原始 tgz 包导入流

入口：

- `POST /api/v1/edge/browser-envs/import-package`

重点验证：

- 外部 `tgz` 包结构是否合法
- 导入时身份一致性是否正确
- 导入时本机 `envSequence`、端口、运行摘要是否重分配
- 导入后 run / stop / backup / restore / delete 的延续性

适合场景：

- 手里已经有标准环境包
- 想验证老资产或迁移资产能否进入新 Client

详细文档：

- [browser-env-e2e-import-flow.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/browser-env-e2e-import-flow.md)

## 3. 两条路线共同规则

- 只使用当前新 Client 已开放的正式接口
- 凡是长链路、多阶段接口，必须同时检查 SSE
- 每一步都同时检查 API 返回、本机 SQLite、磁盘目录和 slot 当前态
- 失败时必须保留 `taskId`、SSE 事件、SQLite 记录和磁盘现场

## 4. 一定要标注 SSE 的接口

- `POST /api/v1/edge/browser-envs/{envId}/run`
- `POST /api/v1/edge/browser-envs/{envId}/backup`
- `POST /api/v1/edge/browser-envs/{envId}/restore`
- `POST /api/v1/edge/browser-envs/{envId}/revalidate`
- `POST /api/v1/edge/browser-envs/import-package`
- `DELETE /api/v1/edge/browser-envs/{envId}/package`

## 5. 不需要 SSE 的接口

- `POST /api/v1/edge/slots`
- `POST /api/v1/edge/browser-envs`
- `POST /api/v1/edge/browser-envs/{envId}/stop`
- `PATCH /api/v1/edge/browser-envs/{envId}/proxy`
- `GET /api/v1/edge/slots/*`

## 6. 共同观察点

- SQLite：
  - `browser_envs`
  - `slots`
  - `runtime_relations`
  - `package_runtime_views`
- 磁盘目录：
  - `data/browser-envs/users/{userId}/{rpaType}/{envId}`
- SSE：
  - `GET /api/v1/edge/tasks/{taskId}/events`

## 7. 当前建议

- 如果要验证“新建能力”，先跑路线 A
- 如果要验证“资产接入能力”，再单独跑路线 B
- 不建议把两条路线硬拼成一轮执行，否则很容易在 `restore / import-package / same envId conflict` 这些点上互相污染现场
- WebVNC 测试要拆成两层记录：
  - 页面入口与连接信息是否可访问
  - 真实桌面画面是否可见
- 当前如果 `slot runtime` 仍是占位镜像，第一层可以通过，第二层不应当作为当前阶段阻塞项
