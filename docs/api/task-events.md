# Edge API 设计：`GET /api/v1/edge/tasks/{taskId}/events`

## 1. 功能目标

`GET /api/v1/edge/tasks/{taskId}/events` 用于订阅当前 Edge 节点任务的 SSE 事件流。

## 2. 业务边界

- 返回 `text/event-stream`
- 先补发历史事件，再推送实时事件
- 不替代任务详情接口
- 不替代中心持久任务

## 3. 请求与响应

```http
GET /api/v1/edge/tasks/{taskId}/events
Accept: text/event-stream
```

典型事件：

- `queued`
- `running`
- `progress`
- `heartbeat`
- `done`
- `error`

## 4. 前置校验

- `taskId` 必须存在于当前内存任务表

## 5. 状态流转

- 只输出事件，不直接修改任务状态

## 6. 成功判定

- SSE 建连成功
- 能收到历史或实时事件

## 7. 失败判定

- 任务不存在
- SSE 代理或写流失败

## 8. 日志字段

- `taskId`
- `event`
- `status`
- `error`

## 9. 联调验收标准

- 创建任务后立刻能订阅到历史事件
- `done/error` 事件能收口最终结果
