# Edge API 设计：`POST /api/v1/edge/docker/pull-image`

## 1. 功能目标

`POST /api/v1/edge/docker/pull-image` 用于在当前 Edge 节点本机拉取指定 Docker 镜像。

## 2. 业务边界

- 只在本机执行 Docker pull
- 不写 SQLite
- 不决定商业镜像策略
- 不因为上层没传对镜像就自动替换 tag

## 3. 请求与响应

```http
POST /api/v1/edge/docker/pull-image
```

请求重点：

- `image`
- `tag`

成功响应先返回：

- `taskId`
- `taskType=docker_pull_image`
- `resourceType=docker_image`
- `eventsUrl`

## 4. 前置校验

- `image` 不能为空
- Docker 必须可达

## 5. 状态流转

- 创建本机内存 Edge task
- 后台执行 Docker pull
- 最终通过 `done/error` 事件收口

## 6. 任务编排

```text
create task
  -> docker pull
  -> stream layer progress
  -> done or error
```

## 7. 成功判定

- 镜像拉取完成
- 任务进入 `done`

## 8. 失败判定

- 镜像名非法
- Docker 不可达
- 仓库认证或网络失败

## 9. 日志字段

- `taskId`
- `taskType`
- `image`
- `tag`
- `error`

## 10. 联调验收标准

- 创建后能从 SSE 看到拉取进度
- 失败时 `error` 事件保留 Docker 返回信息
