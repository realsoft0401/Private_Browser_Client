# Edge API 设计：`POST /api/v1/edge/browser-envs/{envId}/revalidate`

## 1. 功能目标

`POST /api/v1/edge/browser-envs/{envId}/revalidate` 用于对 `status=error` 的本机环境包执行受控重新校验。

## 2. 业务边界

- 只做重新准入校验
- 不启动容器
- 不拉镜像
- 不证明代理出口最终可用
- 不恢复登录态资产内容

## 3. 请求与响应

```http
POST /api/v1/edge/browser-envs/{envId}/revalidate
```

成功响应先返回：

- `taskId`
- `taskType=browser_env_revalidate`
- `eventsUrl`

## 4. 前置校验

- 只允许 `status=error`
- 原子材料必须存在
- Docker 不能有身份冲突
- 本机端口必须可重新确认

## 5. 状态流转

- 创建 Edge task
- 校验 profile/binding/proxy/fingerprint/browser-data/profile
- 重置 `runtimeProtection/proxyRuntime` 为待重新验证
- 成功后回到 `created` 或 `stopped` 等可控状态

## 6. 成功判定

- 原子材料完整
- Docker 身份冲突已排除
- 索引状态恢复到可再次 run 的状态

## 7. 失败判定

- 不是 `error`
- 原子材料不完整
- Docker 容器仍在运行
- 容器身份冲突

## 8. 日志字段

- `taskId`
- `envId`
- `stage`
- `error`

## 9. 联调验收标准

- 只有 `error` 环境可调用
- 成功后不会自动 run
- 失败原因能明确定位到材料缺失或 Docker 冲突
