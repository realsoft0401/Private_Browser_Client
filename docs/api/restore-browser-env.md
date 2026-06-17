# POST /api/v1/edge/browser-envs/{envId}/restore

## 功能目标

把指定环境包从本机受控备份包恢复回正式环境目录，并恢复为可再次运行但未启动状态。

## 业务边界

- 只从本机 `backupPath` 恢复
- 成功后恢复正式环境目录
- 成功后删除本机 backup tar
- 成功后把索引恢复为待运行态
- 不接受上传文件
- 不自动 run
- 不替代 import-package

## 前置校验

- `envId` 存在
- 当前状态允许 restore
- `backupPath` 必须存在且位于受控目录下
- tar 结构合法
- 根目录必须等于 `envId`
- 身份一致性通过
- 没有正式目录冲突
- 没有 Docker 容器冲突

## 请求与响应

### 请求

```http
POST /api/v1/edge/browser-envs/906090001_tk_324867594169356288/restore
```

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-restore-001",
    "taskType": "browser_env_restore",
    "resourceType": "browser_env",
    "resourceId": "906090001_tk_324867594169356288",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-restore-001/events"
  }
}
```

## 状态流转

- restore 前：`backed_up`
- restore 成功后：`created`
- 成功后不自动 run
- `runtimeProtection/proxyRuntime` 恢复为 `pending`

## SSE 说明

- 本接口应使用 SSE
- 原因：解包、staging 校验、目录恢复、删除 backup tar、回写索引都是多阶段长链路

阶段建议：

- `queued`
- `load_env_index`
- `validate_status`
- `validate_backup_path`
- `check_backup_archive`
- `extract_to_staging`
- `validate_archive_structure`
- `load_profile`
- `validate_atomic_materials`
- `check_env_path_conflict`
- `check_container_conflict`
- `reassign_runtime_ports`
- `promote_staging`
- `remove_backup_archive`
- `update_index`
- `finalize_success`
- `finalize_failed`

SSE 中断后的收口规则：

- 不默认成功
- 必须确认正式环境目录恢复成功、backup tar 删除成功、索引已更新
- 如果环境目录已恢复但 backup tar 删除失败，任务必须 failed 并上报管理员

## 任务编排

- 本接口创建长链路 task
- 调用方通过 `taskId/eventsUrl` 观察解包、校验、恢复目录和删除 backup tar 过程

## 成功判定

- `backupPath` 合法且存在
- tar 结构合法
- 身份一致性通过
- 原子材料完整
- 正式目录恢复成功
- 本机 backup tar 删除成功
- SQLite 索引恢复为 `created`

## 失败判定

- `backupPath` 缺失或越界
- tar 结构错误
- 身份不一致
- 正式目录冲突
- Docker 容器冲突
- backup tar 删除失败
- 索引回写失败

## 日志字段

- `action=restore-browser-env`
- `taskId`
- `envId`
- `backupPath`
- `envPath`
- `stage`
- `containerName`
- `containerId`
- `error`

## 联调验收标准

- 返回 `taskId/eventsUrl`
- SSE 能看到完整恢复过程
- 成功后环境目录恢复到正式位置
- backup tar 被删除
- 状态回到 `created`
- 不自动 run
