# Edge API 设计：`GET /api/v1/edge/browser-envs/{envId}/vnc/ws`

## 1. 功能目标

`GET /api/v1/edge/browser-envs/{envId}/vnc/ws` 用于代理当前本机环境包的 VNC WebSocket 流量，给 `web-vnc.html` 或 noVNC 使用。

## 2. 业务边界

- 只代理本机环境包的 VNC WebSocket
- 不做中心鉴权
- 不做公网网关包装
- 主要服务内网诊断和运维工具

## 3. 请求与响应

```http
GET /api/v1/edge/browser-envs/{envId}/vnc/ws
Upgrade: websocket
```

返回是 WebSocket 代理流，不是 JSON。

## 4. 前置校验

- 环境包必须存在
- VNC 端口必须可用
- 后端 VNC 连接必须可建立

## 5. 状态流转

- 只做流量代理，不改环境包状态

## 6. 成功判定

- WebSocket 升级成功
- 能把流量稳定代理到本机 VNC 端口

## 7. 失败判定

- 环境包不存在
- VNC 端口不可达
- WebSocket 代理建立失败

## 8. 日志字段

- `envId`
- `vncPort`
- `remoteAddr`
- `error`

## 9. 联调验收标准

- `web-vnc.html` 能通过该地址打开会话
- 失败时能明确区分“升级失败”和“后端 VNC 不可达”
