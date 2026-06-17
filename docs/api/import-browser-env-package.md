# POST /api/v1/edge/browser-envs/import-package

## 功能目标

把一个外部标准环境包导入到当前 Client 本机，建立新的本机环境资产记录，但不自动运行。

## 业务边界

- 负责接收 tar 包
- 负责校验 tar 结构、身份与原子材料
- 负责重新分配当前节点的 `envSequence`、端口和运行摘要
- 负责建立 SQLite 索引
- 不自动 run
- 不自动拉镜像
- 不允许覆盖现有同 `envId` 环境
- 不允许自动改名或 clone

## 前置校验

- 请求只允许上传一个 tar 包
- tar 内只能有一个合法 `envId/` 根目录
- 根目录名必须符合 `userId_rpaType_snowflakeId`
- `profile.json`、`binding.json`、`proxy`、`fingerprint`、`browser-data/profile` 原子材料完整
- `container.json` 如果存在，只校验格式；如果缺失，允许按当前节点重建空运行摘要
- `profile.envId/userId/rpaType` 与 `binding.identity` 一致
- 当前节点 SQLite 中不存在同 `envId`

## 请求与响应

### 请求

```http
POST /api/v1/edge/browser-envs/import-package
Content-Type: multipart/form-data
```

表单字段：

- `file`

### 接单成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "taskId": "edge-task-import-001",
    "taskType": "browser_env_import_package",
    "resourceType": "browser_env",
    "eventsUrl": "/api/v1/edge/tasks/edge-task-import-001/events"
  }
}
```

## 状态流转

- 导入成功后状态为 `created`
- 不自动进入 `running`
- `runtimeProtection/proxyRuntime` 应恢复为 `pending`

必须保留：

- `envId`
- `userId`
- `rpaType`
- `identityHash`

必须重新分配：

- `envSequence`
- CDP/VNC 宿主端口
- `containerName`
- `containerId`
- `container_status`
- `monitor_status`
- `lastRuntime`

## SSE 说明

- 本接口应使用 SSE
- 原因：上传、解包、身份校验、端口重分配、索引落库都是多阶段长链路

阶段建议：

- `queued`
- `receive_upload`
- `extract_to_staging`
- `validate_archive_structure`
- `load_profile`
- `load_binding`
- `validate_identity`
- `validate_atomic_materials`
- `validate_checksums`
- `check_existing_env`
- `allocate_env_sequence`
- `allocate_runtime_ports`
- `check_container_conflicts`
- `rewrite_runtime_summaries`
- `promote_staging`
- `write_index`
- `cleanup_staging`
- `finalize_success`
- `finalize_failed`

SSE 中断后的收口规则：

- 不默认成功
- 必须确认正式目录已落盘且 SQLite 索引已成功建立

## 任务编排

- 本接口创建长链路 task
- 调用方通过 `taskId/eventsUrl` 观察上传、解包、身份校验、端口重分配和索引落库过程

## 成功判定

- tar 结构合法
- 只有一个合法 `envId` 根目录
- 身份一致性通过
- 原子材料完整
- checksum 校验通过
- 当前节点不存在相同 `envId`
- 本机运行摘要重分配成功
- Docker 无严重冲突
- 正式目录落盘成功
- SQLite 索引写入成功
- 状态为 `created`

## 失败判定

- 上传文件非法
- tar 结构错误
- 多环境目录
- 根目录名不合法
- 身份不一致
- 原子材料缺失
- checksum 校验失败
- 已存在同 `envId`
- 端口分配失败
- Docker 容器冲突
- 正式目录落盘失败
- SQLite 写入失败

## 日志字段

- `action=import-browser-env-package`
- `taskId`
- `envId`
- `userId`
- `rpaType`
- `envPath`
- `stage`
- `envSequence`
- `allocatedPorts`
- `containerName`
- `error`

## 联调验收标准

- 返回 `taskId/eventsUrl`
- SSE 能看到完整导入过程
- 正式目录落在 `data/browser-envs/users/{userId}/{rpaType}/{envId}`
- SQLite 索引成功建立
- 状态为 `created`
- 不自动 run
