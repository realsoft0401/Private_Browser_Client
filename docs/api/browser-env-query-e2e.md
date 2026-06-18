# Browser Env Query E2E

## 1. 适用范围

这份用例只验证这 3 个新补齐并已落地的查询接口：

- `GET /api/v1/edge/browser-envs`
- `GET /api/v1/edge/browser-envs/{envId}`
- `GET /api/v1/edge/tasks/{taskId}`

补充说明：

- `GET /api/v1/edge/tasks/{taskId}/events` 虽然不是这次新增的普通查询接口，但它和 `task detail` 是一组联调口径，所以本用例会一起校验
- 这份用例不覆盖 `create/import-package/backup/restore/delete` 的完整生命周期，它只验证“查询面”是否能把当前本机事实正确返回出来

## 2. 测试目标

- 验证 browser-env 列表能返回分页、统计和索引摘要
- 验证 browser-env 详情能组合 `index/profile/binding/container/proxy/fingerprint/consistency`
- 验证运行中详情能补齐 `vncUrl/vncWsUrl/webVncUrl`
- 验证 task detail 能返回当前阶段、终态、时间戳和 `eventsUrl`
- 验证 task detail 与 SSE 事件流的最终阶段一致

## 3. 测试前提

当前测试基于本机已存在的一条标准环境包：

- `envId=318275706305908736_tk_319725200528642048`
- `slotId=slot001`

如果你换了机器或清空了数据，请先准备：

- 至少 1 条已创建的 browser env
- 至少 1 个可用 slot
- 能触发一次 `run`，用于生成 task

## 4. 统一变量

```bash
export CLIENT_BASE="http://127.0.0.1:3300"
export CLIENT_DB="/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/data/private_browser_client.db"

export ENV_ID="318275706305908736_tk_319725200528642048"
export SLOT_ID="slot001"
```

## 5. 测试前检查

```bash
curl -s "$CLIENT_BASE/health" | jq
curl -s -o /dev/null -w '%{http_code}\n' "$CLIENT_BASE/swagger"
curl -s -o /dev/null -w '%{http_code}\n' "$CLIENT_BASE/openapi.yaml"
sqlite3 "$CLIENT_DB" ".tables"
```

通过标准：

- `/health` 返回 `code=1000`
- `/swagger` 返回 `200`
- `/openapi.yaml` 返回 `200`
- SQLite 至少存在 `browser_envs`、`slots`、`runtime_relations`

## 6. Step 1: 查询 browser-env 列表

```bash
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs" | jq
```

当前实测结果应至少具备这些特征：

- `code=1000`
- `data.total >= 1`
- `data.page=1`
- `data.pageSize=20`
- `data.byStatus` 有值
- `data.byRpaType.tk >= 1`
- `data.items[0].envId` 能看到当前环境包

本轮实测样例：

```json
{
  "total": 1,
  "page": 1,
  "pageSize": 20,
  "byStatus": {
    "created": 1
  },
  "byRpaType": {
    "tk": 1
  }
}
```

继续校验筛选：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs?userId=318275706305908736" | jq '.data.total'
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs?rpaType=tk" | jq '.data.total'
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs?status=created" | jq '.data.byStatus'
```

通过标准：

- 按 `userId/rpaType/status` 查询不报错
- 列表项不泄露 `proxy/clash.yaml` 明文
- 列表项不返回 `browser-data/profile` 具体内容

## 7. Step 2: 查询单环境详情

```bash
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID" | jq
```

通过标准：

- `code=1000`
- `data.index.envId == $ENV_ID`
- `data.profile.envId == $ENV_ID`
- `data.binding.identity.envId == $ENV_ID`
- `data.proxy.configPath == "proxy/clash.yaml"`
- `data.consistency.errors` 是数组

建议单独看几个关键块：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID" | jq '.data.index'
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID" | jq '.data.profile.runtime'
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID" | jq '.data.binding'
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID" | jq '.data.consistency'
```

当前实测重点：

- 详情已成功组合 `index/profile/binding/container/proxy/fingerprint/consistency`
- 未运行时，`index.status` 可能是 `created`
- 已运行时，`index` 会补齐：
  - `vncUrl`
  - `vncWsUrl`
  - `webVncUrl`

## 8. Step 3: 触发 run，生成 task

这一步是为了验证 `GET /tasks/{taskId}`，因为 task 详情只存在于当前 Client 进程内。

```bash
RUN_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/run" \
  -H "Content-Type: application/json" \
  -d "{
    \"slotId\": \"$SLOT_ID\",
    \"forceRecreate\": false
  }")"

printf '%s\n' "$RUN_RESP" | jq
export RUN_TASK_ID="$(printf '%s' "$RUN_RESP" | jq -r '.data.taskId')"
echo "$RUN_TASK_ID"
```

通过标准：

- `code=1000`
- `taskId` 非空
- `taskType=browser_env_run`
- `eventsUrl` 非空

## 9. Step 4: 查询 task detail

