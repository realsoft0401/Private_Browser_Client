# Edge API 设计：`GET /api/v1/edge/device-info`

## 1. 功能目标

`GET /api/v1/edge/device-info` 用于返回当前宿主机和本机 Docker 能力摘要。

它主要服务于：

- `Private_Browser_Server` verify 前探测
- 管理员确认设备架构和 Docker 版本
- 调试镜像策略是否能匹配当前节点

## 2. 业务边界

- 只读本机 Docker `/_ping`、`/info`、`/version`
- 不写 SQLite
- 不生成中心身份
- 不返回环境包列表或用户归属

## 3. 请求与响应

```http
GET /api/v1/edge/device-info
```

返回重点：

- `deviceIp`
- `dockerApiUrl`
- `deviceOs`
- `deviceArch`
- `deviceRawArch`
- `cpuCores`
- `memoryTotalBytes`
- `dockerVersion`
- `lastDockerStatus`

## 4. 前置校验

- Docker API 配置必须存在
- Docker daemon 必须可达

## 5. 状态流转

- 本接口不改变环境包状态
- 不改变任务状态
- 不改变任何中心状态

## 6. 成功判定

- 成功读到 Docker 基本信息
- `deviceArch` 能返回当前节点架构事实

## 7. 失败判定

- Docker API 不可达
- Docker 返回异常
- 设备信息解析失败

## 8. 日志字段

- `dockerApiUrl`
- `deviceArch`
- `dockerVersion`
- `error`

## 9. 联调验收标准

- AMD64/ARM64 节点能返回正确架构
- Docker 故障时能给出明确错误，不静默成功
- 返回里不出现中心 `clientId`
