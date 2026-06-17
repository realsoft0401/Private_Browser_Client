# POST /api/v1/edge/browser-envs/{envId}/run

## 功能目标

启动指定环境包，让该环境包在当前 Client 本机进入可运行状态。

这条接口的真正目标不是“把容器跑起来”，而是：

- 读取并校验完整环境包资产
- 确认 Docker 与镜像条件满足
- 将环境包加载到指定 slot
- 启动浏览器运行容器
- 等待浏览器和 CDP 就绪
- 完成 timezone / proxy 出口 / runtime protection 校验
- 最终把环境包主状态推进到 `running` 或失败收口

## 业务边界

- 负责环境包资产加载、运行校验、CDP 检查、运行保护验证、SSE 任务推进
- 不负责平台商业额度决策
- 不负责自动更换 Client
- 不接受任意 Docker 参数透传
- 不接受请求体临时覆盖镜像、代理或指纹关键配置

## 身份与路径

- `envId` 格式固定为 `userId_rpaType_snowflakeId`
- 环境目录固定为 `data/browser-envs/users/{userId}/{rpaType}/{envId}`
- 读取入口统一先查 SQLite 索引，再读 `profile.json`

示例：

```text
906090001_tk_324867594169356288
data/browser-envs/users/906090001/tk/906090001_tk_324867594169356288
```

## 前置校验

- `envId` 存在于本机索引
- `slotId` 必填
- slot 存在且可用
- Docker 可达
- 环境包目录存在
- `profile.json`、`binding.json`、`proxy`、`fingerprint`、`browser-data/profile` 原子材料完整
- `runtime.image` 可用
- 环境包当前状态允许 run
- 同一 `envId` 当前没有并发 run

## 请求与响应

### 请求

```http
POST /api/v1/edge/browser-envs/906090001_tk_324867594169356288/run
Content-Type: application/json
```

```json
{
  "slotId": "slot001",
  "forceRecreate": false
}
```

正式请求体只收这两个字段：

- `slotId`
  - 必填
  - 表示这次明确要把环境包加载到哪个 slot
  - run 不能省略 slot，也不能让 Client 自己偷选 slot
- `forceRecreate`
  - 选填，默认 `false`
  - 表示即使当前已有可复用容器，也按重建容器链路执行
  - 它只允许重建容器，不允许删除 `browser-data/profile`

明确不允许的请求体扩展：

- 不接受 `image`
- 不接受 `clientId`
- 不接受 `language`
- 不接受临时 `proxy`
- 不接受临时 `fingerprint`
- 不接受任意 Docker 参数透传

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-run-001",
    "taskType": "browser_env_run",
    "resourceType": "browser_env",
    "resourceId": "906090001_tk_324867594169356288",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-run-001/events"
  }
}
```

## 状态流转

- 环境包主状态：`created` / `stopped` / `error` -> `running` 或 `error`
- slot 状态：`waiting -> loading -> occupied`
- 内部加载关系：`loading -> running`
- `runtimeProtection/proxyRuntime`：`pending -> success/failed`

关键原则：

- 容器 `running` 不等于环境可用
- 成功必须包含 CDP 可用与运行保护验证通过
- 如果容器已经启动，但 CDP / timezone / proxyRuntime / runtimeProtection 校验失败，本次 run 仍必须 failed，且 `browser_envs.status` 收口到 `error`

## 任务编排

后台主链路建议：

1. `validate_env_index`
2. `load_profile`
3. `validate_atomic_materials`
4. `check_slot`
5. `check_runtime_image`
6. `pull_runtime_image`
7. `prepare_container`
8. `start_container`
9. `wait_browser`
10. `cdp_check`
11. `timezone_probe`
12. `runtime_protection_update`
13. `finalize_success` / `finalize_failed`

## SSE 说明

- 本接口应使用 SSE
- 原因：多阶段、长链路、阶段性失败差异明显，同步 HTTP 无法充分表达过程
- 必须返回 `taskId/eventsUrl`

SSE 事件至少包含：

- `taskId`
- `envId`
- `slotId`
- `stage`
- `status`
- `message`
- `error`
- `suggestion`
- `timestamp`

推荐固定事件字段：

- `event`
  - 事件类型，建议固定为 `task.progress`、`task.completed`、`task.failed`
- `taskId`
- `taskType`
  - 固定为 `browser_env_run`
- `resourceType`
  - 固定为 `browser_env`
- `resourceId`
  - 即 `envId`
- `envId`
- `slotId`
- `stage`
- `status`
  - 建议值：`queued`、`running`、`success`、`failed`
- `message`
- `error`
- `suggestion`
- `timestamp`

推荐 `stage` 枚举：

- `validate_env_index`
- `load_profile`
- `validate_atomic_materials`
- `check_slot`
- `check_runtime_image`
- `pull_runtime_image`
- `prepare_container`
- `start_container`
- `wait_browser`
- `cdp_check`
- `timezone_probe`
- `runtime_protection_update`
- `finalize_success`
- `finalize_failed`

SSE 示例：

```text
event: task.progress
data: {"taskId":"edge-task-run-001","taskType":"browser_env_run","resourceType":"browser_env","resourceId":"906090001_tk_324867594169356288","envId":"906090001_tk_324867594169356288","slotId":"slot001","stage":"start_container","status":"running","message":"container started, waiting browser bootstrap","timestamp":"2026-06-17T15:20:31+08:00"}

event: task.progress
data: {"taskId":"edge-task-run-001","taskType":"browser_env_run","resourceType":"browser_env","resourceId":"906090001_tk_324867594169356288","envId":"906090001_tk_324867594169356288","slotId":"slot001","stage":"cdp_check","status":"running","message":"checking cdp endpoint","timestamp":"2026-06-17T15:20:36+08:00"}

event: task.completed
data: {"taskId":"edge-task-run-001","taskType":"browser_env_run","resourceType":"browser_env","resourceId":"906090001_tk_324867594169356288","envId":"906090001_tk_324867594169356288","slotId":"slot001","stage":"finalize_success","status":"success","message":"browser env is ready","timestamp":"2026-06-17T15:20:40+08:00"}
```

SSE 中断后的收口规则：

- SSE 中断不等于成功
- 最终必须重新确认环境包事实
- 能确认环境包已进入 `running` 且运行保护通过才记成功
- 无法确认、状态冲突、容器异常、CDP 不可用、运行保护失败都按失败收口

## 成功判定

- 环境包索引存在
- 原子材料完整且一致
- Docker 可达
- 镜像可用
- slot 可用
- 容器成功启动
- CDP 可用
- runtime protection 校验通过
- 环境包主状态进入 `running`

## 失败判定

- `envId` 不存在
- `slotId` 缺失
- 环境包目录不存在
- `profile.json` / `binding.json` / `proxy` / `fingerprint` / `browser-data/profile` 缺失或损坏
- slot 不存在或不可用
- Docker 不可达
- 镜像缺失且拉取失败
- 容器创建或启动失败
- CDP 不可用
- timezone / proxy 出口 / runtime protection 验证失败

## 日志字段

- `action=run-browser-env`
- `taskId`
- `envId`
- `slotId`
- `envPath`
- `runtimeImage`
- `containerName`
- `containerId`
- `stage`
- `status`
- `error`
- `suggestion`

## 联调验收标准

- 返回 `taskId/eventsUrl`
- SSE 能看到完整启动阶段
- 成功后环境包状态为 `running`
- CDP 与 runtime protection 必须都通过
- SSE 断流不能默认成功
