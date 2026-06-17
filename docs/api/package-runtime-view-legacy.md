# GET /api/v1/edge/packages/{packageId}/runtime-view

## 功能目标

返回兼容期 `package` 当前运行视图。

## 业务边界

- 只读取本机 SQLite `package_runtime_views`
- 不读取正式环境包目录
- 不替代 `browser-envs/*` 正式生命周期接口

## 请求与响应

### 请求

```http
GET /api/v1/edge/packages/906090001_tk_324867594169356288/runtime-view
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "packageId": "906090001_tk_324867594169356288",
    "currentRunId": "906090001_tk_324867594169356288-1718500000",
    "currentSlotId": "slot001",
    "runtimeStatus": "running",
    "lastRunAt": 1718500000,
    "lastStopAt": null,
    "lastError": null
  }
}
```

## SSE 说明

- 本接口不使用 SSE

## 兼容期说明

- 当前接口属于兼容层
- 用户主状态应看 `browser-envs.status`
