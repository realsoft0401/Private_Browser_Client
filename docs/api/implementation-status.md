# API Implementation Status

## 目标

这份文档只回答两件事：

- 哪些接口当前已经真实实现
- 哪些接口已经完成正式协议收口，但代码还没补完

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
- `GET /api/v1/edge/browser-envs`
- `GET /api/v1/edge/browser-envs/{envId}`
- `POST /api/v1/edge/browser-envs/{envId}/run`
- `POST /api/v1/edge/browser-envs/{envId}/stop`
- `DELETE /api/v1/edge/browser-envs/{envId}/package`
- `GET /api/v1/edge/tasks/{taskId}`
- `GET /api/v1/edge/tasks/{taskId}/events`

## 已完成正式文档、待代码实现

- `GET /api/v1/edge/docker/status`
- `GET /api/v1/edge/docker/images`
- `GET /api/v1/edge/docker/containers`
- `POST /api/v1/edge/docker/pull-image`
- `POST /api/v1/edge/docker/remove-image`
- `POST /api/v1/edge/containers/{slotId}/start`
- `POST /api/v1/edge/containers/{slotId}/stop`
- `POST /api/v1/edge/containers/{slotId}/restart`
- `DELETE /api/v1/edge/browser-envs/{envId}/del`
- `GET /api/v1/edge/browser-envs/{envId}/cdp-test`
- `PATCH /api/v1/edge/browser-envs/{envId}/proxy`
- `PATCH /api/v1/edge/browser-envs/{envId}/proxy-mode`
- `POST /api/v1/edge/browser-envs/{envId}/backup`
- `POST /api/v1/edge/browser-envs/{envId}/restore`
- `POST /api/v1/edge/browser-envs/{envId}/revalidate`
- `POST /api/v1/edge/browser-envs/import-package`

这些接口保留在文档与 OpenAPI 中，是为了先把正式协议、状态机、SSE 和错误口径固定下来，后续直接按既定协议实现。

## 工具页与文档入口

- `GET /swagger`
- `GET /openapi.yaml`
- `GET /web-vnc.html`

## 收口原则

- 正式业务优先看 `browser-envs/*`
- 资源位能力优先看 `slots/*`
- 不再对外暴露 `packages/*` 旧入口，正式新能力统一收口到 `browser-envs/*`
