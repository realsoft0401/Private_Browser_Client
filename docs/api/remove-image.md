# Edge API 设计：`POST /api/v1/edge/docker/remove-image`

## 1. 功能目标

`POST /api/v1/edge/docker/remove-image` 用于在当前 Edge 节点本机删除指定 Docker 镜像。

## 2. 业务边界

- 只作用于本机 Docker 镜像
- 不删除环境包目录
- 不删除 SQLite 索引
- 不根据环境包引用关系自动修复资产

## 3. 请求与响应

```http
POST /api/v1/edge/docker/remove-image
```

请求重点：

- `image`
- `force`
- `noPrune`

成功响应先返回：

- `taskId`
- `taskType=docker_remove_image`
- `resourceType=docker_image`
- `eventsUrl`

## 4. 前置校验

- `image` 不能为空
- Docker 必须可达

## 5. 状态流转

- 创建本机内存 Edge task
- 后台执行 Docker remove
- 最终通过 `done/error` 事件收口

## 6. 任务编排

```text
create task
  -> docker remove image
  -> emit done or error
```

## 7. 成功判定

- Docker 删除成功
- 任务进入 `done`

## 8. 失败判定

- 镜像不存在
- 镜像仍被容器引用
- Docker 删除失败

## 9. 日志字段

- `taskId`
- `image`
- `force`
- `noPrune`
- `error`

## 10. 联调验收标准

- 删除成功后镜像列表不再显示该标签
- 被引用镜像失败时能保留明确错误
