# Edge API 设计：`POST /api/v1/edge/containers/{id}/restart`

## 1. 功能目标

`POST /api/v1/edge/containers/{id}/restart` 用于重启本机指定 Docker 容器。

## 2. 业务边界

- 这是内网运维诊断接口
- 不替代环境包 `run/stop`
- 不保证环境包配置与 SQLite 运行态完整回写

## 3. 请求与响应

```http
POST /api/v1/edge/containers/{id}/restart
```

可选请求体：

- `timeoutSeconds`

成功响应先返回：

- `taskId`
- `taskType=docker_container_restart`
- `eventsUrl`

## 4. 前置校验

- `id` 必须存在
- Docker 必须可达

## 5. 状态流转

- 创建 Edge task
- 后台调用 Docker restart

## 6. 成功判定

- 容器重启成功

## 7. 失败判定

- 容器不存在
- Docker restart 失败

## 8. 日志字段

- `taskId`
- `containerId`
- `timeoutSeconds`
- `error`

## 9. 联调验收标准

- 能从 SSE 看到任务过程
- 成功后容器重新进入 running
