# Edge API 设计：`PATCH /api/v1/edge/browser-envs/{envId}/proxy`

## 1. 功能目标

`PATCH /api/v1/edge/browser-envs/{envId}/proxy` 用于修改本机环境包的代理配置和代理文件。

## 2. 业务边界

- 修改 `profile.proxy` 和 `proxy/clash.yaml`
- 递增 `binding.version`
- 重置 `runtimeProtection` 为待重新验证
- 不改变 `identityHash`
- 不通过本接口修改 `runtime.image`

## 3. 请求与响应

```http
PATCH /api/v1/edge/browser-envs/{envId}/proxy
```

请求重点：

- `enabled`
- `type`
- `mode`
- `configBase64`

返回重点：

- `envId`
- `proxy`
- `bindingVersion`
- `changed`
- `restartQueued`
- `taskId`
- `eventsUrl`

## 4. 前置校验

- 环境包必须存在
- 代理参数必须合法
- `configBase64` 必须可落盘

## 5. 状态流转

- 非 running 环境：同步修改并返回
- running 环境：快速返回 `restartQueued=true`
- 后台通过 SSE 任务 forceRecreate 重建容器

## 6. 成功判定

- 代理配置落盘成功
- 索引版本更新成功
- running 环境能正确创建重建任务

## 7. 失败判定

- 环境包不存在
- 配置非法
- 配置写盘失败
- 重建任务创建失败

## 8. 日志字段

- `envId`
- `bindingVersion`
- `restartQueued`
- `taskId`
- `error`

## 9. 联调验收标准

- 非 running 环境不返回无意义 task
- running 环境能返回 `taskId/eventsUrl`
- 不泄露代理明文
