# POST /api/v1/edge/docker/remove-image

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

在当前 Client 本机删除指定 Docker 镜像。

## 业务边界

- 只作用于本机 Docker 镜像
- 不删除环境包目录
- 不删除 SQLite 索引
- 不自动修复环境包引用关系

## 前置校验

- `image` 不能为空
- Docker 必须可达

## 请求与响应

### 请求

```http
POST /api/v1/edge/docker/remove-image
Content-Type: application/json
```

```json
{
  "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
  "force": false,
  "noPrune": false
}
```

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-remove-image-001",
    "taskType": "docker_remove_image",
    "resourceType": "docker_image",
    "resourceId": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-remove-image-001/events"
  }
}
```

## 状态流转

- 创建本机短期 task
- 后台执行 Docker remove image
- 最终通过 SSE 收口

## SSE 说明

- 本接口使用 SSE
- 当前只立即返回 `taskId/eventsUrl`
- 最终结果必须继续订阅 `/api/v1/edge/tasks/{taskId}/events`

## 任务编排

- `validate_request`
- `check_docker`
- `remove_image`
- `finalize_success`
- `finalize_failed`

## 成功判定

- Docker 删除成功

## 失败判定

- 镜像不存在
- 镜像仍被容器引用
- Docker 删除失败

## 日志字段

- `action=remove-docker-image`
- `taskId`
- `image`
- `force`
- `noPrune`
- `error`

## 联调验收标准

- 删除成功后镜像列表不再显示该标签
- 被引用镜像失败时能保留明确错误
