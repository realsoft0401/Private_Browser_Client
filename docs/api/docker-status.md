# Edge API 设计：`GET /api/v1/edge/docker/status`

## 1. 功能目标

`GET /api/v1/edge/docker/status` 用于快速返回当前 Edge 节点本机 Docker 可用性摘要。

## 2. 业务边界

- 只返回 Docker 是否可用、镜像数、容器数
- 不返回完整镜像或容器列表
- 不写 SQLite
- 不创建任务

## 3. 请求与响应

```http
GET /api/v1/edge/docker/status
```

返回重点：

- `dockerApiUrl`
- `status`
- `message`
- `imagesCount`
- `containersCount`
- `checkedAt`

## 4. 前置校验

- Docker API 配置必须可读

## 5. 状态流转

- 只读，不改状态

## 6. 成功判定

- 成功返回本机 Docker 摘要

## 7. 失败判定

- Docker API 不可达
- Docker 状态读取失败

## 8. 日志字段

- `dockerApiUrl`
- `status`
- `error`

## 9. 联调验收标准

- Docker 正常时 `status=available`
- Docker 故障时返回明确错误
