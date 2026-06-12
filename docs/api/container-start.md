# Edge API 设计：`POST /api/v1/edge/containers/{id}/start`

## 1. 功能目标

`POST /api/v1/edge/containers/{id}/start` 用于启动本机指定 Docker 容器。

## 2. 业务边界

- 这是内网运维诊断接口
- 不读取环境包资产
- 不保证 SQLite 环境生命周期状态同步完整
- 不应作为平台业务主入口

## 3. 请求与响应

```http
POST /api/v1/edge/containers/{id}/start
```

成功响应先返回：

- `taskId`
- `taskType=docker_container_start`
- `resourceType=docker_container`
- `resourceId`
- `eventsUrl`

## 4. 前置校验

- `id` 必须存在
- Docker 必须可达

## 5. 状态流转

- 创建 Edge task
- 后台调用 Docker start

## 6. 成功判定

- 容器启动成功

## 7. 失败判定

- 容器不存在
- Docker start 失败

## 8. 日志字段

- `taskId`
- `containerId`
- `error`

## 9. 联调验收标准

- 成功后可通过 Docker 容器列表看到运行状态变化
- 不应误承诺环境包已经业务可用
