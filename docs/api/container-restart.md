# POST /api/v1/edge/containers/{slotId}/restart

## 当前状态

- 正式协议已收口
- 当前新 Client 代码暂未实现

## 功能目标

重启指定 `slot` 对应的本机容器。

## 业务边界

- 这是内网运维诊断接口
- 不替代 `browser-env run/stop`
- 不保证环境包配置与 SQLite 运行态完整回写

## 前置校验

- `slotId` 必须存在
- Docker 必须可达

## 请求与响应

### 请求

```http
POST /api/v1/edge/containers/slot001/restart
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
    "taskId": "edge-task-container-restart-001",
    "taskType": "docker_container_restart",
    "resourceType": "docker_container",
    "resourceId": "slot001",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-container-restart-001/events"
  }
}
```

## 状态流转

- 创建短期 task
- 后台调用 Docker restart

## SSE 说明

- 本接口使用 SSE

## 任务编排

- `load_slot`
- `check_docker`
- `restart_container`
- `finalize_success`
- `finalize_failed`

## 成功判定

- 容器重启成功

## 失败判定

- `slotId` 不存在
- Docker restart 失败

## 日志字段

- `action=restart-slot-container`
- `taskId`
- `slotId`
- `timeoutSeconds`
- `error`

## 联调验收标准

- 能从 SSE 看到任务过程
- 成功后容器重新进入 running
