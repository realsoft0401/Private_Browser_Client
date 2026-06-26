# POST /api/v1/edge/node-registration/clear

## 功能目标

接收 Node Server 在 unbind 成功后的清理请求，并删除本地 `node-registration.json` 留痕。

> 当前文档定位：这是 Node unbind 成功后清理 Client 本地身份留痕的正式配套接口，但它不参与 discovery，也不直接承担业务放行。
> 最新总口径下，中心 `clientId` 身份真相属于 `Private_Browser_Server`；Client 这里只负责清理本地缓存，不反向改中心事实。

## 业务边界

- 负责删除本地 `data/node-registration.json`
- 负责校验 `X-Edge-API-Key`
- 负责把“文件原本是否存在”明确回给调用方
- 不负责中心 unbind 决策
- 不负责变更中心 `clientId`
- 不负责清理 SQLite、slot、browser-env 或任何登录态资产

## 前置校验

- Header 必须带 `X-Edge-API-Key`
- 请求体必须是合法 JSON
- `source` 和 `clearedAt` 可为空

## 状态流转

成功后：

- 本地 `node-registration.json` 被删除
- 如果文件原本不存在，也按成功收口

失败后：

- 本地缓存文件保持原样

## 请求与响应

### 请求

```http
POST /api/v1/edge/node-registration/clear
X-Edge-API-Key: private-browser-edge-key
Content-Type: application/json
```

```json
{
  "source": "node-unbind",
  "clearedAt": 1718500001
}
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "cleared": true,
    "existed": true,
    "cachePath": "/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/data/node-registration.json",
    "clearedAt": 1718500001
  }
}
```

文件原本就不存在：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "cleared": true,
    "existed": false,
    "cachePath": "/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/data/node-registration.json",
    "clearedAt": 1718500001
  }
}
```

### 失败响应

未授权：

```json
{
  "code": 1006,
  "message": "clear node registration failed: X-Edge-API-Key 无效"
}
```

参数错误：

```json
{
  "code": 1001,
  "message": "clear node registration failed: request body 非法"
}
```

## SSE 说明

- 本接口不使用 SSE
- 原因：clear 是一次短链路本地文件清理动作，同步 HTTP 已足够表达最终结果

## 任务编排

当前接口不创建 task。

## 口径说明

- 该接口只用于 unbind 收口和本地缓存清理
- 即使清理成功，也不代表中心已经自动重新绑定或自动重新发现
- 重新绑定后，新的本地 JSON 只能由 Node 再次调用 assign 写入

## 成功判定

- API Key 合法
- 请求体是合法 JSON
- 本地缓存文件删除成功，或原本就不存在

## 失败判定

- API Key 缺失或错误
- 请求体不是合法 JSON
- 本地文件存在但删除失败

## 日志字段

- `action=node-registration-clear`
- `source`
- `cachePath`
- `existed`
- `clearedAt`

## 联调验收标准

- Node unbind 后调用这条接口，Client 本地 JSON 必须被删除
- 当本地文件已不存在时，接口仍然返回 success，且 `existed=false`
- 该接口执行后，slot、browser-env、SQLite 记录都不能被误删
