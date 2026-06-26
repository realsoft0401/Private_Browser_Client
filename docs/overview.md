# Overview

新的 `Private_Browser_Client` 采用当前项目架构重建，聚焦单机边缘执行和 `browser-env` 正式生命周期；`slot` 只是本机资源位，不是正式业务主线。

当前骨架同时保留：

- `browser-env / slot / runtime relation` 新模型方向
- `browser-envs` 正式资产层方向
- `Swagger / OpenAPI` 正式文档能力
- `WebVNC` 的 `slot` 视角新口径
- 关键动作代码中用 `********` 标出的平台端接口预留接入点，后续平台接口完成后直接在对应 `Service` 层接入
- `Service/Platform` 平台同步骨架，当前默认 noop，实现“暂不限制、create-slot 可无限创建”

当前统一口径：

- `browser-envs/*` 是正式业务生命周期入口
- `slots/*` 是 Client 本机资源层与运维层
- `node-registration/*` 是当前 bind 成功后的正式写回与本地留痕接口，但它不参与 discovery，也不直接决定业务放行
- 中心 `clientId` 身份、节点归属和 run 准入都由 `Private_Browser_Server` 负责

## 当前文档口径

- `docs/api/*.md` 按一接口一文件维护
- `docs/api/implementation-status.md` 标注“已实现 / 待实现”
- `docs/api/interface-layer-boundary.md` 统一说明 `docker/*`、`containers/*`、`slots/*`、`browser-envs/*` 四层边界
- `docs/openapi.yaml` 统一维护当前正式接口与已收口待实现接口
