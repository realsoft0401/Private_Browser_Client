# PATCH /api/v1/edge/browser-envs/{envId}/proxy

## 功能目标

修改指定环境包的代理配置，并在需要时触发受控运行验证，最终重新确认网络运行保护状态。

## 业务边界

- 负责写入 `proxy/clash.yaml`
- 负责更新 `profile.proxy`、`binding.version`、`profile.updatedAt`
- 负责把 `runtimeProtection/proxyRuntime` 标记为 `pending`
- 环境运行中时，可触发后续运行验证链路
- 不改变 `envId/userId/rpaType`
- 不改变 `identityHash`
- 不接受任意容器参数透传

## 前置校验

- `envId` 存在
- `profile.json`、`binding.json` 可读
- proxy 路径必须在环境包受控目录内
- `configBase64` 合法
- 代理 YAML 合法
- 环境包不处于已删除或已彻底销毁态

## 请求与响应

### 请求

```http
PATCH /api/v1/edge/browser-envs/906090001_tk_324867594169356288/proxy
Content-Type: application/json
```

```json
{
  "enabled": true,
  "type": "clash",
  "mode": "rule",
  "configBase64": "cG9ydDogNzg5MAptb2RlOiBydWxlCg=="
}
```

### 同步成功响应

环境未运行，或当前修改不需要长链路验证时：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "envId": "906090001_tk_324867594169356288",
    "restartQueued": false,
    "runtimeProtectionStatus": "pending",
    "proxyRuntimeStatus": "pending"
  }
}
```

### 任务化成功响应

环境正在运行且需要后续重启/重建和运行验证时：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-proxy-001",
    "taskType": "browser_env_proxy_update",
    "resourceType": "browser_env",
    "resourceId": "906090001_tk_324867594169356288",
    "restartQueued": true,
    "eventsUrl": "/api/v1/edge/tasks/edge-task-proxy-001/events"
  }
}
```

## 状态流转

- `binding.version` 递增
- `profile.updatedAt` 更新
- `runtimeProtection/proxyRuntime` 改为 `pending`
- 环境运行中且触发验证时，再进入后续容器和网络探测链路

## SSE 说明

- 本接口不能默认滥用 SSE
- 只有当这次代理修改触发明显多阶段、耗时较长的容器侧重新验证时，才使用 SSE
- 如果只是快速完成文件落盘与摘要更新，同步 HTTP 已足够表达结果，则不用 SSE

需要 SSE 时，阶段建议：

- `queued`
- `load_env`
- `decode_proxy_config`
- `write_proxy_config`
- `update_profile_binding`
- `mark_runtime_pending`
- `prepare_restart`
- `restart_container`
- `wait_browser`
- `timezone_probe`
- `proxy_runtime_probe`
- `runtime_protection_update`
- `finalize_success`
- `finalize_failed`

## 任务编排

- 当前接口存在两种执行方式：
- 同步模式：只做配置落盘和摘要更新，不创建 task
- 任务模式：运行中环境触发重启和运行验证时，返回 `taskId/eventsUrl`

## 成功判定

- 新代理配置落盘成功
- `binding.version` 递增成功
- `profile.updatedAt` 更新成功
- `runtimeProtection/proxyRuntime` 已改为 `pending`
- 若触发长链路验证，则后续验证通过

## 失败判定

- `envId` 不存在
- `configBase64` 非法
- 代理 YAML 非法
- 路径越界
- 文件写入失败
- 容器重启/重建失败
- timezone / proxy 出口 / runtime protection 验证失败

## 日志字段

- `action=patch-proxy`
- `taskId`
- `envId`
- `proxyType`
- `proxyMode`
- `bindingVersionBefore`
- `bindingVersionAfter`
- `restartQueued`
- `stage`
- `error`

## 联调验收标准

- 未运行环境修改代理时，同步 HTTP 能直接返回明确结果
- 运行中环境触发重建验证时，返回 `taskId/eventsUrl`
- 不能因为改完文件就继续沿用旧可用结论
