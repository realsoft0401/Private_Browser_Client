# API Implementation Status

## 目标

这份文档只回答三件事：

- 哪些接口当前已经真实实现
- 哪些接口已经完成正式协议收口，但代码还没补完
- 哪些接口属于兼容期旧入口

## 已实现的正式接口

- `GET /health`
- `GET /api/v1/edge/device-info`
- `GET /api/v1/edge/node-registration`
- `POST /api/v1/edge/node-registration/assign`
- `GET /api/v1/edge/slots`
- `POST /api/v1/edge/slots`
- `GET /api/v1/edge/slots/{slotId}`
- `GET /api/v1/edge/slots/{slotId}/vnc-info`
- `GET /api/v1/edge/slots/{slotId}/cdp-info`
- `GET /api/v1/edge/slots/{slotId}/vnc/ws`
- `POST /api/v1/edge/slots/{slotId}/reinit`
- `DELETE /api/v1/edge/slots/{slotId}`
- `POST /api/v1/edge/browser-envs`
- `POST /api/v1/edge/browser-envs/{envId}/run`
- `POST /api/v1/edge/browser-envs/{envId}/stop`
- `DELETE /api/v1/edge/browser-envs/{envId}/package`
- `GET /api/v1/edge/tasks/{taskId}/events`

## 已完成正式文档、待代码实现

- `PATCH /api/v1/edge/browser-envs/{envId}/proxy`
- `POST /api/v1/edge/browser-envs/{envId}/backup`
- `POST /api/v1/edge/browser-envs/{envId}/restore`
- `POST /api/v1/edge/browser-envs/{envId}/revalidate`
- `POST /api/v1/edge/browser-envs/import-package`

这些接口保留在文档与 OpenAPI 中，是为了先把正式协议、状态机、SSE 和错误口径固定下来，后续直接按既定协议实现。

## 兼容期旧入口

- `GET /api/v1/edge/packages/{packageId}/runtime-view`
- `POST /api/v1/edge/packages/{packageId}/run`
- `POST /api/v1/edge/packages/{packageId}/stop`

口径：

- 只用于兼容过渡和本机调试
- 不再继续扩展新业务能力
- OpenAPI 应标记 `deprecated`
- 正式新能力统一收口到 `browser-envs/*`

## 工具页与文档入口

- `GET /swagger`
- `GET /openapi.yaml`
- `GET /web-vnc.html`

## 收口原则

- 正式业务优先看 `browser-envs/*`
- 资源位能力优先看 `slots/*`
- 兼容期入口只做过渡，不再作为正式能力继续设计
