# POST /api/v1/edge/browser-envs/{envId}/revalidate

## 功能目标

对异常环境包执行受控重新校验，把它从异常态恢复到可再次尝试运行的待启动状态，但不启动容器。

## 业务边界

- 负责重新校验身份、一致性、原子材料、Docker 冲突、端口占用
- 允许修复少量可受控的非身份类运行摘要
- 成功后把环境恢复为 `created` 或 `stopped`
- 成功后把 `runtimeProtection/proxyRuntime` 改为 `pending`
- 不启动容器
- 不自动 run
- 不修复身份字段
- 不伪造登录态、proxy 或 fingerprint

## 前置校验

- `envId` 存在
- 当前状态主口径应为 `error`
- `profile.json`、`binding.json`、`proxy`、`fingerprint`、`browser-data/profile` 必须存在且可信
- 节点本机健康允许执行受控校验

## 请求与响应

### 请求

```http
POST /api/v1/edge/browser-envs/906090001_tk_324867594169356288/revalidate
```

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-revalidate-001",
    "taskType": "browser_env_revalidate",
    "resourceType": "browser_env",
    "resourceId": "906090001_tk_324867594169356288",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-revalidate-001/events"
  }
}
```

## 状态流转

- revalidate 前：`error`
- 成功后：
  - 无有效容器时恢复为 `created`
  - 存在本环境已停止容器时恢复为 `stopped`
- `runtimeProtection/proxyRuntime` 改为 `pending`
- 不自动进入 `running`

## SSE 说明

- 本接口应使用 SSE
- 原因：涉及多阶段文件校验、Docker 事实校验、端口冲突处理、摘要回写

阶段建议：

- `queued`
- `load_env_index`
- `validate_status`
- `load_profile`
- `load_binding`
- `validate_identity`
- `validate_proxy_materials`
- `validate_fingerprint_materials`
- `validate_browser_data`
- `load_container_summary`
- `check_docker_facts`
- `check_port_conflicts`
- `reassign_ports`
- `write_runtime_summaries`
- `reset_runtime_protection`
- `update_env_status`
- `finalize_success`
- `finalize_failed`

SSE 中断后的收口规则：

- 不默认成功
- 必须确认环境状态已恢复为 `created/stopped` 且 `runtimeProtection/proxyRuntime` 已改为 `pending`

## 任务编排

- 本接口创建长链路 task
- 调用方通过 `taskId/eventsUrl` 观察重新校验、端口收口和运行摘要回写过程

## 成功判定

- 身份一致性通过
- 原子材料完整且可解析
- 没有严重 Docker 容器冲突
- 允许范围内的端口问题已收口
- 运行摘要回写成功
- 环境状态恢复为 `created` 或 `stopped`

## 失败判定

- 身份不一致
- 原子材料缺失或损坏
- browser-data/profile 不可信
- Docker 容器严重冲突
- 关键文件回写失败
- 最终事实无法确认

## 日志字段

- `action=revalidate-browser-env`
- `taskId`
- `envId`
- `envPath`
- `stage`
- `statusBefore`
- `statusAfter`
- `reassignedPorts`
- `containerConflictType`
- `error`

## 联调验收标准

- 返回 `taskId/eventsUrl`
- SSE 能看到完整重新校验过程
- 成功后恢复为 `created` 或 `stopped`
- `runtimeProtection/proxyRuntime` 变为 `pending`
- 不能自动进入 `running`
