# GET /api/v1/edge/slots/{slotId}/vnc/ws

## 功能目标

把 noVNC WebSocket 代理到指定 slot 对应的 VNC TCP 端口。

这条接口只负责转发 WebSocket 流量，不负责证明后端一定是“真实浏览器桌面”。

也就是说：

- `ws` 代理存在
- 不等于 VNC 后端一定有真实桌面可看

## 业务边界

- 这是运行态传输入口，不是普通 JSON REST 接口
- 只负责连接转发
- 不负责创建 slot
- 不负责判断 package 是否成功登录

## 前置校验

- `slotId` 必须存在
- slot 必须存在有效 `vncPort`
- 目标 VNC TCP 端口必须可连接

## 状态流转

- 不修改 slot/package 主状态

## 请求与响应

### 请求

```http
GET /api/v1/edge/slots/slot001/vnc/ws
Upgrade: websocket
```

### 响应类型

```http
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
```

## SSE 说明

- 本入口不使用 SSE
- 原因：这是 WebSocket 代理入口，不是 SSE 事件流接口

### 成功判定

- WebSocket 握手成功
- 能持续转发到目标 VNC 端口
- 如果后端只是占位容器但端口被映射，可能出现“代理链路存在但真实画面不可用”的情况；这应归类为运行镜像能力问题，而不是本接口路由缺失

### 失败判定

- slot 不存在
- VNC 端口不存在
- VNC 后端不可连接

## 任务编排

当前入口不创建 task。

## 日志字段

- `action=proxy-slot-vnc`
- `slotId`
- `vncPort`
- `remoteAddr`
- `error`

## 联调验收标准

- `web-vnc.html?slot=...` 能通过这个入口建立连接
- slot 无 VNC 端口时必须明确失败，不能挂空连接
- 页面可访问、`ws` 可达，与真实桌面可见性是两个层面的结论，测试记录里必须分开写
