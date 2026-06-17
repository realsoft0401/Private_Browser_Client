# POST /api/v1/edge/browser-envs

## 功能目标

在当前 `Private_Browser_Client` 本机创建一个新的浏览器环境包。

这条接口只负责环境包资产建立，不负责启动 Docker 容器，不负责自动运行环境。

## 设计方式

当前按 old 的设计方式执行：

- `profile.json` 是唯一详细配置文档
- `profile.json` 不增加状态字段
- `profile.json` 不保存 `running/stopped/error`
- 创建成功后主状态进入 `created`
- 运行态事实留给后续 `run/stop/status sync/Docker`

## 业务边界

- 负责生成 `envId = userId_rpaType_snowflakeId`
- 负责生成 `bindingId`
- 负责分配 `envSequence`
- 负责分配本机 `CDP/VNC` 端口
- 负责创建正式目录 `data/browser-envs/users/{userId}/{rpaType}/{envId}`
- 负责写入：
  - `profile.json`
  - `binding.json`
  - `container.json`
  - `proxy/clash.yaml`
  - `proxy/proxy-runtime.json`
  - `fingerprint/*`
  - `browser-data/profile`
  - `logs/`
- 负责写入 SQLite `browser_envs` 索引
- 不负责启动容器
- 不负责自动 run
- 不负责平台额度或中心身份判断

## 当前固定配置

- `profile.environment.language` 固定为 `us-en`
- 不接受调用方自由传入 `language`
- `runtime.image` 必须在 create 阶段就写入

## 前置校验

- `userId` 必须合法并通过路径安全校验
- `rpaType` 必须合法
- `name` 必须合法
- `runtime.image` 必填
- `environment.timezone` 必填
- `environment.screen` 关键字段完整
- `proxy` 配置合法
- fingerprint 恢复材料如果提供，必须合法
- 本机能分配新的 `envSequence`
- 本机能分配新的 `CDP/VNC` 端口
- 目标目录不能已存在同名环境包

## 请求与响应

### 请求

```http
POST /api/v1/edge/browser-envs
Content-Type: application/json
```

```json
{
  "userId": "906090001",
  "rpaType": "tk",
  "name": "tk-main-account",
  "runtime": {
    "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
    "startupUrl": "https://www.tiktok.com",
    "shmSize": "1g"
  },
  "environment": {
    "timezone": "Asia/Shanghai",
    "screen": {
      "width": 1440,
      "height": 900,
      "depth": 24
    }
  },
  "proxy": {
    "enabled": true,
    "type": "clash",
    "configBase64": "cG9ydDogNzg5MAptb2RlOiBydWxlCg=="
  }
}
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "envId": "906090001_tk_324867594169356288",
    "userId": "906090001",
    "rpaType": "tk",
    "envSequence": 12,
    "ports": {
      "cdp": 8112,
      "vnc": 9112
    },
    "envPath": "data/browser-envs/users/906090001/tk/906090001_tk_324867594169356288",
    "files": {
      "profile": "profile.json",
      "binding": "binding.json",
      "container": "container.json"
    },
    "identityHash": "sha256:xxx",
    "createdAt": 1718500000
  }
}
```

## 状态流转

- 创建前：不存在
- 创建成功后：`browser_envs.status = created`
- `container_status = missing`
- `monitor_status = unknown`
- 不自动进入 `running`

## SSE 说明

- 本接口当前不使用 SSE
- 原因：虽然步骤较多，但本质是一次本地快速文件创建和索引写入
- 同步 HTTP 足够表达成功或失败

## 任务编排

- 当前接口不创建独立 task
- 创建链路在一次同步请求里完成目录创建、文件落盘和索引写入

## 成功判定

- `envId` 成功生成
- `envSequence` 成功分配
- 本机端口成功分配
- 正式目录创建成功
- 原子文件写入成功
- SQLite 索引写入成功
- `profile.json` 中 `environment.language` 固定写入 `us-en`
- 主状态进入 `created`

## 失败判定

- 参数非法
- `userId` / `rpaType` 非法
- `runtime.image` 缺失
- 代理配置非法
- fingerprint 数据非法
- 端口分配失败
- 目标目录冲突
- 文件写入失败
- SQLite 写入失败

## 失败回滚规则

- 创建失败时必须清理本次新建目录
- 不能留下半成品 `envPath`
- 不能留下“目录创建成功但索引未写入”的脏状态

## 日志字段

- `action=create-browser-env`
- `envId`
- `userId`
- `rpaType`
- `envSequence`
- `envPath`
- `runtimeImage`
- `identityHash`
- `stage`
- `result`
- `error`

## 联调验收标准

- 返回新的 `envId`
- 正式目录创建成功
- `profile.json` / `binding.json` / `container.json` 都存在
- SQLite 能查到该环境
- 主状态为 `created`
- `profile.environment.language` 固定为 `us-en`
- 不会自动 run
