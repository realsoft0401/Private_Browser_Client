# WebVNC 真实画面接通实现清单

## 1. 当前现状

当前 `Private_Browser_Client` 已经具备下面这些能力：

- `web-vnc.html?slot=...` 页面可访问
- `GET /api/v1/edge/slots/{slotId}/vnc-info` 可返回连接信息
- `GET /api/v1/edge/slots/{slotId}/vnc/ws` WebSocket 代理入口已存在
- `POST /api/v1/edge/browser-envs/{envId}/run` 可以把状态机推进到 `task.completed`
- `slot.status`、`runtime_relations`、`package_runtime_views` 可以完成本机状态收口

但当前还不具备下面这条真正业务能力：

- 浏览器真实桌面画面可见

## 2. 为什么现在没画面

根因不是页面路由错误，而是运行链路还停在“占位资源模型”阶段。

### 2.1 slot runtime 还是占位容器

当前配置是：

```yaml
slot_runtime:
  image: alpine:3.20
  command:
    - sleep
    - infinity
```

这说明 slot 当前只是一个常驻占位容器，用来表达：

- 这个 slot 已存在
- 这个 slot 有端口
- 这个 slot 可被占用或释放

但它不具备：

- 浏览器进程
- VNC Server
- CDP 服务
- 浏览器桌面

### 2.2 run 现在只推进“状态机”，没有推进“真实运行现场”

当前 `RunPackage` / `browser_env run` 这条链路，本质上只做了：

- 校验 slot 是否空闲
- 校验 package 是否有冲突
- 创建 runtime relation
- 把 slot 改成 `occupied`
- 把 package runtime view 改成 `running`
- 把 browser_envs 主状态改成 `running`

但还没有做：

- 读取环境包 `profile/binding/proxy/fingerprint/browser-data`
- 启动真实浏览器运行容器
- 把环境包内容挂载进运行容器
- 等待浏览器启动
- 校验 CDP
- 校验 VNC
- 把 slot 的 WebVNC 入口指向真实浏览器现场

## 3. 当前缺的 4 层

要真正看到画面，至少缺下面 4 层：

### 3.1 真实浏览器运行镜像层

必须有一个真正带下面能力的镜像：

- 浏览器
- Xvfb 或等价图形环境
- VNC Server
- CDP 浏览器启动能力
- 必要启动脚本

这层不再能用 `alpine + sleep infinity`。

### 3.2 环境包加载层

run 时必须把环境包真正加载进去，包括：

- `profile.json`
- `binding.json`
- `proxy/`
- `fingerprint/`
- `browser-data/profile`

这一步决定：

- 启动的是哪个账号环境
- 用哪个代理
- 用哪套 fingerprint
- 浏览器是否能恢复登录态

### 3.3 slot 与真实运行容器绑定层

当前 slot 只是“本机资源位记录”，后续必须明确：

- 一个 slot 在某一时刻到底代理哪个真实浏览器容器
- `web-vnc.html?slot=...` 最终应连接到哪个真实 VNC 服务
- `cdp-info` 最终应返回哪个真实浏览器的 CDP 地址

也就是说，slot 后面不能永远挂一个占位容器，而是要挂真实运行现场。

### 3.4 真实成功判定层

当前 `task.completed` 还偏“状态成功”，后续必须升级为“现场成功”：

- 容器创建成功不等于成功
- slot 标成 `occupied` 不等于成功
- 必须至少确认：
  - 浏览器进程存在
  - CDP 可连
  - VNC 可连
  - 必要时 timezone / proxyRuntime / runtimeProtection 可验证

## 4. 推荐改造方向

这里建议不要在现有“占位 slot 容器”上硬补浏览器能力，而是把设计收口成：

### 方案 A：slot 直接代表真实浏览器运行容器

含义：

- slot 创建时不一定立即起浏览器
- run 时由该 slot 启动或接管真实浏览器容器
- `slot.vncPort/cdpPort` 对应的就是当前真实浏览器现场

优点：

- WebVNC 语义最直观
- `slot -> VNC/CDP` 一一对应
- 页面和排障口径简单

代价：

- run/stop/reinit 的容器切换更复杂

### 方案 B：slot 保留资源位，额外挂真实运行容器映射

含义：

- slot 仍是资源位
- 真实浏览器运行容器单独创建
- slot 只保存“当前映射到哪个运行容器”
- WebVNC / CDP 请求时转发到当前运行容器

