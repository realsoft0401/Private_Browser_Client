# GET /api/v1/edge/node-registration

## 功能目标

返回当前 Client 的 Node 登记协同状态视图。

## 业务边界

- 负责回显当前 Client 对 Node 暴露的 `baseUrl/clientIp/dockerApiUrl`
- 负责实时查询 Node 当前是否已存在这台 Client 的中心登记结果
- 负责同时回显本地 `node-registration.json` 缓存留痕
- 不负责主动注册 Node
- 不负责中心 bind 判定
- 不负责生成 `clientId`

## 前置校验

- 若 `node_register` 配置未启用或不完整，接口仍返回成功包装，但 `lookupStatus` 会给出明确原因

## 状态流转

- 只读接口，不改本机 JSON
- 不改 Node 中心状态

## 请求与响应

### 请求

```http
GET /api/v1/edge/node-registration
```

### 成功响应示例

Node 已找到：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "enabled": true,
    "configReady": true,
    "configMessage": "ready",
    "nodeName": "edge-119",
    "baseUrl": "http://192.168.10.220:3300",
    "clientIp": "192.168.10.220",
    "dockerApiUrl": "http://192.168.10.220:2375",
    "serverBaseUrl": "http://127.0.0.1:3400",
    "mainAccountId": "906090119",
    "registered": true,
    "lookupStatus": "found",
    "lookupMessage": "Node 已存在当前 Client 的中心登记结果",
    "cacheStatus": "cached",
    "cacheMessage": "本地 JSON 已留存上次 Node 分配结果",
    "registration": {
      "clientId": "9060901190001",
      "mainAccountId": "906090119",
      "nodeServerBaseUrl": "http://127.0.0.1:3400",
      "nodeName": "edge-119",
      "baseUrl": "http://192.168.10.220:3300",
      "clientIp": "192.168.10.220",
      "dockerApiUrl": "http://192.168.10.220:2375",
      "source": "remote-list",
      "registeredAt": 1718500000,
      "updatedAt": 1718500000
    }
  }
}
```

Node 未找到：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "registered": false,
    "lookupStatus": "not_found",
    "lookupMessage": "Node 当前尚未登记这台 Client"
  }
}
```

## SSE 说明

- 本接口不使用 SSE
- 原因：这是一次当前态查询，不是长链路任务观察接口

## 任务编排

当前接口不创建 task。

## 成功判定

- 能稳定返回配置状态
- 能在 Node 可达时给出实时登记事实
- Node 不可达时也能给出明确失败原因

## 失败判定

- 当前接口本身不因 Node 查不到而返回业务失败码
- Node 不可达、配置不完整等问题写进 `lookupStatus/lookupMessage`

## 日志字段

- `action=get-node-registration`
- `baseUrl`
- `clientIp`
- `serverBaseUrl`
- `lookupStatus`
- `cacheStatus`

## 联调验收标准

- 未绑定时允许 `registered=false`
- 绑定后能看到 Node 当前返回的中心身份
- 本地 JSON 损坏时要能暴露 `cache_invalid`
