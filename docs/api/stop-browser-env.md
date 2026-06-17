# POST /api/v1/edge/browser-envs/{envId}/stop

## 功能目标

停止指定环境包在当前 Client 本机上的运行容器，并把环境包运行态同步收口到停止后事实。

这条接口的真正目标不是“删东西”，而是：

- 按环境包安全停止本机浏览器容器
- 保留环境包目录和 `browser-data/profile`
- 回写最近一次停止后的运行摘要
- 让环境包主状态从运行态收口到非运行态

## 业务边界

- 负责按环境包停止容器并同步运行态
- 负责回写 `container.json` 和 SQLite 运行态摘要
- 不删除环境包目录
- 不删除 `browser-data/profile`
- 不删除镜像
- 不修改 `identityHash`
- 不重新校验 `profile.json` / `proxy` / `fingerprint` 原子材料

关键边界：

- `stop` 只围绕“当前运行态事实”收口
- 它不应因为代理文件、指纹文件或 profile 配置问题而阻止停止动作

## 身份与读取口径

- `envId` 格式固定为 `userId_rpaType_snowflakeId`
- 读取入口统一先查 SQLite 索引
- `stop` 只读取 SQLite 索引和 `container.json`
- 不读取 `profile.json` / `binding.json` / `proxy` / `fingerprint`

这样设计的原因：

- stop 的目标是安全收口运行态
- 不是重新做一次完整资产校验
- 不能因为环境包配置层局部异常，反而导致连停止容器都做不到

## 前置校验

- `envId` 存在于本机索引
- 环境包不是 `deleted`
- Docker 可达
- 同一 `envId` 当前没有并发 stop

补充规则：

- 如果当前已经是 `stopped` / `created` / `backed_up` 这类非运行态，也统一返回成功，不能报内部错误
- 如果 SQLite 里有 `containerId` 或 `containerName`，但 Docker 查不到对应容器，应先按受控缺失容器收口为 `stopped`

## 请求与响应

### 请求

```http
POST /api/v1/edge/browser-envs/906090001_tk_324867594169356288/stop
Content-Type: application/json
```

```json
{
  "timeoutSeconds": 10
}
```

正式请求体只收这一个字段：

- `timeoutSeconds`
  - 选填，默认 `10`
  - 表示调用 Docker stop 的等待秒数
  - 超时后如何升级到更强制动作，后续如果有需要再单独扩展，不在当前文档默认承诺

明确不允许的请求体扩展：

- 不接受 `slotId`
- 不接受 `force`
- 不接受 `image`
- 不接受 `clientId`
- 不接受任意 Docker 参数透传

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "envId": "906090001_tk_324867594169356288",
    "status": "stopped",
    "containerStatus": "missing",
    "stoppedAt": 1718501000
  }
}
```

## 状态流转

- 环境包主状态：`running` / `error` -> `stopped`
- 容器摘要状态：`running` -> `exited` / `missing`
- slot 资源位：如果该环境包当前占用 slot，则由运行关系释放链路把 slot 收口到 `releasing -> waiting`

关键原则：

- 停止容器成功，不代表环境包资产被删除
- stop 后只是回到非运行态，环境包本身仍保留
- stop 不回写 `profile.json` 运行态字段
- stop 成功判定不要求 slot 已经同步释放完成；slot 可以继续按自己的链路异步收口

## 执行链路

后台主链路建议：

1. `load_env_index`
2. `load_container_summary`
3. `check_docker`
4. `resolve_container`
5. `stop_container`
6. `sync_container_summary`
7. `sync_env_runtime_state`
8. `release_slot_relation`
9. `finalize_success` / `finalize_failed`

## SSE 说明

- 本接口当前不建议使用 SSE
- 原因：`stop` 是短链路、单动作收口接口，普通 HTTP 已足够表达成功或失败
- 如果后续 stop 被扩展成明显多阶段、长耗时、需要持续观察的后台任务，再单独升级为 SSE/task 模式

## 任务编排

- 当前接口不创建独立 task
- 停止动作和运行态回写在一次同步请求里完成
## 成功判定

- `envId` 存在
- Docker stop 成功，或容器已不存在但已按受控规则收口
- `container.json` 运行摘要回写成功
- SQLite 运行态摘要回写成功
- 环境包主状态进入 `stopped` 或等价非运行态

## 失败判定

- `envId` 不存在
- 环境包已经 `deleted`
- Docker 不可达
- Docker stop 返回明确失败
- 运行态摘要回写失败
- stop 过程中发现同一环境包存在并发生命周期冲突

## 运行态回写口径

成功停止后，必须至少同步下面两处：

1. `container.json`
   - `status`
   - `stoppedAt`
   - `updatedAt`
2. SQLite 运行态摘要
   - `browser_envs.status`
   - `browser_envs.container_status`
   - `browser_envs.last_stopped_at`
   - `browser_envs.last_checked_at`
   - 必要时同步 `last_error`

明确不回写：

- `profile.json` 的运行态字段
- `identityHash`
- `proxyRuntime`
- `runtimeProtection`

## 日志字段

- `action=stop-browser-env`
- `envId`
- `timeoutSeconds`
- `containerName`
- `containerId`
- `dockerStopResult`
- `status`
- `error`
- `suggestion`

## 联调验收标准

- 调用成功后，容器应停止或被受控归一为已停止
- `browser-data/profile` 仍然保留
- 环境包目录仍然保留
- `container.json` 与 SQLite 运行态摘要同步正确
- 已经停止的环境再次 stop，不能返回内部错误
- Docker 查不到旧容器时，能按受控规则收口，不把接口做成假失败
