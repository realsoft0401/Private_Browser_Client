# Edge API 设计：`GET /api/v1/edge/browser-envs/{envId}/vnc-info`

## 1. 功能目标

`GET /api/v1/edge/browser-envs/{envId}/vnc-info` 用于返回当前本机环境包的 VNC 连接信息。

## 2. 业务边界

- 返回的是当前 Edge 节点可访问地址
- 不是公网地址
- 不做中心网关包装
- `webVncUrl` 应跟随当前宿主机地址，不应写死 `127.0.0.1`

## 3. 请求与响应

```http
GET /api/v1/edge/browser-envs/{envId}/vnc-info
```

返回重点：

- `envId`
- `vncPort`
- `vncUrl`
- `wsUrl`
- `webVncUrl`

## 4. 前置校验

- 环境包必须存在
- 需要有可用 VNC 端口信息

## 5. 状态流转

- 只读，不改状态

## 6. 成功判定

- 返回的地址与当前 Edge 节点宿主机地址一致
- `webVncUrl` 可直接用于内网页面访问

## 7. 失败判定

- 环境包不存在
- VNC 信息缺失

## 8. 日志字段

- `envId`
- `vncPort`
- `wsUrl`
- `webVncUrl`
- `error`

## 9. 联调验收标准

- 返回值不出现错误的 `127.0.0.1` 示例
- `webVncUrl` 与实际 Swagger 展示口径一致
