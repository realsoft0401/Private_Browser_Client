# POST /api/v1/edge/node-registration/assign

## 功能目标

接收 Node Server 在 bind 成功后反向下发的 `clientId/accountId`，并写入本地 `node-registration.json` 留痕。

## 业务边界

- 负责接收 Node 已经决定好的中心身份结果
- 负责校验 `X-Edge-API-Key`
- 负责把结果落到本地 JSON
- 不负责中心 bind 决策
- 不负责账号权限判断
- 不负责生成新的 `clientId`

## 前置校验

- Header 必须带 `X-Edge-API-Key`
- `clientId` 必填
- `accountId` 必填

## 状态流转

成功后：

- 覆盖写入本地 `node-registration.json`
- 返回写入结果和缓存路径

失败后：

- 不写入 JSON

## 请求与响应

### 请求

```http
POST /api/v1/edge/node-registration/assign
X-Edge-API-Key: private-browser-edge-key
Content-Type: application/json
```

```json
{
  "clientId": "9060901190001",
  "accountId": "906090119",
  "source": "node-bind",
  "assignedAt": 1718500000
}
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "written": true,
    "cachePath": "/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/data/node-registration.json",
    "registration": {
      "clientId": "9060901190001",
      "mainAccountId": "906090119",
      "nodeServerBaseUrl": "http://127.0.0.1:3400",
      "nodeName": "edge-119",
      "baseUrl": "http://127.0.0.1:3300",
      "clientIp": "127.0.0.1",
      "dockerApiUrl": "http://127.0.0.1:2375",
      "source": "node-bind",
      "registeredAt": 1718500000,
      "updatedAt": 1718500001
    }
  }
}
```

### 失败响应

未授权：

```json
{
  "code": 1006,
  "message": "assign clientId failed: X-Edge-API-Key 无效"
}
```

参数错误：

```json
{
  "code": 1001,
  "message": "assign clientId failed: assign request invalid"
}
```

## SSE 说明

- 本接口不使用 SSE
- 原因：assign 是一次短链路写入动作，同步 HTTP 已足够表达成功或失败

## 任务编排

当前接口不创建 task。

## 成功判定

- API Key 合法
- 请求体合法
- JSON 写入成功

## 失败判定

- API Key 缺失或错误
- `clientId/accountId` 缺失
- 本地 JSON 目录不可写

## 日志字段

- `action=node-registration-assign`
- `clientId`
- `accountId`
- `source`
- `cachePath`
- `overwrittenClientId`

## 联调验收标准

- bind 成功后 Node 调这个接口，Client 本地 JSON 必须有留痕
- 重复下发同一个 `clientId` 允许覆盖
- 下发新 `clientId/accountId` 允许覆盖，但必须保留覆盖日志
