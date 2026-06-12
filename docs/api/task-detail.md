# Edge API 设计：`GET /api/v1/edge/tasks/{taskId}`

## 1. 功能目标

`GET /api/v1/edge/tasks/{taskId}` 用于读取当前 Edge 节点内存中的任务详情。

## 2. 业务边界

- Edge task 只做本机短期观察
- 服务重启后任务不会恢复
- 不替代 `Private_Browser_Server` 的平台持久任务

## 3. 请求与响应

```http
GET /api/v1/edge/tasks/{taskId}
```

返回重点：

- `taskId`
- `taskType`
- `status`
- `resourceType`
- `resourceId`
- `message`
- `result`
- `error`

## 4. 前置校验

- `taskId` 必须存在于当前进程内存

## 5. 状态流转

- 只读，不改状态

## 6. 成功判定

- 能读取到任务详情

## 7. 失败判定

- 任务不存在
- 服务重启后任务丢失

## 8. 日志字段

- `taskId`
- `taskType`
- `status`
- `error`

## 9. 联调验收标准

- Docker 任务、环境包任务都能通过该接口查看即时状态
- 服务重启后任务丢失时必须明确失败，不伪造历史
