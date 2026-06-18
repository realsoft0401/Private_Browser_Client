# PATCH /api/v1/edge/browser-envs/{envId}/proxy-mode

## 当前状态

- 正式协议已收口
- 当前新 Client 代码暂未实现

## 功能目标

只切换环境包代理配置里的模式，不提交整份代理配置文件。

这条接口保留的原因，是为了让“改模式”和“改整份配置”边界清楚，避免只想切 `rule/global/direct` 时也必须上传整份 YAML。

## 业务边界

- 只修改 `proxy/clash.yaml` 顶层 `mode`
- 支持 `rule`、`global`、`direct`
- 负责递增 `binding.version`
- 负责把 `runtimeProtection/proxyRuntime` 标记为 `pending`
- 环境运行中且需要重建时，返回 `taskId/eventsUrl`
- 不接收整份代理配置
- 不改变 `identityHash`
- 不改变 `envId/userId/rpaType`

## 前置校验

- `envId` 必须存在
- `mode` 必须属于允许枚举
- `proxy/clash.yaml` 必须存在且可改写
- 环境包不能是 `deleted`

## 请求与响应

### 请求

```http
PATCH /api/v1/edge/browser-envs/906090001_tk_324867594169356288/proxy-mode
Content-Type: application/json
```

```json
{
  "mode": "global"
}
```

### 同步成功响应

环境未运行，或当前只需快速改写配置时：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "envId": "906090001_tk_324867594169356288",
    "mode": "global",
    "restartQueued": false,
    "runtimeProtectionStatus": "pending",
    "proxyRuntimeStatus": "pending"
  }
}
```

### 任务化成功响应

环境正在运行且需要重建容器时：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-proxy-mode-001",
    "taskType": "browser_env_proxy_mode_update",
    "resourceType": "browser_env",
    "resourceId": "906090001_tk_324867594169356288",
    "restartQueued": true,
    "eventsUrl": "/api/v1/edge/tasks/edge-task-proxy-mode-001/events"
  }
}
```

## 状态流转

- `binding.version` 递增
- `profile.updatedAt` 更新
- `runtimeProtection/proxyRuntime` 改为 `pending`
- 运行中环境如触发重建，则后续按受控重建链路重新验证

## SSE 说明

- 本接口不能默认使用 SSE
- 只有运行中环境触发明显多阶段重建时才使用 SSE
- 只是快速改模式时，普通 HTTP 即可

推荐阶段：

- `load_env`
- `read_proxy_config`
- `update_proxy_mode`
- `update_binding_version`
- `mark_runtime_pending`
- `prepare_restart`
- `restart_container`
- `timezone_probe`
- `proxy_runtime_probe`
- `runtime_protection_update`
- `finalize_success`
- `finalize_failed`

## 任务编排

- 同步模式：只改配置与摘要，不创建 task
- 任务模式：运行中环境触发重建时，返回 `taskId/eventsUrl`

## 成功判定

- `mode` 成功落盘
- `binding.version` 成功递增
- `runtimeProtection/proxyRuntime` 成功进入 `pending`
- 如进入任务模式，则后续重建验证通过

## 失败判定

- `envId` 不存在
- `mode` 非法
- 代理配置不存在或格式损坏
- 文件写入失败
- 容器重建失败
- timezone / proxyRuntime / runtimeProtection 验证失败

## 日志字段

- `action=patch-browser-env-proxy-mode`
- `taskId`
- `envId`
- `mode`
- `restartQueued`
- `stage`
- `error`

## 联调验收标准

- 只改 `mode` 时不要求上传整份代理配置
- 非运行态优先同步返回
- 运行中且触发重建时，必须返回 `taskId/eventsUrl`
- `identityHash` 不因 `proxy-mode` 变化而改变
