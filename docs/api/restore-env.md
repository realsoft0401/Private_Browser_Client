# Edge API 设计：`POST /api/v1/edge/browser-envs/{envId}/restore`

## 1. 功能目标

`POST /api/v1/edge/browser-envs/{envId}/restore` 用于从本机备份包恢复环境包目录。

## 2. 业务边界

- 只从本机 `backupPath` 恢复
- 不自动 run
- 恢复后删除本机备份包
- 重新回到可运行目录状态

## 3. 请求与响应

```http
POST /api/v1/edge/browser-envs/{envId}/restore
```

同步返回结果，不创建 Edge task。

返回重点：

- `envId`
- `status=created`
- `restoredAt`

## 4. 前置校验

- 环境包必须处于 `backed_up`
- `backupPath` 必须存在
- checksum 必须匹配

## 5. 状态流转

- 恢复成功后进入 `created`
- 容器运行态重置
- 下一步需要显式 `run`

## 6. 成功判定

- 备份包校验通过
- 环境目录恢复成功
- 备份包清理成功
- SQLite backup 字段清空

## 7. 失败判定

- 不是备份状态
- 备份包缺失
- checksum 不匹配
- 恢复写盘失败

## 8. 日志字段

- `envId`
- `backupPath`
- `checksum`
- `error`

## 9. 联调验收标准

- 恢复后不自动运行
- 恢复后可以再次走 run
- checksum 失败时必须明确拒绝
