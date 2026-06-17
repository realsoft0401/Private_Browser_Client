# POST /api/v1/edge/packages/{packageId}/stop

## 功能目标

按兼容期旧协议，结束指定 `package` 当前运行关系并释放 `slot`。

## 业务边界

- 只围绕 `package_runtime_views / runtime_relations / slots` 收口
- 不替代正式 `browser-envs/{envId}/stop`

## 请求与响应

### 请求

```http
POST /api/v1/edge/packages/906090001_tk_324867594169356288/stop
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
    "currentRunId": null,
    "currentSlotId": null,
    "runtimeStatus": "stopped",
    "lastStopAt": 1718500010
  }
}
```

## SSE 说明

- 本接口不使用 SSE

## 兼容期说明

- 当前接口不再继续扩展新语义
- 正式 stop 能力统一应走 `browser-envs/{envId}/stop`
