# GET /health

## 功能目标

返回当前 `Private_Browser_Client` 的本机健康摘要。

## 业务边界

- 只表达 Client 本机视角的 `healthy/unhealthy`
- 只返回本机服务、SQLite、Swagger、Node 协同配置的检查结果
- 不表达 Server 侧 `offline/stale`
- 不表达中心是否已经完成 bind
- 不因为还没有 `clientId` 就判失败

## 前置校验

- 无

## 状态流转

- 只读接口，不修改任何本机状态

## 请求与响应

### 请求

```http
GET /health
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "ok": true,
    "status": "healthy",
    "service": "Private_Browser_Client",
    "mode": "docker",
    "version": "0.2.0",
    "configFile": "Settings/config-docker.yaml",
    "dockerApi": "http://127.0.0.1:2375",
    "checkedAt": 1718500000,
    "checks": {
      "api": {
        "status": "healthy",
        "message": "http 服务可响应"
      },
      "sqlite": {
        "status": "healthy",
        "message": "sqlite 已初始化"
      },
      "swagger": {
        "status": "healthy",
        "message": "swagger/openapi 入口已挂载"
      },
      "nodeRegistration": {
        "status": "healthy",
        "message": "Node 登记协同配置已就绪，可查询中心状态并接收 assign"
      }
    }
  }
}
```

## 任务编排

当前接口不创建 task。

## SSE 说明

- 本接口不使用 SSE
- 原因：健康检查是短链路只读接口，同步 HTTP 已足够表达结果

## 成功判定

- HTTP 服务能正常返回健康摘要

## 失败判定

- 当前实现原则上总是返回成功包装
- 真正异常会体现在 `checks` 子项 message 中

## 日志字段

- `action=health-check`
- `checkedAt`
- `sqliteStatus`
- `nodeRegistrationStatus`

## 联调验收标准

- 无论是否已绑定 Node，接口都能返回健康摘要
- 未配置 node_register 时，只能影响 `checks.nodeRegistration`，不能把整个接口打成失败
