# GET /api/v1/edge/slots/{slotId}/vnc-info

## 功能目标

返回指定 slot 的 VNC / noVNC 连接信息。

这里要特别区分两层含义：

- 返回 `vncUrl/wsUrl/webVncUrl`，只表示连接入口信息已经生成
- 不自动等价于“真实桌面画面一定可见”

真实画面是否可见，还取决于当前 `slot runtime` 镜像内部是否真的提供：

- VNC Server
- 浏览器桌面
- 对应的 VNC 协议服务

## 业务边界

- 只返回连接事实
- 以 slot 为视角，不再以 package/env 为视角
- 不负责判断当前 package 业务是否成功
- 不负责代替 WebSocket 代理本身

## 前置校验

- `slotId` 必须存在
- slot 必须已经初始化出 `vncPort`

## 状态流转

- 只读接口，不修改状态

## 请求与响应

### 请求

```http
GET /api/v1/edge/slots/slot001/vnc-info
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "slotId": "slot001",
    "vncPort": 5901,
    "vncUrl": "127.0.0.1:5901",
    "wsUrl": "ws://127.0.0.1:3300/api/v1/edge/slots/slot001/vnc/ws",
    "webVncUrl": "http://127.0.0.1:3300/web-vnc.html?slot=slot001",
    "cdpPort": 9222
  }
}
```

## SSE 说明

- 本接口不使用 SSE
- 原因：连接信息查询是短链路只读接口，同步 HTTP 已足够表达结果

## 任务编排

当前接口不创建 task。

## 成功判定

- 能返回 slot 对应的 VNC/noVNC 连接地址
- `webVncUrl` 可访问时，说明 WebVNC 页面入口已挂载
- `wsUrl` 可连接时，说明 WebSocket 代理链路存在
- 但“是否能看到真实桌面画面”必须额外结合 `slot runtime` 镜像能力判断，不能只凭本接口成功就下结论

## 失败判定

- slot 不存在
- slot 未初始化 VNC 端口

## 日志字段

- `action=get-slot-vnc-info`
- `slotId`
- `vncPort`

## 联调验收标准

- 返回的 `webVncUrl` 必须是 `?slot=...` 口径
- 返回的 `wsUrl` 必须能对应到同 slot 的代理入口
- 如果当前 `slot runtime` 是占位容器，本接口仍可成功返回连接信息；此时测试结论应写成“WebVNC 页面与连接信息可访问”，而不是“真实桌面可见”
