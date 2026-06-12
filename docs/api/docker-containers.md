# Edge API 设计：`GET /api/v1/edge/docker/containers`

## 1. 功能目标

`GET /api/v1/edge/docker/containers` 用于返回当前项目相关的本机 Docker 容器列表。

它主要区分两类：

- `edge-service`
- `browser-env`

## 2. 业务边界

- 只返回本项目相关容器
- 不把所有宿主机容器无差别暴露出来
- 不写 SQLite
- 不替代环境包详情接口

## 3. 请求与响应

```http
GET /api/v1/edge/docker/containers
```

返回重点：

- `id`
- `names`
- `image`
- `ports`
- `labels`
- `state`
- `status`
- `projectRole`
- `envId`

## 4. 前置校验

- Docker 必须可达

## 5. 状态流转

- 只读，不改状态

## 6. 成功判定

- 能正确过滤并返回本项目相关容器

## 7. 失败判定

- Docker API 不可达
- 容器列表读取失败

## 8. 日志字段

- `dockerApiUrl`
- `containersCount`
- `error`

## 9. 联调验收标准

- `browser-env` 容器能带出 `envId/userId/rpaType`
- 不相关容器不会被误返回
