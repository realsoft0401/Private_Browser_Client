# Private_Browser_Client API 文档目录

本文档目录用于给 `Private_Browser_Client` 提供逐接口 Markdown 说明。

`docs/openapi.yaml` 继续承担协议事实源；这里重点补足：

- 这个接口解决什么问题
- 它负责什么、不负责什么
- 哪些状态允许调用，哪些状态必须拒绝
- 是否创建 Edge task、如何看 SSE
- 失败后管理员该看什么、怎么排查

## System

- [health.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/health.md)

## Device And Docker

- [device-info.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/device-info.md)
- [docker-status.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/docker-status.md)
- [docker-images.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/docker-images.md)
- [docker-containers.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/docker-containers.md)
- [pull-image.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/pull-image.md)
- [remove-image.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/remove-image.md)
- [container-start.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-start.md)
- [container-stop.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-stop.md)
- [container-restart.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-restart.md)

## Edge Tasks

- [task-detail.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/task-detail.md)
- [task-events.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/task-events.md)

## Browser Envs

- [create-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/create-env.md)
- [list-envs.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/list-envs.md)
- [env-detail.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/env-detail.md)
- [import-package.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/import-package.md)
- [rebuild-candidates.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/rebuild-candidates.md)
- [rebuild-index.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/rebuild-index.md)
- [run-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/run-env.md)
- [stop-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/stop-env.md)
- [revalidate-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/revalidate-env.md)
- [backup-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/backup-env.md)
- [restore-env.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/restore-env.md)
- [delete-env-package.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/delete-env-package.md)
- [delete-env-image.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/delete-env-image.md)
- [update-proxy.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/update-proxy.md)
- [update-proxy-mode.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/update-proxy-mode.md)
- [cdp-test.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/cdp-test.md)
- [vnc-info.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/vnc-info.md)
- [vnc-ws.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/vnc-ws.md)

## 当前约束

- 这些文档只描述单个 Edge 节点的本机能力，不描述中心 Server 的聚合、审计、任务持久化和节点验证。
- `clientId`、节点归属、UDP 发现后的中心绑定、`verified`、`healthy + verified + online` 这些中心概念不应加回 Client 文档。
- 本机 Edge task 是短期内存事实；平台级最终任务事实仍由 `Private_Browser_Server` 保存。
