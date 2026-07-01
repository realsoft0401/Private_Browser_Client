# Requirements Boundary

## 1. 职责边界

`Private_Browser_Client` 只负责本机边缘能力：

- 设备信息
- Docker 状态
- 本机运行环境
- HTTP API
- UDP beacon

不负责：

- 用户体系
- JWT / API Key / mTLS
- 多节点调度
- 设备归属
- 中心权限判断
- 中心 `clientId` 身份维护

补充收口：

- `clientId` 是 Server 中心节点身份，不是 Client 本机资源 ID
- Client 不应要求正式 Edge API 请求携带 `clientId`
- `node-registration` 相关接口负责 bind 成功后的中心身份写回与本地留痕，但不参与 discovery，不替代节点健康与业务放行判断

## 2. UDP 自动发现边界

- 只广播服务入口和非敏感摘要
- 不承载业务动作
- 不传环境包状态
- 不传敏感内容
- `discoveryMagic/service/discoveryGroup` 用于识别当前平台和发现域

## 3. 安全边界

- 默认内网受信模式
- 不直接暴露公网
- 安全和权限控制由 Server 或网络边界承担
- 未来进入公网或跨客户网络时再单独设计节点鉴权

## 4. WebVNC 边界

- `WebVNC` 围绕 `slot`，不再围绕 `envId`
- 入口形态应是 `/web-vnc.html?slot=...`
- 展示的是 slot 当前承载的浏览器，不是固定包实例

补充说明：

- `slot` 是运行承载位，不是正式业务资产
- 正式业务生命周期主入口仍然是 `browser-envs/*`

## 5. 镜像版本边界

- `slot_runtime.image` 是 Client 当前“新建 slot / 显式 reinit slot”使用的默认基础镜像。
- `browser-env runtime.image` 是某个环境包正式运行时使用的业务运行镜像。
- 修改默认 `slot_runtime.image` 只影响后续新建 slot 或显式 reinit 后的 slot。
- 已存在 slot 必须保留自己当前实际 `runtimeImage`，不能因为默认值更新自动漂移。
- 老 slot 升级基础镜像必须走显式重初始化链路，成功后仍视为原 slot 恢复 `waiting`。
- browser-env 的 `runtime.image` 不能因为 slot 默认镜像变化被自动改写。
- browser-env 正式运行镜像只能通过受控 runtime-image 修改接口显式变更。
- 修改 browser-env `runtime.image` 时，目标环境包必须处于 `created` 或 `stopped` 状态，不能在 `loading/running/ending/backed_up/deleted/error` 状态下修改。
- `created` 表示首次运行前配置态；`stopped` 表示运行后已与 slot/container 彻底隔离的干净态；二者都允许修改 runtime.image。
- 修改 browser-env `runtime.image` 后不自动 run、不自动 pull image、不自动 reinit slot，下一次 run 才使用新镜像。
