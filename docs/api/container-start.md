# POST /api/v1/edge/containers/{slotId}/start

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

启动指定 `slot` 对应的本机容器。

## 业务边界

- 这是内网运维诊断接口
- 只面向 slot 容器，不直接面向 browser-env 资产
- 不读取环境包资产
- 不保证 SQLite 生命周期状态完整同步
- 不应作为平台业务主入口

## 前置校验

- `slotId` 必须存在
- Docker 必须可达

## 请求与响应

### 请求

```http
POST /api/v1/edge/containers/slot001/start
```

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-container-start-001",
    "taskType": "docker_container_start",
    "resourceType": "docker_container",
    "resourceId": "slot001",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-container-start-001/events"
  }
}
```

## 状态流转

- 创建短期 task
- 后台调用 Docker start

## SSE 说明

- 本接口使用 SSE
- 只立即返回 `taskId/eventsUrl`

## 任务编排

- `load_slot`
- `check_docker`
- `start_container`
- `finalize_success`
- `finalize_failed`

## 成功判定

- 容器启动成功

## 失败判定

- `slotId` 不存在
- Docker start 失败

## 日志字段

- `action=start-slot-container`
- `taskId`
- `slotId`
- `error`

## 联调验收标准

- 成功后容器状态变为 running
- 不误承诺 browser-env 已业务可用
