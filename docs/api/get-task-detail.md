# GET /api/v1/edge/tasks/{taskId}

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

查询当前 Client 进程内短期任务的详情摘要，给前端刷新页面、Node Server 排障和管理员查看任务终态使用。

这条接口的定位是“任务摘要查询”，不是事件流本身。真正多阶段过程仍以 SSE 为准。

## 业务边界

- 负责返回当前进程内 task 摘要
- 负责返回任务终态、当前阶段、错误信息、时间戳
- 负责说明该 task 是否仍可继续订阅 SSE
- 不替代 `GET /api/v1/edge/tasks/{taskId}/events`
- 不承担长期持久化
- Client 重启后不保证还能查到旧 task

## 前置校验

- `taskId` 必须合法
- 任务必须仍存在于当前 Client 进程可见范围

## 请求与响应

### 请求

```http
GET /api/v1/edge/tasks/edge-task-run-001
Accept: application/json
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-run-001",
    "taskType": "browser_env_run",
    "resourceType": "browser_env",
    "resourceId": "906090001_tk_324867594169356288",
    "status": "success",
    "currentStage": "finalize_success",
    "message": "browser env is ready",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-run-001/events",
    "createdAt": "2026-06-17T15:20:31+08:00",
    "updatedAt": "2026-06-17T15:20:40+08:00",
    "finishedAt": "2026-06-17T15:20:40+08:00",
    "error": "",
    "suggestion": ""
  }
}
```

## 状态流转

- 本接口只读，不改变 task 状态
- 建议终态只收口为 `success` 或 `failed`
- 中间态可返回 `queued`、`running`

## SSE 说明

- 本接口本身不用 SSE
- 但返回值里应包含 `eventsUrl`
- 如果任务仍在执行中，调用方应继续订阅 `GET /api/v1/edge/tasks/{taskId}/events`

## 任务编排

- 本接口不创建 task
- 只读取已存在 task 的当前进程内摘要

## 成功判定

- 能查到当前 task 摘要
- 终态、阶段、错误信息与 SSE 已发生事实一致

## 失败判定

- `taskId` 不存在
- Client 重启导致历史内存任务已丢失

## 日志字段

- `action=get-task-detail`
- `taskId`
- `taskType`
- `resourceType`
- `resourceId`
- `status`
- `error`

## 联调验收标准

- 前端刷新后能先查 task 摘要，再决定是否继续订阅 SSE
- 查询不到旧 task 时，返回结果必须明确是“当前 Client 进程无此任务”
- 不得把 task 详情接口写成事件流接口
