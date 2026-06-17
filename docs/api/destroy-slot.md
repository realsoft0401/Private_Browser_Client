# DELETE /api/v1/edge/slots/{slotId}

## 功能目标

销毁指定 slot。

## 业务边界

- 负责删除本机 slot 当前态记录
- 负责删除 slot runtime 容器
- 当前骨架阶段默认只允许销毁 `waiting` 状态的 slot
- `force` 只保留协议位，当前不扩展完整强制结束运行关系链路
- 不负责自动 stop package

## 前置校验

- `slotId` 必须存在
- 默认要求 slot 当前是 `waiting`
- 若请求体带 `force=true`，当前只绕过 waiting 校验，不等于已经做完完整业务强删链路

## 状态流转

成功后：

- 删除 slot runtime 容器
- 删除 slot 当前态记录

失败后：

- 保留原 slot 记录

## 请求与响应

### 请求

```http
DELETE /api/v1/edge/slots/slot001
Content-Type: application/json
```

```json
{
  "force": false
}
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "slotId": "slot001",
    "status": "deleted"
  }
}
```

### 失败响应

```json
{
  "code": 1003,
  "message": "slot 当前不是 waiting，不能直接销毁"
}
```

## SSE 说明

- 本接口当前不使用 SSE
- 原因：slot 销毁按当前设计是同步动作，调用方需要立即知道是否已删除

## 任务编排

当前接口是同步接口，不创建 task。

## 成功判定

- slot 存在
- slot runtime 销毁成功
- slot 当前态记录删除成功

## 失败判定

- slot 不存在
- slot 非 waiting 且未允许 force
- Docker 容器删除失败

## 日志字段

- `action=destroy-slot`
- `slotId`
- `force`
- `containerId`
- `error`

## 联调验收标准

- waiting 态 slot 可以直接销毁
- 非 waiting 态 slot 默认必须拒绝
- 销毁后 `GET /slots/{slotId}` 应返回不存在
