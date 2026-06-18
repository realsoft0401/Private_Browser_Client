# POST /api/v1/edge/containers/{slotId}/stop

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

停止指定 `slot` 对应的本机容器。

## 业务边界

- 这是内网运维诊断接口
- 不等价于 `browser-env stop`
- 不负责 `browser-data/profile` 资产语义
- 不保证包侧主状态完整同步

## 前置校验

- `slotId` 必须存在
- Docker 必须可达

## 请求与响应

### 请求

```http
POST /api/v1/edge/containers/slot001/stop
Content-Type: application/json
```

```json
{
  "timeoutSeconds": 10
}
```

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-container-stop-001",
    "taskType": "docker_container_stop",
    "resourceType": "docker_container",
    "resourceId": "slot001",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-container-stop-001/events"
  }
}
```

## 状态流转

- 创建短期 task
- 后台调用 Docker stop

## SSE 说明

- 本接口使用 SSE

## 任务编排

- `load_slot`
- `check_docker`
- `stop_container`
- `finalize_success`
- `finalize_failed`

## 成功判定

- 容器停止成功

## 失败判定

- `slotId` 不存在
- Docker stop 失败

## 日志字段

- `action=stop-slot-container`
- `taskId`
- `slotId`
- `timeoutSeconds`
- `error`

## 联调验收标准

- 成功后容器状态变为 exited 或等价非运行态
- 不把裸容器 stop 误当成环境包主状态已同步
