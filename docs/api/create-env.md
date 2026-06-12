# Edge API 设计：`POST /api/v1/edge/browser-envs`

## 1. 功能目标

`POST /api/v1/edge/browser-envs` 用于在当前 Edge 节点本机创建一个新的浏览器环境包。

## 2. 业务边界

- 只创建本机环境包目录、原子文件和 SQLite 索引
- 不启动 Docker 容器
- 不决定镜像策略
- `userId` 只是业务标识，不是权限判断

## 3. 请求与响应

```http
POST /api/v1/edge/browser-envs
```

请求重点：

- `userId`
- `rpaType`
- `name`
- `runtime.image`
- `environment`
- `proxy`

成功返回重点：

- `envId`
- `envSequence`
- `ports`
- `envPath`
- `files`
- `identityHash`

## 4. 前置校验

- `userId` 必须通过格式和路径安全校验
- `rpaType` 必须合法
- `runtime.image` 必须存在
- `environment` 关键字段必须完整
- 本机端口分配不能冲突

## 5. 状态流转

- 创建成功后主状态进入 `created`
- `containerStatus` 初始不是运行态
- 不创建 Edge task

## 6. 成功判定

- 环境包目录和原子文件创建成功
- SQLite 索引写入成功
- `envId`、端口和 `identityHash` 已生成

## 7. 失败判定

- 参数非法
- 目录或端口冲突
- 文件写入失败
- SQLite 写入失败

## 8. 日志字段

- `envId`
- `userId`
- `rpaType`
- `runtime.image`
- `error`

## 9. 联调验收标准

- 创建后列表接口能看到 `created` 环境包
- 不会因为创建成功就自动 run
- 不返回 proxy 明文和 fingerprint raw
