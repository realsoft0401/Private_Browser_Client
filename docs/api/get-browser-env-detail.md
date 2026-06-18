# GET /api/v1/edge/browser-envs/{envId}

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

读取单个浏览器环境包的完整详情，用于管理员排障、Node Server 校验和前端详情页展示。

这条接口解决的是“单个环境包现在到底长什么样、缺什么、状态是否一致”，不是执行生命周期动作。

## 业务边界

- 负责组合 SQLite 索引、`profile.json`、`binding.json`、`container.json`
- 负责返回代理摘要、指纹摘要、一致性检查结果
- 负责返回当前 `slot` / VNC / CDP 可连接摘要
- 不返回 `proxy/clash.yaml` 明文
- 不返回 fingerprint raw
- 不返回 `browser-data/profile` 文件内容
- 不替代 `run/stop/backup/restore/delete`

## 前置校验

- `envId` 必须存在于本机索引
- 索引对应路径必须位于受控 `data/browser-envs` 目录内
- 详情读取失败时必须明确指出缺失材料

## 请求与响应

### 请求

```http
GET /api/v1/edge/browser-envs/906090001_tk_324867594169356288
Accept: application/json
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "index": {
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
      "createdAt": 1718500000,
      "updatedAt": 1718500300
    },
    "profile": {
      "envId": "906090001_tk_324867594169356288",
      "runtime": {
        "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64"
      },
      "ports": {
        "cdp": 9201,
        "vnc": 9101
      }
    },
    "binding": {
      "id": "binding-906090001-tk-0001",
      "version": 3,
      "identityHash": "sha256:xxx",
      "identity": {
        "envId": "906090001_tk_324867594169356288",
        "userId": "906090001",
        "rpaType": "tk"
      }
    },
    "container": {
      "envId": "906090001_tk_324867594169356288",
      "containerName": "private-browser-slot-slot001",
      "status": "running",
      "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
      "ports": {
        "cdp": 9201,
        "vnc": 9101
      }
    },
    "proxy": {
      "enabled": true,
      "type": "clash",
      "mode": "rule",
      "configPath": "proxy/clash.yaml"
    },
    "consistency": {
      "profileMatchesIndex": true,
      "identityHashMatches": true,
      "proxyConfigExists": true,
      "browserDataExists": true,
      "errors": []
    },
    "vnc": {
      "vncUrl": "vnc://127.0.0.1:9101",
      "vncWsUrl": "ws://127.0.0.1:3300/api/v1/edge/slots/slot001/vnc/ws",
      "webVncUrl": "http://127.0.0.1:3300/web-vnc.html?slot=slot001"
    }
  }
}
```

## 状态流转

- 本接口只读，不改变任何状态
- 但必须把“资产主状态”和“容器事实状态”分开返回
- 用户主状态仍以 `browser_envs.status` 为准

## SSE 说明

- 本接口不用 SSE
- 原因：单次只读详情，不存在长时后台编排

## 任务编排

- 本接口不创建 task

## 成功判定

- 能稳定读取索引和关键文件
- 能返回一致性检查摘要
- 对缺失文件能如实标记，而不是静默伪装成功

## 失败判定

- `envId` 不存在
- 索引存在但受控目录越界
- 关键文件不可读且无法形成最小详情响应

## 日志字段

- `action=get-browser-env-detail`
- `envId`
- `envPath`
- `status`
- `containerStatus`
- `consistencyErrors`
- `error`

## 联调验收标准

- 返回 index/profile/binding/container 四层核心摘要
- 不泄露代理明文、fingerprint raw、登录态内容
- 一旦存在一致性问题，`consistency.errors` 要明确指出
