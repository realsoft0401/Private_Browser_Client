# Private_Browser_Client API Docs

## 说明

这个目录记录当前 `Private_Browser_Client` 的接口文档全量清单。

文档收口原则：

- 正式接口可以先完成协议设计，再进入实现
- 但必须明确标注当前是“已实现”还是“待实现”
- 平台端、Node Server、完整浏览器运行链路等“后续接入点”会单独标明，但不伪装成当前已完成能力
- 正式 REST 接口按“一接口一文件”维护
- WebSocket / 工具页这类非标准 REST 入口，也单独留文档，避免前后端和联调口径分散

## 实现状态总表

- [implementation-status.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/implementation-status.md)

## 当前接口清单

基础事实：

- [health.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/health.md)
- [device-info.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/device-info.md)

Docker 与运维诊断：

- [docker-status.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/docker-status.md)
- [docker-images.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/docker-images.md)
- [docker-containers.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/docker-containers.md)
- [pull-image.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/pull-image.md)
- [remove-image.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/remove-image.md)
- [container-start.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-start.md)
- [container-stop.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-stop.md)
- [container-restart.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-restart.md)

Node 登记协同：

- [node-registration-status.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/node-registration-status.md)
- [node-registration-assign.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/node-registration-assign.md)

Slot 资源位：

- [list-slots.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/list-slots.md)
- [create-slot.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/create-slot.md)
- [get-slot.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/get-slot.md)
- [get-slot-vnc-info.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/get-slot-vnc-info.md)
- [get-slot-cdp-info.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/get-slot-cdp-info.md)
- [slot-vnc-ws.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/slot-vnc-ws.md)
- [reinit-slot.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/reinit-slot.md)
- [destroy-slot.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/destroy-slot.md)

正式 browser-env 生命周期：

- [browser-env-contract.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/browser-env-contract.md)
- [interface-layer-boundary.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/interface-layer-boundary.md)
- [integration-guide.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/integration-guide.md)
- [development-priority.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/development-priority.md)
- [container-json-contract.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-json-contract.md)
- [list-browser-envs.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/list-browser-envs.md)
- [get-browser-env-detail.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/get-browser-env-detail.md)
- [get-task-detail.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/get-task-detail.md)
- [task-events.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/task-events.md)
- [webvnc-real-runtime-implementation.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/webvnc-real-runtime-implementation.md)
- [full-browser-env-e2e.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/full-browser-env-e2e.md)
- [browser-env-e2e-create-flow.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/browser-env-e2e-create-flow.md)
- [browser-env-e2e-import-flow.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/browser-env-e2e-import-flow.md)
- [browser-env-query-e2e.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/browser-env-query-e2e.md)
- [create-browser-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/create-browser-env.md)
- [run-browser-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/run-browser-env.md)
- [stop-browser-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/stop-browser-env.md)
- [delete-browser-env-image.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/delete-browser-env-image.md)
- [get-browser-env-cdp-test.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/get-browser-env-cdp-test.md)
- [delete-browser-env-package.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/delete-browser-env-package.md)

已完成正式文档、待代码实现：

- [patch-browser-env-proxy.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/patch-browser-env-proxy.md)
- [patch-browser-env-proxy-mode.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/patch-browser-env-proxy-mode.md)
- [backup-browser-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/backup-browser-env.md)
- [restore-browser-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/restore-browser-env.md)
- [revalidate-browser-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/revalidate-browser-env.md)
- [import-browser-env-package.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/import-browser-env-package.md)

## 当前统一响应口径

除特别说明外，当前接口统一返回：

```json
{
  "code": 1000,
  "message": "success",
  "data": {}
}
```

当前已定义业务码：

- `1000` 成功
- `1001` 请求参数错误
- `1002` 数据不存在
- `1003` 数据状态冲突
- `1004` 本机依赖调用失败
- `1005` 服务繁忙
- `1006` 未授权

补充约定：

- `1003` 在生命周期接口里优先表示 `browser env lifecycle conflict`
- 典型场景包括：并发 run/stop、running 状态下直接 delete、slot 正在被占用但又触发不允许的动作

## 当前状态机总口径

slot 侧当前主状态：

- `waiting`
- `loading`
- `occupied`
- `releasing`

browser-env 侧当前主状态：

- `created`
- `running`
- `stopped`
- `backed_up`
- `deleted`
- `error`

当前正式文档只保留两层可见口径：

- `browser-env` 资产生命周期状态
- `slot` 资源位状态

## 当前分层口径

- `slots/*`：本机资源位接口，围绕 slot 常驻容器与分配状态。
- `browser-envs/*`：正式业务生命周期接口，围绕完整环境包资产、原子材料、SSE 任务链路和管理员排障口径。
补充约定：

- `container.json` 的最终职责边界见 [container-json-contract.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-json-contract.md)
- SSE 统一字段和订阅规则见 [task-events.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/task-events.md)
