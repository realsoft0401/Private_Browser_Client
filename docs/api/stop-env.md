# Edge API 设计：`POST /api/v1/edge/browser-envs/{envId}/stop`

## 1. 功能目标

`POST /api/v1/edge/browser-envs/{envId}/stop` 用于停止当前 Edge 节点本机浏览器环境包对应的容器。

## 2. 业务边界

- 停止容器
- 保留 `browser-data/profile`
- 不删除镜像
- 不删除环境包目录

## 3. 请求与响应

```http
POST /api/v1/edge/browser-envs/{envId}/stop
```

可选请求体：

- `timeoutSeconds`

成功响应先返回：

- `taskId`
- `taskType=browser_env_stop`
- `eventsUrl`

## 4. 前置校验

- `envId` 必须存在
- Docker 必须可达

## 5. 状态流转

- 创建 Edge task
- 停止本机容器
- 回写 `container.json/profile.lastRuntime/browser_envs`
- 最终进入 `stopped` 或保持受控非运行态

## 6. 成功判定

- 容器停止成功
- 索引运行态回写成功

## 7. 失败判定

- 环境包不存在
- Docker stop 失败
- 运行态回写失败

## 8. 日志字段

- `taskId`
- `envId`
- `timeoutSeconds`
- `containerId`
- `error`

## 9. 联调验收标准

- 成功后容器停掉，`browser-data/profile` 仍保留
- 停止后详情和列表状态同步正确