```bash
curl -s "$CLIENT_BASE/api/v1/edge/tasks/$RUN_TASK_ID" | jq
```

当前实测样例：

```json
{
  "taskId": "edge-task-1781696094373940000",
  "taskType": "browser_env_run",
  "resourceType": "browser_env",
  "resourceId": "318275706305908736_tk_319725200528642048",
  "status": "success",
  "currentStage": "finalize_success",
  "message": "browser env is ready",
  "eventsUrl": "/api/v1/edge/tasks/edge-task-1781696094373940000/events",
  "createdAt": "2026-06-17T19:34:54+08:00",
  "updatedAt": "2026-06-17T19:34:54+08:00",
  "finishedAt": "2026-06-17T19:34:54+08:00"
}
```

通过标准：

- `status` 能返回 `queued/running/success/failed`
- `currentStage` 与最近一次 SSE 事件一致
- `eventsUrl` 指向 `/api/v1/edge/tasks/{taskId}/events`

重要说明：

- 这条接口不是 SSE
- 它只返回当前 Client 进程内的任务摘要
- 如果 Client 重启，这个 `taskId` 可能查不到，这是当前设计允许的

## 10. Step 5: 校验 SSE 事件流

这是带 SSE 的接口，必须明确标注：

- `GET /api/v1/edge/tasks/{taskId}/events`
- `Content-Type=text/event-stream`

订阅命令：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$RUN_TASK_ID/events"
```

当前实测结果：

```text
event: task.progress
data: {"stage":"validate_env_index","status":"queued"}

event: task.progress
data: {"stage":"start_container","status":"running"}

event: task.completed
data: {"stage":"finalize_success","status":"success"}
```

通过标准：

- 至少能看到 `task.progress`
- 成功链路最终是 `task.completed`
- 失败链路最终是 `task.failed`
- 最后一条 SSE 事件的 `stage/status` 必须和 `GET /tasks/{taskId}` 一致

## 11. Step 6: 运行中再次查询详情，确认连接地址

```bash
curl -s "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID" | jq '.data.index'
curl -s "$CLIENT_BASE/api/v1/edge/slots/$SLOT_ID/vnc-info" | jq
```

当前实测重点：

- `index.status=running`
- `index.containerStatus=running`
- `index.vncUrl=vnc://127.0.0.1:9101`
- `index.vncWsUrl=ws://127.0.0.1:3300/api/v1/edge/slots/slot001/vnc/ws`
- `index.webVncUrl=http://127.0.0.1:3300/web-vnc.html?slot=slot001`

补充验证：

```bash
open "http://127.0.0.1:3300/web-vnc.html?slot=$SLOT_ID"
```

说明：

- 这里验证的是“详情接口能否把运行态连接地址正确拼出来”
- 是否能看到桌面画面，还取决于 slot 容器内 VNC 服务是否正常

## 12. Step 7: SQLite 回看事实

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,container_status,last_error,last_started_at,last_stopped_at
FROM browser_envs
WHERE env_id='$ENV_ID';
"

sqlite3 -header -column "$CLIENT_DB" "
SELECT slot_id,status,current_package_id,current_run_id,updated_at
FROM slots
WHERE slot_id='$SLOT_ID';
"

sqlite3 -header -column "$CLIENT_DB" "
SELECT run_id,package_id,slot_id,status,last_error,started_at,updated_at
FROM runtime_relations;
"
```

当前实测结果：

- `browser_envs.status=running`
- `browser_envs.container_status=running`
- `slots.status=occupied`
- `slots.current_package_id=$ENV_ID`
- `runtime_relations.status=running`

## 13. Step 8: 收尾 stop

为了避免把测试环境一直挂在运行态，最后执行一次 stop：

```bash
curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/stop" \
  -H "Content-Type: application/json" \
  -d "{}" | jq
```

建议继续确认：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$SLOT_ID" | jq
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,container_status,last_stopped_at
FROM browser_envs
WHERE env_id='$ENV_ID';
"
```

预期：

- `slot.status=waiting`
- `browser_envs.status=stopped`
- slot 恢复为空白 waiting 容器，不继续挂着旧包

## 14. 常见误判

- `GET /api/v1/edge/tasks/{taskId}` 返回不存在，不一定是实现有问题，也可能是 Client 进程重启后内存任务丢失
- `browser-env detail` 没有 `webVncUrl`，不一定是 bug，也可能是当前 env 根本没处于运行态
- 列表里看不到某条 env，不一定是查询坏了，也可能是你传了 `status` 过滤条件

## 15. 本轮实测结论

以 `2026-06-17` 本机实测结果为准，这 3 组新能力已经验证通过：

- `GET /api/v1/edge/browser-envs`
- `GET /api/v1/edge/browser-envs/{envId}`
- `GET /api/v1/edge/tasks/{taskId}`

并且配套的 SSE 联调链路已确认可用：

- `GET /api/v1/edge/tasks/{taskId}/events`
