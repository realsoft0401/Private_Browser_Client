# GET /api/v1/edge/docker/status

## 当前状态

- 正式协议已收口
- 当前新 Client 代码暂未实现

## 功能目标

返回当前 Client 本机 Docker 的最小可用性摘要。

## 业务边界

- 只返回 Docker 健康摘要
- 只返回镜像数、容器数、连接地址等最小事实
- 不返回完整镜像列表或容器列表
- 不写 SQLite
- 不创建 task

## 前置校验

- Docker API 配置必须可读

## 请求与响应

### 请求

```http
GET /api/v1/edge/docker/status
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "dockerApiUrl": "http://192.168.10.119:2375",
    "status": "available",
    "message": "docker is reachable",
    "imagesCount": 8,
    "containersCount": 5,
    "checkedAt": 1718501000
  }
}
```

## 状态流转

- 只读接口，不修改任何本机状态

## SSE 说明

- 本接口不用 SSE

## 任务编排

- 不创建 task

## 成功判定

- 能返回本机 Docker 可用性摘要

## 失败判定

- Docker API 不可达
- Docker 状态读取失败

## 日志字段

- `action=get-docker-status`
- `dockerApiUrl`
- `status`
- `error`

## 联调验收标准

- Docker 正常时 `status=available`
- Docker 故障时返回明确错误
