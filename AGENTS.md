# Private_Browser_Client

## 当前约束

- 本项目是新的 `Private_Browser_Client` 起点。
- 项目目录层次完全沿用 old 项目，不重新设计第一层目录。
- old 业务实现不直接复制进入新项目。
- 后续开发以 `package / slot / runtime relation` 新模型为准。
- `docs/openapi.yaml` 和 `public/swagger.html` 作为正式能力保留，不要等接口写完再临时补。
- `README.md` 里的职责边界、UDP 自动发现边界和安全边界属于正式需求，后续实现不能退回 old 的强绑定模型。
- `WebVNC` 后续必须按 `slot` 视角实现，不再按 `envId/package` 视角实现。
