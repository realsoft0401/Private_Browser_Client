# GET /api/v1/edge/browser-envs/{envId}/cdp-test

## 当前状态

- 正式协议已收口
- 当前新 Client 代码暂未实现

## 功能目标

对指定环境包做基础 CDP 连通性诊断。

这条接口只回答“CDP 能不能用”，不负责代理出口验证，不负责 timezone 验证，也不负责执行业务 RPA。

## 业务边界

- 负责测试 `/json/version`
- 负责测试目标页创建或读取
- 负责测试 WebSocket 连接
- 负责测试 `Runtime.enable` / `Runtime.evaluate`
- 不判断 timezone
- 不判断代理出口
- 不作为生命周期接口
- 不替代未来的受控 `cdp/command`

## 前置校验

- `envId` 必须存在
- 环境包应处于 `running`
- 当前必须能拿到可访问的 CDP 地址

## 请求与响应

### 请求

```http
GET /api/v1/edge/browser-envs/906090001_tk_324867594169356288/cdp-test
Accept: application/json
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "ok": true,
    "envId": "906090001_tk_324867594169356288",
    "stage": "runtime_evaluate",
    "httpVersionUrl": "http://127.0.0.1:9201/json/version",
    "webSocketDebuggerUrl": "ws://127.0.0.1:9201/devtools/browser/abc123",
    "browserVersion": "Chrome/137.0.0.0",
    "targetId": "target-001",
    "evaluateResult": {
      "type": "string",
      "value": "ok"
    },
    "checkedAt": "2026-06-17T15:20:40+08:00"
  }
}
```

### 失败响应语义

失败时仍建议统一 `code != 1000`，并在 `data` 或错误信息中明确指出卡住阶段，例如：

- `http_version`
- `create_target`
- `websocket`
- `runtime_enable`
- `runtime_evaluate`

## 状态流转

- 本接口只读诊断，不改变环境包主状态
- 但可以回写最近一次诊断摘要到日志或运行摘要字段

## SSE 说明

- 本接口不用 SSE
- 原因：诊断链路虽多步，但整体应是单次快速检测

## 任务编排

- 本接口不创建 task

## 成功判定

- `/json/version` 可访问
- WebSocket 可连接
- `Runtime.evaluate` 返回成功

## 失败判定

- `envId` 不存在
- 环境未运行
- CDP 地址不可达
- WebSocket 建连失败
- `Runtime.evaluate` 失败

## 日志字段

- `action=browser-env-cdp-test`
- `envId`
- `stage`
- `httpVersionUrl`
- `webSocketDebuggerUrl`
- `error`

## 联调验收标准

- 成功时能明确看到 `ok=true`
- 失败时能明确知道卡在哪个 CDP 阶段
- 不能把 `cdp-test` 结果冒充成 timezone/代理出口成功
