# POST /api/v1/edge/packages/{packageId}/run

## 功能目标

按兼容期旧协议，把指定 `package` 放进指定 `slot` 运行。

## 业务边界

- 只围绕 `package_runtime_views / runtime_relations / slots` 工作
- 不承诺完整正式环境包资产校验
- 不替代正式 `browser-envs/{envId}/run`

## 请求与响应

### 请求

```http
POST /api/v1/edge/packages/906090001_tk_324867594169356288/run
Content-Type: application/json
```

```json
{
  "slotId": "slot001"
}
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
    "lastRunAt": 1718500000
  }
}
```

## SSE 说明

- 本接口不使用 SSE

## 兼容期说明

- 当前接口只保留给旧链路和本机调试
- 正式 run 能力统一以 `browser-envs/{envId}/run` 为准
