# Swagger 能力说明

## 1. 当前结论

新的 `Private_Browser_Client` 从第一版开始就保留 `Swagger / OpenAPI` 能力。

这不是临时调试页，而是正式文档能力的一部分。

## 2. 当前骨架

- `docs/openapi.yaml`
- `public/swagger.html`
- `GET /swagger`
- `GET /openapi.yaml`

## 3. 后续要求

- 新增正式接口时，同步更新 `docs/openapi.yaml`
- 不要等接口实现很久以后再补 Swagger
- `Routes` 层后续要显式挂出 `/openapi.yaml` 和 `/swagger`

## 4. 展示口径

- 已实现的正式接口：正常展示
- 已完成协议收口、待实现的正式接口：继续展示，但描述里要说清当前阶段
- 兼容期旧入口：继续展示，但应标记 `deprecated`
