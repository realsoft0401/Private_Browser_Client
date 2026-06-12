# Edge API 设计：`GET /api/v1/edge/browser-envs/{envId}/cdp-test`

## 1. 功能目标

`GET /api/v1/edge/browser-envs/{envId}/cdp-test` 用于对当前 running 环境执行基础 CDP 连通性诊断。

## 2. 业务边界

- 只测 CDP 连通性
- 不判断 timezone provider
- 不判断代理出口质量
- 不是 run 成功的替代判据

## 3. 请求与响应

```http
GET /api/v1/edge/browser-envs/{envId}/cdp-test
```

返回重点：

- `envId`
- `cdpPort`
- `endpoint`
- `ok`
- `stage`
- `browser`
- `error`

## 4. 前置校验

- 环境包必须存在
- 环境包应处于 running

## 5. 状态流转

- 只读诊断，不改环境包主状态

## 6. 成功判定

- `/json/version` 可达
- DevTools WebSocket 可连接
- `Runtime.evaluate` 成功

## 7. 失败判定

- 环境包未运行
- CDP 端口不可达
- WebSocket 连接失败
- 诊断执行失败

## 8. 日志字段

- `envId`
- `cdpPort`
- `stage`
- `error`

## 9. 联调验收标准

- running 环境能完成基础 CDP 测试
- 失败时能明确指出阶段，不只返回失败
