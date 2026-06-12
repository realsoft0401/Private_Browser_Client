# Edge API 设计：`PATCH /api/v1/edge/browser-envs/{envId}/proxy-mode`

## 1. 功能目标

`PATCH /api/v1/edge/browser-envs/{envId}/proxy-mode` 用于切换本机环境包 `proxy/clash.yaml` 的顶层代理模式。

## 2. 业务边界

- 只改 `mode`
- 属于代理配置修改能力
- running 环境需要受控重建容器
- 不改变 `identityHash`

## 3. 请求与响应

```http
PATCH /api/v1/edge/browser-envs/{envId}/proxy-mode
```

请求重点：

- `mode`

返回重点：

- `envId`
- `proxy.mode`
- `bindingVersion`
- `restartQueued`
- `taskId`
- `eventsUrl`

## 4. 前置校验

- 环境包必须存在
- `mode` 只能是支持的 Clash 模式

## 5. 状态流转

- 非 running：同步写入
- running：快速返回并后台 forceRecreate

## 6. 成功判定

- mode 修改成功
- 运行中环境能正确进入后台重建链路

## 7. 失败判定

- 环境包不存在
- `mode` 非法
- 配置写盘失败

## 8. 日志字段

- `envId`
- `mode`
- `bindingVersion`
- `taskId`
- `error`

## 9. 联调验收标准

- 只改顶层 mode，不误改其他代理配置
- running 环境能通过 SSE 观察重建
