# GET /api/v1/edge/tasks/{taskId}/events

## 功能目标

为长链路任务提供统一 SSE 订阅入口，让前端、Node Server 和管理员能够看到多阶段进度、失败阶段和最终结论。

这份文档不绑定某一个具体业务动作，而是为下面这些正式任务接口提供统一事件协议：

- `browser-envs/{envId}/run`
- `browser-envs/{envId}/backup`
- `browser-envs/{envId}/restore`
- `browser-envs/{envId}/revalidate`
- `browser-envs/import-package`
- 以及后续确认需要任务化的 delete / proxy update 等动作

## 业务边界

- 负责输出任务进度事件
- 负责输出最终 success / failed 结论
- 不负责创建任务
- 不负责替代资源详情接口
- SSE 断流不等于任务成功

## 前置校验

- `taskId` 必须存在
- `taskId` 必须属于当前 Client 可见任务
- 调用方必须接受 `text/event-stream`

## 请求与响应

### 请求

```http
GET /api/v1/edge/tasks/edge-task-run-001/events
Accept: text/event-stream
```

### 响应类型

```http
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

## SSE 说明

- 本接口本身就是统一 SSE 订阅入口
- 只有已经返回 `taskId/eventsUrl` 的长链路接口才应该使用它

## 任务编排

- 本接口不创建 task
- 它只消费和转发既有 task 的事件流

## 统一事件字段

每条事件的 `data` 统一建议至少包含：

- `event`
- `taskId`
- `taskType`
- `resourceType`
- `resourceId`
- `stage`
- `status`
- `message`
- `error`
- `suggestion`
- `timestamp`

如果该任务天然关联 `envId`、`slotId`，也应带上：

- `envId`
- `slotId`

## 统一枚举

### event

- `task.progress`
- `task.completed`
- `task.failed`

### status

- `queued`
- `running`
- `success`
- `failed`

## 示例

```text
event: task.progress
data: {"event":"task.progress","taskId":"edge-task-run-001","taskType":"browser_env_run","resourceType":"browser_env","resourceId":"906090001_tk_324867594169356288","envId":"906090001_tk_324867594169356288","slotId":"slot001","stage":"start_container","status":"running","message":"container started, waiting browser bootstrap","timestamp":"2026-06-17T15:20:31+08:00"}

event: task.failed
data: {"event":"task.failed","taskId":"edge-task-run-001","taskType":"browser_env_run","resourceType":"browser_env","resourceId":"906090001_tk_324867594169356288","envId":"906090001_tk_324867594169356288","slotId":"slot001","stage":"timezone_probe","status":"failed","message":"timezone probe failed","error":"all providers failed","suggestion":"check proxy route and container outbound network","timestamp":"2026-06-17T15:20:39+08:00"}
```

## 成功判定

- 不是看 SSE 有没有连上
- 不是看中间有没有 progress
- 只看最终是否收到 `task.completed`，并且资源事实后验一致

## 失败判定

下面这些情况都不能按成功处理：

- 收到 `task.failed`
- SSE 中途断开且无法确认最终资源事实
- 资源详情回读与事件最终结论冲突
- Edge 重启、task 丢失或长时间无新事件，导致最终结论无法确认

## 日志字段

- `action=subscribe-task-events`
- `taskId`
- `taskType`
- `resourceType`
- `resourceId`
- `remoteAddr`
- `error`

## 收口原则

- SSE 只表达进度，不单独构成业务真相源
- 最终成功或失败，仍要以任务终态加资源事实核对为准
- `run` 这类动作如果 Docker 容器已经启动，但最终 `task.failed`，仍要把业务口径判成失败

## 联调验收标准

- 能持续收到 `task.progress`
- 任务结束时能收到 `task.completed` 或 `task.failed`
- SSE 断流后不能默认把任务判成功
