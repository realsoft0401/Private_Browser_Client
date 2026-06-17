# GET /api/v1/edge/slots/{slotId}

## 功能目标

查询单个 slot 当前态。

## 业务边界

- 只返回单个 slot 当前资源位事实
- 不负责返回历史运行列表
- 不负责自动初始化缺失 slot

## 前置校验

- `slotId` 必须存在

## 状态流转

- 只读接口，不改状态

## 请求与响应

### 请求

```http
GET /api/v1/edge/slots/slot001
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "slotId": "slot001",
    "status": "occupied",
    "currentPackageId": "906090001_tk_324867594169356288",
    "currentRunId": "906090001_tk_324867594169356288-1718500000123456789",
    "containerName": "browser-slot-slot001",
    "containerStatus": "running",
    "cdpPort": 9222,
    "vncPort": 5901,
    "initializedAt": 1718500000,
    "updatedAt": 1718500030
  }
}
```

### 失败响应

```json
{
  "code": 1002,
  "message": "数据不存在"
}
```

## SSE 说明

- 本接口不使用 SSE
- 原因：单个 slot 查询是短链路只读接口，同步 HTTP 已足够表达结果

## 任务编排

当前接口不创建 task。

## 成功判定

- 返回指定 slot 当前态

## 失败判定

- 指定 slot 不存在

## 日志字段

- `action=get-slot`
- `slotId`

## 联调验收标准

- slot 不存在必须返回 `1002`
- 占用中的 slot 能看见当前 package/run 关系
