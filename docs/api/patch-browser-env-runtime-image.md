# PATCH /api/v1/edge/browser-envs/{envId}/runtime-image

## 当前状态

已实现并已回归。

## 功能目标

修改指定 browser-env 的正式运行镜像地址。

这条接口只修改环境包自己的 `profile.runtime.image`，用于后续下一次 run 使用新镜像。

## 业务边界

负责：

- 校验环境包当前处于 `waiting`
- 更新 `profile.runtime.image`
- 同步更新 `container.json.image`
- 同步更新 SQLite 摘要中的 `updatedAt/lastError`

不负责：

- 不自动 run
- 不自动 pull image
- 不自动 reinit slot
- 不修改 slot 当前实际 `runtimeImage`
- 不删除旧镜像
- 不修改 proxy、fingerprint、browser-data/profile
- 不允许临时透传任意 Docker 参数

## 状态前置条件

正式业务口径只有下面状态允许修改：

- `stopped`

当前实现补充：

- `created` 表示首次运行前配置态，还没有挂载 slot 或运行容器
- `stopped` 表示运行后已经释放 slot/container 关系，是配置与容器隔离后的干净态

必须拒绝：

- `loading`
- `running`
- `ending`
- `backed_up`
- `deleted`
- `error`

关键原则：

- 镜像地址是环境包运行契约的一部分，不能在运行中热改。
- 修改后不代表当前容器已经切换镜像。
- 下一次 `run` 才会读取新的 `profile.runtime.image` 创建运行容器。

## 请求

```http
PATCH /api/v1/edge/browser-envs/906090001_tk_324867594169356288/runtime-image
Content-Type: application/json
```

```json
{
  "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.2-amd64"
}
```

字段：

- `image`
  - 必填
  - 完整 Docker image 引用
  - 不拆分 repository/tag

## 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "envId": "906090001_tk_324867594169356288",
    "status": "waiting",
    "previousImage": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64",
    "image": "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.2-amd64",
    "updatedAt": 1718501000
  }
}
```

## SSE 说明

不使用 SSE。

原因：

- 这是短链路配置修改动作。
- 不拉镜像、不创建容器、不执行 run。
- 同步 HTTP 足够表达成功或失败。

## 失败判定

- env 不存在
- env 当前不是 `created` 或 `stopped`
- image 为空或格式非法
- profile/container 文件写入失败
- SQLite 摘要更新失败

## 回归记录

- 2026-07-01：使用远端 Client `192.168.111.119:3300` 完成回归
- 测试镜像：`crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.2-amd64`
- `created/stopped` 修改准入已按最终口径收口
- `running` 状态修改会被拒绝
- 修改后不自动 run、不自动 pull image、不自动 reinit slot
- 下一次 run 已确认读取新 `runtime.image`
- stop + slot reinit 后，env 保持 `stopped` 且 slot 回到空白 `waiting`

## 与相近接口的边界

- `POST /api/v1/edge/docker/pull-image`
  - 只提前拉取镜像，不修改 env 使用哪张镜像
- `DELETE /api/v1/edge/browser-envs/{envId}/del`
  - 只删除当前 env 关联的本机镜像，不修改 runtime.image
- `POST /api/v1/edge/slots/{slotId}/reinit`
  - 只重建 slot 基础容器，不修改 browser-env 运行镜像
- `POST /api/v1/edge/browser-envs/{envId}/run`
  - 读取当前 `profile.runtime.image` 执行运行，不接受请求体临时覆盖 image
