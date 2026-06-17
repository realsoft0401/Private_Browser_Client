# GET /api/v1/edge/slots

## 功能目标

列出当前 Client 已创建的全部 slot 当前态。

## 业务边界

- 只返回当前资源位事实
- 不返回历史审计
- 不返回 package 完整资产
- 不负责自动修复 slot

## 前置校验

- 无

## 状态流转

- 只读接口，不改任何状态

## 请求与响应

### 请求

```http
GET /api/v1/edge/slots
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": [
    {
      "slotId": "slot001",
      "status": "waiting",
      "containerName": "browser-slot-slot001",
      "runtimeImage": "private/browser-slot-runtime:latest",
      "cdpPort": 9222,
      "vncPort": 5901,
      "initializedAt": 1718500000,
      "updatedAt": 1718500000
    }
  ]
}
```

## SSE 说明

- 本接口不使用 SSE
- 原因：slot 列表查询是短链路只读接口，同步 HTTP 已足够表达结果

## 任务编排

当前接口不创建 task。

## 成功判定

- 返回当前全部 slot 快照

## 失败判定

- 内部存储读取失败

## 日志字段

- `action=list-slots`
- `slotCount`

## 联调验收标准

- 创建后的 slot 能立刻出现在列表里
- slot 占用中时要能看见 `currentPackageId/currentRunId`
