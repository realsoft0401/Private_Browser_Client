# Edge API 设计：`POST /api/v1/edge/browser-envs/{envId}/run`

## 1. 功能目标

`POST /api/v1/edge/browser-envs/{envId}/run` 用于启动当前 Edge 节点本机的浏览器环境包。

## 2. 业务边界

- 镜像、端口、代理、挂载都来自环境包资产
- 不接受任意 Docker 参数透传
- 不负责中心任务持久化
- 不能绕过原子材料校验

## 3. 请求与响应

```http
POST /api/v1/edge/browser-envs/{envId}/run
```

可选请求体：

- `forceRecreate`

成功响应先返回：

- `taskId`
- `taskType=browser_env_run`
- `resourceType=browser_env`
- `eventsUrl`

## 4. 前置校验

- `envId` 必须存在
- 环境包原子材料必须完整
- Docker 必须可达
- `runtime.image` 必须可用
- `backed_up/deleted` 等不允许直接 run

## 5. 状态流转

- 创建 Edge task
- 创建或启动容器
- 等待浏览器/CDP
- 执行 timezone 与网络指纹相关验证
- 最终进入 `running` 或 `error`

## 6. 任务编排

```text
queued
  -> docker create/start
  -> wait browser
  -> cdp check
  -> timezone probe
  -> done or error
```

## 7. 成功判定

- 容器启动成功
- 浏览器/CDP 可达
- 运行态验证通过
- 环境包主状态进入 `running`

## 8. 失败判定

- 原子材料缺失
- Docker 不可达
- 镜像不存在
- CDP 不可达
- timezone 或运行保护验证失败

## 9. 日志字段

- `taskId`
- `envId`
- `image`
- `stage`
- `containerName`
- `error`

## 10. 联调验收标准

- SSE 能看到完整启动阶段
- 成功后详情接口返回 `running`
- 失败时不能静默保留旧可用结论
