# POST /api/v1/edge/docker/pull-image

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

在当前 Client 本机拉取指定 Docker 镜像。

## 业务边界

- 只在本机执行 Docker pull
- 不写 SQLite
- 不决定商业镜像策略
- 不自动替换 tag

## 前置校验

- `image` 不能为空
- Docker 必须可达

## 请求与响应

### 请求

```http
POST /api/v1/edge/docker/pull-image
Content-Type: application/json
```

```json
{
  "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64"
}
```

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-pull-image-001",
    "taskType": "docker_pull_image",
    "resourceType": "docker_image",
    "resourceId": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-pull-image-001/events"
  }
}
```

## 状态流转

- 创建本机短期 task
- 后台执行 Docker pull
- 最终通过 SSE 收口为 `success/failed`

## SSE 说明

- 本接口使用 SSE
- 当前只立即返回 `taskId/eventsUrl`
- 最终结果必须继续订阅 `/api/v1/edge/tasks/{taskId}/events`

## 任务编排

- `validate_request`
- `check_docker`
- `pull_image`
- `stream_progress`
- `finalize_success`
- `finalize_failed`

## 成功判定

- 镜像拉取完成

## 失败判定

- 镜像名非法
- Docker 不可达
- 仓库认证失败
- 网络拉取失败

## 日志字段

- `action=pull-docker-image`
- `taskId`
- `image`
- `error`

## 联调验收标准

- 创建后能从 SSE 看到拉取进度
- 失败时保留 Docker 返回错误
