# WebVNC 新方案设计图

## 1. 目标

这份文档用于回答一个具体问题：

- `Private_Browser_Client_Old` 已经有真实可用的 WebVNC
- 新 Client 不想退回 old 的 `envId` 强绑定视角
- 那么新 Client 应该如何“参考 old 的能力”，同时保留现在的 `slot` 思想

最终结论先写在前面：

- 要参考 old 的“真实 VNC 来源”
- 不要退回 old 的“入口直接绑定 envId”
- 新方案应保持：
  - 页面入口还是 `slot`
  - 但 `slot` 背后代理的是“当前真实浏览器运行容器”

## 2. old 里真正有价值的能力

old 里真正让 WebVNC 有画面的关键，不是页面本身，而是这一条链：

```text
web-vnc.html
-> noVNC(RFB)
-> /api/v1/edge/browser-envs/{envId}/vnc/ws
-> Go WebSocket 代理
-> 浏览器容器暴露的 VNC TCP 端口
-> 容器内 x11vnc
```

对应关键实现：

- 路由：
  - [Routes.go](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client_Old/Routes/Routes.go)
- WebSocket 代理：
  - [vnc.go](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client_Old/Service/BrowserEnv/vnc.go)
- noVNC 页面：
  - [web-vnc.html](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client_Old/public/web-vnc.html)

## 3. old 为什么能出画面

因为 old 的 `vnc/ws` 不是代理到一个“占位容器”，而是代理到：

- 当前 running 环境包对应的真实浏览器容器 VNC 端口

old 的成功关键点有 3 个：

### 3.1 VNC 来源是真实浏览器容器

不是抽象的 slot，不是静态壳容器，而是：

- 当前这个环境包运行起来后
- 真正那一个浏览器 Docker 容器
- 它暴露出来的 `5900`

### 3.2 WebSocket 代理只做字节转发

old 的 Go 服务没有自己理解 VNC 协议，而只是做：

- noVNC WebSocket 二进制帧
- 转发到 VNC TCP

这点很好，后续新方案应该保留。

### 3.3 连接目标端口不让前端传

old 的设计是：

- 目标 VNC 端口只能从本机索引读取
- 前端不能手动传目标端口

这个边界也应该保留。

## 4. old 不应该直接继承的地方

虽然 old 能出画面，但它的入口口径不适合直接搬到新方案。

old 的入口是：

- `web-vnc.html?envId=...`
- `/api/v1/edge/browser-envs/{envId}/vnc-info`
- `/api/v1/edge/browser-envs/{envId}/vnc/ws`

这隐含的是：

- 画面天然跟某个 env 绑定
- 查看画面时是“我打开某个环境”

而新方案已经明确：

- slot 是资源位
- 包运行到哪个 slot，就通过哪个 slot 查看
- 不希望再退回 old 的“一包一固定浏览器入口”思路

所以 old 不能照搬的不是技术实现，而是入口语义。

## 5. 新方案应该怎么借 old

新方案应该这样拆：

### 5.1 入口语义保留 slot

继续保留：

- `web-vnc.html?slot=slot001`
- `GET /api/v1/edge/slots/{slotId}/vnc-info`
- `GET /api/v1/edge/slots/{slotId}/vnc/ws`

因为这符合你已经收口的思想：

- 用户看的不是“这个 env 的固定桌面”
- 而是“这个 slot 当前承载的桌面”

### 5.2 VNC 来源借 old，改成真实运行容器

也就是说：

- 页面入口还是 slot
- 但 `vnc/ws` 不再接 slot 占位容器
- 而是查“这个 slot 当前绑定的真实运行容器”
- 再代理到这个真实运行容器的 VNC TCP 端口

这就是“入口保留新思想，底层运行现场参考 old”。

## 6. 新方案最终链路

建议最终跑成下面这条链：

```text
web-vnc.html?slot=slot001
-> noVNC
-> /api/v1/edge/slots/slot001/vnc/ws
-> Go WebSocket 代理
-> 查询 slot 当前绑定的运行容器
-> 转发到真实浏览器容器 VNC TCP 端口
-> 容器内 x11vnc / 桌面服务
```

对应地，`vnc-info` 也不应再返回占位容器信息，而应返回：

- 当前 slot 绑定的真实运行容器 VNC 地址

## 7. 新旧方案对照表

### old

- 入口
  - `envId`
- 运行对象
  - 真实浏览器容器
- 画面来源
  - 真实浏览器容器 VNC
- 优点
  - 能真出画面
- 缺点
  - 入口和 env 强绑定

### new 当前状态

- 入口
  - `slot`
- 运行对象
  - slot 占位容器
- 画面来源
  - 占位容器映射端口
- 优点
  - 资源位思想已经立住
- 缺点
  - 没真实画面

### new 目标状态

- 入口
  - `slot`
- 运行对象
  - 真实浏览器运行容器
- 画面来源
  - slot 当前绑定的真实运行容器 VNC
- 优点
  - 思想和能力都兼顾

## 8. 数据模型建议

为了让 `slot -> 真实运行容器` 关系稳定，建议新增一层显式运行现场模型。

至少应有：

- `runId`
- `envId`
- `slotId`
- `containerId`
- `containerName`
- `runtimeImage`
- `vncPort`
- `cdpPort`
- `status`
- `startedAt`
- `updatedAt`

这样：

- slot 仍然只表达资源位
- 运行容器表达真实现场
- WebVNC / CDP 接口都去查真实现场

## 9. 接口改造建议

### 9.1 `run`

当前：

- 只改状态

后续：

- 真正创建浏览器运行容器
- 建立 `slot -> run container` 绑定
- 回写运行现场信息

### 9.2 `vnc-info`

当前：

- 返回 slot 固定 `vncPort`

后续：

- 返回当前 slot 活动运行容器的 `vncPort/wsUrl/webVncUrl`

### 9.3 `vnc/ws`

当前：

- 代理 slot 占位容器的端口

后续：

- 代理当前活动运行容器的 VNC TCP 端口

### 9.4 `stop`

当前：

- 回收状态

后续：

- 停止真实浏览器容器
- 清理 slot 绑定
- slot 回到 `waiting`

## 10. 真实成功标准

参考 old 之后，新方案里真正的 WebVNC 成功，不应只看：

- 页面是否能打开
- `vnc-info` 是否有返回
- `ws` 是否存在

而应该看：

- 当前 slot 是否绑定了真实运行容器
- 真实运行容器里是否有桌面/VNC 服务
- `vnc/ws` 是否成功代理到真实容器端口
- noVNC 是否能真正接收到画面

## 11. 一句话收口

新方案最正确的方向不是：

- 把 old 的 `envId` WebVNC 原样搬回来

也不是：

- 继续把 slot 占位容器当成真实画面来源

而是：

- 保留 `slot` 作为入口
- 参考 old，把 WebVNC 最终连接到真实浏览器运行容器
