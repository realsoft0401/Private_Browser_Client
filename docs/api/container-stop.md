# Edge API 设计：`POST /api/v1/edge/containers/{id}/stop`

## 1. 功能目标

`POST /api/v1/edge/containers/{id}/stop` 用于停止本机指定 Docker 容器。

## 2. 业务边界

- 这是内网运维诊断接口
- 不等价于环境包 `stop`
- 不负责 browser-data/profile 资产语义

## 3. 请求与响应

```http
POST /api/v1/edge/containers/{id}/stop
```

可选请求体：

- `timeoutSeconds`

成功响应先返回：

- `taskId`
- `taskType=docker_container_stop`
- `eventsUrl`

## 4. 前置校验

- `id` 必须存在
- Docker 必须可达

## 5. 状态流转

- 创建 Edge task
- 后台调用 Docker stop

## 6. 成功判定

- 容器停止成功

## 7. 失败判定

- 容器不存在
- Docker stop 失败

## 8. 日志字段

- `taskId`
- `containerId`
- `timeoutSeconds`
- `error`

## 9. 联调验收标准

- 成功后容器状态变为 exited 或等价非运行态
- 不把裸容器 stop 误当成环境包主状态已同步
