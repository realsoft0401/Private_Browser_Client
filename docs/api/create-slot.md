# POST /api/v1/edge/slots

## 功能目标

在当前 `Private_Browser_Client` 上创建一个新的本机 `slot` 当前态记录，并立即初始化这个 slot 对应的本机常驻运行容器。

> 当前文档定位：`slot` 是 Client 本机资源层能力。它服务 `browser-env` 运行承载与运维观察，不是平台正式业务资产主线。

## 业务边界

- 负责接收 `slotId`
- 负责在 Client 本机创建 slot 当前态
- 负责初始化 slot 容器、分配本机 `CDP/VNC` 端口、回写容器摘要
- 负责把创建成功后的 slot 状态收口到 `waiting`
- 不负责 package 资产创建
- 不负责平台端槽位额度判断
- 不负责 Node Server 中心配额控制
- 不负责把 package 放进 slot 运行
- 不负责替代 `browser-envs/*` 正式生命周期

## 前置校验

- 请求体必须可解析
- `slotId` 必填
- `slotId` 去空格后不能为空
- `slotId` 格式固定为 `slot001` 这种三位编号形式
- 同一个 `slotId` 不能重复创建
- 如果已配置 slot runtime 容器能力，本机 Docker 必须可达

## 状态流转

进入链路时：

- 先写入一条 `status=loading` 的 slot 记录

成功后：

- 初始化常驻运行容器
- 分配并回写 `cdpPort/vncPort`
- 回写 `containerId/containerName/runtimeImage/containerStatus`
- 最终收口为 `status=waiting`

失败后：

- 删除本次新建 slot 记录
- 不保留半初始化 slot

## 请求与响应

### 请求

```http
POST /api/v1/edge/slots
Content-Type: application/json
```

```json
{
  "slotId": "slot001"
}
```

### 成功响应

- HTTP Status: `201 Created`

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "slotId": "slot001",
    "status": "waiting",
    "containerName": "browser-slot-slot001",
    "runtimeImage": "private/browser-slot-runtime:latest",
    "cdpPort": 9222,
    "vncPort": 5901,
    "initializedAt": 1718500000,
    "updatedAt": 1718500000
  }
}
```

### 失败响应

参数错误：

```json
{
  "code": 1001,
  "message": "请求体解析失败，请检查 slotId"
}
```

冲突：

```json
{
  "code": 1003,
  "message": "数据状态冲突"
}
```

依赖失败示例：

```json
{
  "code": 1005,
  "message": "slot runtime start container failed: status=500 body=no such image"
}
```

## SSE 说明

- 本接口当前不使用 SSE
- 原因：slot 创建按当前设计是同步初始化动作，调用方需要立即拿到 slot 当前态结果

## 任务编排

当前接口是同步接口，不创建独立 task。

## 口径说明

- `create-slot` 属于 Client 本机资源层能力
- 正式业务资产仍然是 `browser-env`
- 后续对外平台文档、BP 和研发架构叙事，不应把 `slot` 描述成产品核心卖点

## 成功判定

- `slotId` 合法
- slot 记录写入成功
- slot runtime 初始化成功
- 最终返回 `status=waiting`

## 失败判定

- `slotId` 缺失或为空
- `slotId` 格式不符合 `slot001`
- `slotId` 重复
- Docker API 不可达
- 镜像缺失且自动拉取失败
- 容器创建失败
- 容器启动失败

## 日志字段

- `slotId`
- `action=create-slot`
- `containerName`
- `runtimeImage`
- `cdpPort`
- `vncPort`
- `error`

## 联调验收标准

- 创建一个全新 `slotId` 返回 `201`，并且 `status=waiting`
- 再调 `GET /api/v1/edge/slots/{slotId}` 能查到同一条记录
- 重复创建同一个 `slotId` 必须返回冲突
- Docker 不可达时必须明确失败，不能伪造成功

## 后续接入点

`******** 平台端是否允许创建 slot`

- 预留在 [service.go](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/Service/Slot/service.go) 的 `CreateSlot` 前置链路

`******** 平台端创建成功回告`

- 预留在 [service.go](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/Service/Slot/service.go) 的 `CreateSlot` 后置链路
