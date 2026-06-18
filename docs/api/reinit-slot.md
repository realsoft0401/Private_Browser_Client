# POST /api/v1/edge/slots/{slotId}/reinit

## 功能目标

重初始化指定 slot，把它收回 `waiting` 初始态。

## 业务边界

- 负责销毁旧 slot runtime 容器并重新初始化
- 负责清空 `currentPackageId/currentRunId/lastError`
- 负责保留同一个 `slotId`
- 不负责强制结束仍在占用中的运行关系
- 不负责生成新的 slot 记录

## 前置校验

- `slotId` 必须存在
- slot 不能处于 `occupied/loading/releasing`

## 状态流转

成功后：

- 清空旧运行关系引用
- 重建 slot runtime 容器
- 重新分配并回写端口、容器摘要
- 最终收口为 `waiting`

失败后：

- 维持原 slot 事实或返回错误

## 请求与响应

### 请求

```http
POST /api/v1/edge/slots/slot001/reinit
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "slotId": "slot001",
    "status": "waiting",
    "currentPackageId": null,
    "currentRunId": null,
    "lastError": null,
    "cdpPort": 9222,
    "vncPort": 5901
  }
}
```

### 失败响应

```json
{
  "code": 1003,
  "message": "数据状态冲突"
}
```

## SSE 说明

- 本接口当前不使用 SSE
- 原因：slot 重初始化按当前设计是同步收口动作，调用方需要立即知道是否已回到 `waiting`

## 任务编排

当前接口是同步接口，不创建 task。

## 成功判定

- slot 不在占用中
- runtime 重初始化成功
- 最终状态为 `waiting`

## 失败判定

- slot 不存在
- slot 仍被占用
- Docker 容器销毁失败
- Docker 容器重建失败

## 日志字段

- `action=reinit-slot`
- `slotId`
- `previousContainerId`
- `newContainerId`
- `error`

## 联调验收标准

- waiting 态 slot 可以重初始化成功
- occupied 态 slot 必须拒绝
