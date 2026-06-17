# POST /api/v1/edge/browser-envs/{envId}/backup

## 功能目标

把指定环境包备份成标准归档包，并在备份成功后释放本机运行目录。

## 业务边界

- 负责生成标准 tar.gz 备份包
- 负责校验备份包可用性
- 负责删除源环境目录
- 负责删除关联的已停止容器
- 负责保留 SQLite 索引并将状态改为 `backed_up`
- 不负责下载备份给前端
- 不自动 restore
- 不自动 run
- 不删除镜像

## 环境包 backup 与容器 backup 的边界

- 容器是环境包的运行载体，容器里最终承载的是环境包加载后的运行结果。
- 但 `POST /api/v1/edge/browser-envs/{envId}/backup` 不是“容器现场快照”，而是正式的环境包资产级 backup。
- 这条接口归档的核心对象是：
  - `profile.json`
  - `binding.json`
  - `proxy/`
  - `fingerprint/`
  - `browser-data/profile`
  - `container.json`
  - 整个 `envPath`
- 容器相关处理在这里属于从属动作：用于校验当前现场、删除已停止容器、保证资产归档后运行目录释放，但容器本身不是资产真相源。
- 如果后续存在容器层导出、快照或诊断备份，它们也不能替代正式环境包 backup，更不能直接作为 `restore(envId)` 的标准数据源。

## 前置校验

- `envId` 存在
- 当前状态允许 backup
- 当前环境不在运行中
- `envPath` 必须在 `data/browser-envs` 受控目录内
- `envPath` 最后一层必须等于 `envId`
- 原子材料完整

## 请求与响应

### 请求

```http
POST /api/v1/edge/browser-envs/906090001_tk_324867594169356288/backup
```

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-backup-001",
    "taskType": "browser_env_backup",
    "resourceType": "browser_env",
    "resourceId": "906090001_tk_324867594169356288",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-backup-001/events"
  }
}
```

## 状态流转

- backup 前：`created` / `stopped` / `error`
- backup 成功后：`backed_up`
- 成功后环境目录被删除，后续不能直接 run，必须先 restore

## SSE 说明

- 本接口应使用 SSE
- 原因：涉及多阶段打包、校验、目录删除、索引回写，过程明显较长

阶段建议：

- `queued`
- `load_env_index`
- `validate_status`
- `validate_env_path`
- `load_profile`
- `validate_atomic_materials`
- `check_container_state`
- `prepare_backup_path`
- `create_archive`
- `validate_archive`
- `promote_archive`
- `remove_env_directory`
- `remove_stopped_container`
- `update_index`
- `finalize_success`
- `finalize_failed`

SSE 中断后的收口规则：

- 不能默认成功
- 必须确认备份包有效、源目录已删、索引已改为 `backed_up`
- 如果备份包已生成但源目录删除失败，任务必须失败并上报管理员人工处理

## 任务编排

- 本接口创建长链路 task
- 调用方通过 `taskId/eventsUrl` 观察打包、校验、删除源目录和索引回写过程

## 成功判定

- 备份包创建成功
- 备份包校验成功
- 正式备份包落位成功
- 源环境目录删除成功
- 已停止容器删除成功
- SQLite 索引更新为 `backed_up`

## 失败判定

- `envId` 不存在
- 当前仍在运行
- 原子材料缺失
- 备份包创建失败
- 备份包校验失败
- 源目录删除失败
- 容器删除失败
- 索引回写失败

## 日志字段

- `action=backup-browser-env`
- `taskId`
- `envId`
- `envPath`
- `backupPath`
- `stage`
- `archiveSize`
- `containerId`
- `containerName`
- `error`

## 联调验收标准

- 返回 `taskId/eventsUrl`
- SSE 能看到打包与删除源目录全过程
- 成功后状态为 `backed_up`
- 源目录删除后不能直接 run
- 备份包已生成但释放失败时，任务必须 failed 并上报管理员
