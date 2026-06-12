# Edge API 设计：`DELETE /api/v1/edge/browser-envs/{envId}/del`

## 1. 功能目标

`DELETE /api/v1/edge/browser-envs/{envId}/del` 用于删除本机环境包 `profile.runtime.image` 对应的 Docker 镜像。

## 2. 业务边界

- 只删镜像
- 不删环境包目录
- 不删 `browser-data/profile`
- 不删 SQLite 索引
- 同步返回结果，不创建 Edge task

## 3. 请求与响应

```http
DELETE /api/v1/edge/browser-envs/{envId}/del
```

返回重点：

- `envId`
- `image`
- `imageRemoved`
- `results`
- `warningMessage`
- `deletedAt`

## 4. 前置校验

- 环境包必须存在
- 环境包不能处于 `running`
- 必须能从 `profile.json` 读到 `runtime.image`

## 5. 状态流转

- 不改变环境包主状态
- 有 warning 时只给出排障提示

## 6. 成功判定

- Docker 镜像删除成功
- 或同步返回受控 warning 结果

## 7. 失败判定

- 环境包正在运行
- `runtime.image` 缺失
- Docker 删除失败

## 8. 日志字段

- `envId`
- `image`
- `warningMessage`
- `error`

## 9. 联调验收标准

- 删除成功后环境包资产仍存在
- 被其他容器引用时 warning 语义明确
- 不误删环境包索引
