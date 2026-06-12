# Edge API 设计：`GET /api/v1/edge/browser-envs`

## 1. 功能目标

`GET /api/v1/edge/browser-envs` 用于返回当前 Edge 节点本机 SQLite 中的环境包摘要列表。

## 2. 业务边界

- 这是本机环境包索引视图
- 不扫描中心 Server
- 不主动刷新所有 Docker 容器事实
- 不返回完整 profile 或登录态资产

## 3. 请求与响应

```http
GET /api/v1/edge/browser-envs
```

支持过滤：

- `userId`
- `rpaType`
- `status`
- `page`
- `pageSize`

返回重点：

- `total`
- `byStatus`
- `byRpaType`
- `items`

列表项重点字段：

- `envId`
- `userId`
- `rpaType`
- `status`
- `containerStatus`
- `monitorStatus`
- `vncUrl`
- `vncWsUrl`
- `webVncUrl`

## 4. 前置校验

- 查询参数格式必须合法

## 5. 状态流转

- 只读，不改状态

## 6. 成功判定

- 能按过滤条件返回本机环境包摘要

## 7. 失败判定

- 查询参数非法
- SQLite 查询失败

## 8. 日志字段

- `userId`
- `rpaType`
- `status`
- `page`
- `pageSize`
- `error`

## 9. 联调验收标准

- 默认不返回 `deleted`
- 显式 `status=deleted` 时能查回收站
- 不泄露 proxy 明文和 browser-data 内容
