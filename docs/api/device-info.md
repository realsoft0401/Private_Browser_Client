# GET /api/v1/edge/device-info

## 功能目标

返回当前 Client 所在机器的最小设备事实摘要。

## 业务边界

- 只返回 Edge 本机事实
- 当前只返回 `os/deviceArch/dockerApiUrl/discoveryMode`
- 不返回中心 `clientId`
- 不返回用户、包状态、代理、登录态或敏感资产内容

## 前置校验

- 无

## 状态流转

- 只读接口，不修改任何本机状态

## 请求与响应

### 请求

```http
GET /api/v1/edge/device-info
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "os": "linux",
    "deviceArch": "arm64",
    "dockerApiUrl": "http://127.0.0.1:2375",
    "discoveryMode": "independent-intranet"
  }
}
```

## SSE 说明

- 本接口不使用 SSE
- 原因：设备信息查询是短链路只读接口，同步 HTTP 已足够表达结果

## 任务编排

当前接口不创建 task。

## 成功判定

- 能返回当前宿主机最小设备摘要

## 失败判定

- 当前实现基本不会主动失败
- 未来如果 Docker 探测扩进来，再按依赖失败返回

## 日志字段

- `action=get-device-info`
- `os`
- `deviceArch`
- `dockerApiUrl`

## 联调验收标准

- Docker API 若配置为回环地址或泛监听地址，返回值应被改写成 Node 可理解的 `clientIp:2375`
- 响应里不能出现 `clientId`
