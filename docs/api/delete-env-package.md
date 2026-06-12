# Edge API 设计：`DELETE /api/v1/edge/browser-envs/{envId}/package`

## 1. 功能目标

`DELETE /api/v1/edge/browser-envs/{envId}/package` 用于彻底删除本机浏览器环境包资产。

## 2. 业务边界

- 删除环境包目录
- 删除 `browser-data/profile`
- 删除已停止容器
- 删除 SQLite 索引
- 不删除浏览器镜像

## 3. 请求与响应

```http
DELETE /api/v1/edge/browser-envs/{envId}/package
```

成功响应先返回：

- `taskId`
- `taskType=browser_env_delete`
- `eventsUrl`

## 4. 前置校验

- `envId` 必须存在
- 环境包不能处于 `running`

## 5. 状态流转

- 创建 Edge task
- 后台删除已停止容器、环境目录和 SQLite 索引
- 成功后本机不再保留该环境包资产

## 6. 成功判定

- 目录物理删除成功
- 索引删除成功
- 任务进入 `done`

## 7. 失败判定

- 环境包仍在运行
- 容器删除失败
- 目录删除失败
- SQLite 删除失败

## 8. 日志字段

- `taskId`
- `envId`
- `stage`
- `error`

## 9. 联调验收标准

- 成功后 detail 和 list 都不再返回该 env
- 不会连带删除镜像
- running 环境必须拒绝删除