优点：

- 更贴合“包是包、容器是容器、slot 是资源位”的设计思想
- 后续扩展更灵活

代价：

- 实现比方案 A 多一层映射
- 代理和排障链路更复杂

## 5. 结合你当前思想，建议选 B

你前面已经明确：

- 包是包
- 容器是容器
- slot 是资源位
- 不再强绑 old 那种一包一固定容器视角

所以这里更推荐：

- 保留 slot 作为资源位
- run 时创建或接管真实浏览器运行容器
- slot 记录“当前包 + 当前运行容器 + 当前 VNC/CDP 映射”
- `web-vnc.html?slot=...` 永远只看 slot
- 但 slot 背后真正代理的是当前运行容器

这和你现在的整体思想最一致。

## 6. 推荐实现顺序

建议按下面顺序做，不要一口气全改：

### 第一步：把真实运行容器模型补出来

先增加一层明确的“浏览器运行容器事实”，至少要有：

- runId
- envId
- slotId
- containerId
- containerName
- runtimeImage
- cdpPort
- vncPort
- status

目的：

- 不再让 slot 自己既当资源位，又假装自己就是浏览器现场

### 第二步：run 真的启动浏览器容器

run 时不只写状态，要真的：

- 读取环境包
- 组装挂载
- 起真实浏览器容器
- 回写运行容器事实

### 第三步：让 vnc-info / cdp-info 查真实运行容器

当前这两个接口不要只读 slot 固定端口，而应改成：

- 如果 slot 当前没有运行容器，返回“当前无活动画面”
- 如果 slot 当前有运行容器，返回真实运行容器的 `vncPort/cdpPort`

### 第四步：让 slot-vnc-ws 代理真实运行容器

当前 `ws` 代理要从“代理 slot 自己的占位端口”改成：

- 先查 slot 当前绑定的真实运行容器
- 再代理到真实运行容器的 VNC 端口

### 第五步：升级 run 成功标准

当前 `task.completed` 不能只看状态更新成功，必须至少检查：

- 容器启动成功
- CDP 可连
- VNC 可连

如果以后把 timezone / proxyRuntime / runtimeProtection 接上，则再继续增加：

- 网络指纹验证通过

## 7. 各接口未来应该怎么变

### 7.1 `POST /api/v1/edge/slots`

当前：

- 创建 slot
- 起占位容器

后续建议：

- 创建 slot 资源位记录
- 是否立即起占位容器，可以作为过渡阶段保留
- 但正式运行画面不要再依赖这个占位容器

### 7.2 `POST /api/v1/edge/browser-envs/{envId}/run`

当前：

- 写状态

后续：

- 真正启动浏览器运行容器
- 建立 slot 到运行容器的映射
- 校验 CDP/VNC

### 7.3 `GET /api/v1/edge/slots/{slotId}/vnc-info`

当前：

- 返回 slot 固定 VNC 地址

后续：

- 返回 slot 当前映射的真实运行容器 VNC 地址

### 7.4 `GET /api/v1/edge/slots/{slotId}/vnc/ws`

当前：

- 代理 slot 当前记录的 VNC 端口

后续：

- 代理 slot 当前活动运行容器的 VNC 端口

### 7.5 `POST /api/v1/edge/browser-envs/{envId}/stop`

当前：

- 回收状态

后续：

- 停止真实浏览器运行容器
- 清除 slot 与运行容器映射
- slot 回到 `waiting`

## 8. 当前阶段测试结论怎么写

在真正补完上面能力之前，测试结论必须写准确：

可以写：

- `webVNC 页面可访问`
- `vnc-info 可返回`
- `ws 代理入口存在`
- `run/stop 状态机可跑通`

不能写：

- `真实浏览器桌面已验证通过`
- `VNC 画面已验证成功`
- `CDP 和浏览器现场已完整接通`

## 9. 最该先做的事

如果只选一个最优先动作，我建议先做：

- 让 `run` 真正创建并绑定“真实浏览器运行容器”

原因：

- 只改页面没用
- 只改 `vnc-info` 没用
- 只换 slot 占位镜像也不够

真正的分水岭是：

- `run` 之后有没有一个真实浏览器现场存在

只要这层没立住，WebVNC 永远只是在看一个空壳入口。
