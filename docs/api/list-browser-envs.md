# GET /api/v1/edge/browser-envs

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

查询当前 `Private_Browser_Client` 本机的浏览器环境包索引列表，给 Server、管理员和后续前端提供统一资产摘要入口。

这条接口解决的是“看本机有哪些环境包、它们大致处于什么状态”，不是读取完整配置原文，也不是替代单环境详情接口。

## 业务边界

- 负责从本机 SQLite `browser_envs` 索引读取列表摘要
- 负责按 `userId/rpaType/status` 筛选
- 负责分页
- 负责返回按状态、按类型的简单统计
- 负责在 `running` 或 `occupied` 关系成立时补充当前可连接摘要
- 不返回 `proxy/clash.yaml` 明文
- 不返回 fingerprint raw
- 不返回 `browser-data/profile` 内容
- 不替代 `GET /api/v1/edge/browser-envs/{envId}`

## 前置校验

- Client 本机 SQLite 可读
- 查询参数必须合法
- `userId` 如传入，必须通过路径与格式安全校验
- `status` 如传入，必须属于正式生命周期枚举
- `page/pageSize` 必须在允许范围内

## 请求与响应

### 请求

```http
GET /api/v1/edge/browser-envs?userId=906090001&rpaType=tk&status=running&page=1&pageSize=20
Accept: application/json
```

### Query 参数

- `userId`
  - 选填
  - 按主账号或业务账号标识筛选
- `rpaType`
  - 选填
  - 例如 `tk`
- `status`
  - 选填
  - 建议枚举：`created`、`running`、`stopped`、`backed_up`、`deleted`、`error`
  - 不传时默认排除 `deleted`
- `page`
  - 选填
  - 默认 `1`
- `pageSize`
  - 选填
  - 默认 `20`
  - 建议最大 `100`

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "total": 1,
    "page": 1,
    "pageSize": 20,
    "byStatus": {
      "running": 1
    },
    "byRpaType": {
      "tk": 1
    },
    "items": [
      {
        "envId": "906090001_tk_324867594169356288",
        "userId": "906090001",
        "rpaType": "tk",
        "name": "tk-main-account",
        "envSequence": 1,
        "cdpPort": 9201,
        "vncPort": 9101,
        "vncUrl": "vnc://127.0.0.1:9101",
        "vncWsUrl": "ws://127.0.0.1:3300/api/v1/edge/slots/slot001/vnc/ws",
        "webVncUrl": "http://127.0.0.1:3300/web-vnc.html?slot=slot001",
        "envPath": "data/browser-envs/users/906090001/tk/906090001_tk_324867594169356288",
        "status": "running",
        "containerName": "private-browser-slot-slot001",
        "containerStatus": "running",
        "monitorStatus": "unknown",
        "fingerprintRestored": false,
        "hasBrowserData": true,
        "createdAt": 1718500000,
        "updatedAt": 1718500300
      }
    ]
  }
}
```

## 状态流转

- 本接口只读，不改变任何状态
- 返回时主状态优先看 `browser_envs.status`
- `containerStatus`、`monitorStatus` 只作为运行事实摘要

## SSE 说明

- 本接口不用 SSE
- 原因：它是立即返回的只读查询接口，不是长链路、多阶段动作

## 任务编排

- 本接口不创建 task
- 也不消费 `taskId`

## 成功判定

- 参数合法
- SQLite 查询成功
- 返回的分页、统计、列表摘要与当前本机索引一致

## 失败判定

- 查询参数非法
- SQLite 不可达或查询失败
- 状态枚举非法

## 日志字段

- `action=list-browser-envs`
- `userId`
- `rpaType`
- `status`
- `page`
- `pageSize`
- `resultCount`
- `error`

## 联调验收标准

- 能按 `userId/rpaType/status` 正确筛选
- 不传 `status` 时默认不返回 `deleted`
- 返回结果不包含代理明文和登录态内容
- 运行中环境可返回当前 `slot` 视角连接摘要
