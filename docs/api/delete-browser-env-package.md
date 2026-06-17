# DELETE /api/v1/edge/browser-envs/{envId}/package

## 功能目标

彻底删除当前节点上的环境包资产、登录态目录、已停止容器和 SQLite 索引。

这条接口的真正目标不是“停掉并删除”，而是：

- 在确认环境已经不再运行后
- 删除正式环境目录
- 删除 `browser-data/profile`
- 删除已停止容器
- 删除本机 backup 资产
- 删除 SQLite 索引

## 业务边界

- 负责彻底销毁当前节点上的环境包资产
- 负责删除本机 backup 包
- 负责删除已停止容器
- 负责删除 SQLite 索引
- 不删除 Docker 镜像
- 不自动 stop
- 不自动 backup
- 不自动 restore

关键边界：

- `delete package` 不是 `backup`
- `delete package` 也不是 `stop`
- 如果仍在运行，必须先 stop，再 delete

## 前置校验

- `envId` 存在
- 当前环境不是 `running`
- 没有并发 run / stop / backup / restore / revalidate / delete
- `envPath` 位于受控目录内

补充规则：

- `running` 状态一律拒绝 delete
- 当前接口不允许在 delete 内部偷偷 stop
- 如果目标环境已经 `deleted` 或索引已不存在，后续是否做幂等兼容可以单独扩展；当前正式文档先按“资源不存在”处理

## 请求与响应

### 请求

```http
DELETE /api/v1/edge/browser-envs/906090001_tk_324867594169356288/package
```

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-delete-001",
    "taskType": "browser_env_delete_package",
    "resourceType": "browser_env",
    "resourceId": "906090001_tk_324867594169356288",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-delete-001/events"
  }
}
```

## 状态流转

- delete 前：`created` / `stopped` / `backed_up` / `error`
- delete 成功后：`deleted`
- 成功后正式环境目录不存在
- 成功后本机 backup 包不存在
- 成功后 SQLite 索引删除或等价软删收口完成

## SSE 说明

- 本接口建议使用 SSE
- 原因：会涉及多阶段校验、目录删除、backup 删除、容器删除和索引清理，过程不适合只靠一次同步返回

阶段建议：

- `queued`
- `load_env_index`
- `validate_status`
- `validate_env_path`
- `check_container_state`
- `remove_stopped_container`
- `remove_backup_archive`
- `remove_env_directory`
- `remove_index`
- `finalize_success`
- `finalize_failed`

## 任务编排

- 本接口创建长链路 task
- 调用方通过 `taskId/eventsUrl` 观察容器删除、backup 删除、目录删除和索引清理过程

## 成功判定

- 当前状态允许 delete
- 已停止容器删除成功或确认不存在
- backup 包删除成功或确认不存在
- 正式环境目录删除成功或确认不存在
- SQLite 索引删除或等价 `deleted` 收口成功

## 失败判定

- `envId` 不存在
- 当前仍在 `running`
- 发生命周期冲突
- 容器仍在运行
- backup 包删除失败
- 正式目录删除失败
- 索引删除失败

## 日志字段

- `action=delete-browser-env-package`
- `taskId`
- `envId`
- `envPath`
- `backupPath`
- `containerId`
- `containerName`
- `stage`
- `error`
- `suggestion`

## 联调验收标准

- 返回 `taskId/eventsUrl`
- `running` 状态直接 delete 时，必须返回 `1003`
- SSE 能看到删除过程
- 成功后正式目录不存在
- 成功后 backup 包不存在
- 成功后索引不再可用于普通生命周期动作
