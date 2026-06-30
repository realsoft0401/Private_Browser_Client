# DELETE /api/v1/edge/browser-envs/{envId}/del

## 当前状态

- 正式协议已收口
- 当前新 Client 代码已实现

## 功能目标

删除某个环境包关联的本机运行镜像。

这条接口只解决“镜像清理”问题，不碰环境包资产本身，避免和 `/package` 彻底删除混淆。

## 业务边界

- 负责从 `profile.runtime.image` 读取镜像名
- 负责调用本机 Docker 删除镜像
- 不删除环境包目录
- 不删除 `browser-data/profile`
- 不删除 `profile.json`、`binding.json`、`container.json`
- 不删除 SQLite 索引
- 不自动 stop
- 不创建 SSE task

## 前置校验

- `envId` 必须存在
- 环境包当前不能是 `running`
- Docker 必须可达
- `runtime.image` 必须存在

## 请求与响应

### 请求

```http
DELETE /api/v1/edge/browser-envs/906090001_tk_324867594169356288/del
Accept: application/json
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "envId": "906090001_tk_324867594169356288",
    "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
    "imageRemoved": true,
    "results": [
      {
        "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
        "deleted": "sha256:abc123",
        "untagged": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64"
      }
    ],
    "warningMessage": "",
    "deletedAt": 1718501000
  }
}
```

## 状态流转

- 本接口不改变环境包主状态
- 只影响本机 Docker 镜像事实
- 如果镜像删除后环境包仍存在，后续下次 `run` 再决定是否重新拉镜像

## SSE 说明

- 本接口不用 SSE
- 原因：镜像删除是单次同步动作，返回一次结果即可

## 任务编排

- 本接口不创建 task

## 成功判定

- `runtime.image` 成功解析
- Docker 删除镜像成功，或能明确返回镜像已不存在的受控结果

## 失败判定

- `envId` 不存在
- 环境仍在 `running`
- Docker 不可达
- 镜像仍被其它容器占用
- Docker 删除镜像失败

## 日志字段

- `action=delete-browser-env-image`
- `envId`
- `image`
- `imageRemoved`
- `warningMessage`
- `error`

## 联调验收标准

- `/del` 不能删除环境包目录和索引
- `running` 状态调用必须明确拒绝
- 如果镜像被占用，错误或 warning 必须可排查
