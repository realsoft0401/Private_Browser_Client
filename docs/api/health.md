# Edge API 设计：`GET /health`

## 1. 功能目标

`GET /health` 用于返回当前 `Private_Browser_Client` 自身的本机健康摘要。

它回答的是：

- Edge HTTP 服务是否活着
- SQLite、Docker、data 目录是否正常
- 状态同步 Worker 是否在运行

它不回答：

- 节点是否已经被中心 `verified`
- 节点是否 `offline/stale`
- 某个环境包是否可业务放行

## 2. 业务边界

- 只返回本机视角
- 不读中心节点表
- 不读中心任务
- 不扫描所有环境包做业务结论

## 3. 请求与响应

```http
GET /health
```

无请求体，无 `clientId`。

返回重点：

- `ok`
- `status`
- `service`
- `version`
- `dockerApi`
- `checks`
- `statusSync`

## 4. 前置校验

- 无认证、无 Header 前置要求

## 5. 状态流转

- 本接口不改变任何状态
- 只读取当前进程和本机依赖的即时事实

## 6. 成功判定

- HTTP 200 正常返回健康结构
- `checks` 至少包含 API、SQLite、Docker、dataDir、statusSync 关键检查

## 7. 失败判定

- 服务进程未启动
- 健康结构构建失败

## 8. 日志字段

- `service`
- `status`
- `dockerApi`
- `error`

## 9. 联调验收标准

- 服务启动后能稳定返回 200
- Docker 不可用时 `status` 应转为 `unhealthy`
- 不出现 `verified/offline/stale` 这类中心状态字段
