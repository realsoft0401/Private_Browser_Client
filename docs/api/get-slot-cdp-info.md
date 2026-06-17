# GET /api/v1/edge/slots/{slotId}/cdp-info

## 功能目标

返回指定 slot 的 CDP 连接信息。

## 业务边界

- 只返回连接入口
- 当前不做 CDP 可用性测试
- 不读取 package 完整运行资产

## 前置校验

- `slotId` 必须存在
- slot 必须已经初始化出 `cdpPort`

## 状态流转

- 只读接口，不修改状态

## 请求与响应

### 请求

```http
GET /api/v1/edge/slots/slot001/cdp-info
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "slotId": "slot001",
    "cdpPort": 9222,
    "httpUrl": "http://192.168.10.220:9222",
    "versionUrl": "http://192.168.10.220:9222/json/version",
    "wsBaseUrl": "ws://192.168.10.220:9222"
  }
}
```

## SSE 说明

- 本接口不使用 SSE
- 原因：CDP 连接信息查询是短链路只读接口，同步 HTTP 已足够表达结果

## 任务编排

当前接口不创建 task。

## 成功判定

- 能返回 slot 对应 CDP 入口

## 失败判定

- slot 不存在
- slot 未初始化 CDP 端口

## 日志字段

- `action=get-slot-cdp-info`
- `slotId`
- `cdpPort`

## 联调验收标准

- `versionUrl` 可直接用于后续 CDP 可用性检测
