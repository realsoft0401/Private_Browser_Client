# GET /api/v1/edge/docker/containers

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

返回当前项目相关的本机 Docker 容器摘要列表。

## 业务边界

- 只返回本项目相关容器
- 主要区分 `edge-service`、`slot-runtime`、`browser-env-runtime`
- 不把宿主机所有无关容器全量暴露
- 不写 SQLite
- 不替代环境包详情接口

## 前置校验

- Docker 必须可达

## 请求与响应

### 请求

```http
GET /api/v1/edge/docker/containers
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": [
    {
      "id": "477d81bdb34f",
      "names": [
        "private-browser-slot-slot001"
      ],
      "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
      "state": "running",
      "status": "Up 10 minutes",
      "projectRole": "slot-runtime",
      "slotId": "slot001",
      "envId": ""
    }
  ]
}
```

## 状态流转

- 只读接口，不改状态

## SSE 说明

- 本接口不用 SSE

## 任务编排

- 不创建 task

## 成功判定

- 能正确过滤并返回本项目相关容器

## 失败判定

- Docker API 不可达
- 容器列表读取失败

## 日志字段

- `action=list-docker-containers`
- `dockerApiUrl`
- `containersCount`
- `error`

## 联调验收标准

- `slot` 容器能带出 `slotId`
- 运行中浏览器环境容器能带出 `envId`
- 无关容器不会被误返回
