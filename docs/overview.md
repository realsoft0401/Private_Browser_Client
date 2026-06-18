# Overview

新的 `Private_Browser_Client` 采用当前项目架构重建，聚焦单机边缘执行、slot 资源位和 browser-env 正式生命周期。

当前骨架同时保留：

- `slot / package / runtime relation` 新模型方向
- `browser-envs` 正式资产层方向
- `Swagger / OpenAPI` 正式文档能力
- `WebVNC` 的 `slot` 视角新口径
- 关键动作代码中用 `********` 标出的平台端接口预留接入点，后续平台接口完成后直接在对应 `Service` 层接入
- `Service/Platform` 平台同步骨架，当前默认 noop，实现“暂不限制、create-slot 可无限创建”

## 当前文档口径

- `docs/api/*.md` 按一接口一文件维护
- `docs/api/implementation-status.md` 标注“已实现 / 待实现”
- `docs/api/interface-layer-boundary.md` 统一说明 `docker/*`、`containers/*`、`slots/*`、`browser-envs/*` 四层边界
- `docs/openapi.yaml` 统一维护当前正式接口与已收口待实现接口
