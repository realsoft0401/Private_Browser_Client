# Browser Env E2E 导入回归方案

## 1. 目标

这份文档测试的是：

```text
已有标准 tgz 包
-> create slot
-> import-package
-> run
-> stop
-> backup
-> restore
-> delete package
-> destroy slot
```

这条路线不是测试“新建环境包”，而是测试“把一个现成标准包重新接入到当前 Client”。

本方案重点验证：

- 外部 `tgz` 是否能被正式 `import-package` 接受
- 导入后 SQLite 索引是否建立正确
- 导入后是否能继续走 `run / stop / backup / restore / delete`
- `slot` 是否遵守最新规则
  - `run` 时临时挂载包
  - `stop / ending / backup` 后恢复为空白 `waiting` 容器

## 2. 本轮测试使用的包

本轮示例使用：

```bash
export IMPORT_ARCHIVE="/Users/lining/Documents/Browser_virtualization/318275706305908736_tk_319725200528642048-backup-1781688927.tar.gz"
```

说明：

- 这是一个标准备份包，不是随手 tar 的裸目录
- 如果你换别的包，后面所有 `ENV_ID`、`userId`、`rpaType` 预期也要跟着换

## 3. 导入包最低要求

导入前必须先确认这个 `tgz` 是标准包，而不是随便打出来的目录压缩包。

至少要满足：

- tar 内只有一个根目录
- 根目录名符合 `userId_rpaType_snowflakeId`
- 根目录下至少有：
  - `profile.json`
  - `binding.json`
  - `proxy/`
  - `fingerprint/`
  - `browser-data/profile`
- `container.json` 可以有，也可以没有
- `profile.json` 里的 `envId/userId/rpaType` 与 `binding.json` 身份字段一致

## 4. 测试变量

先统一导出变量，后面所有命令都直接复用：

```bash
export CLIENT_BASE="http://127.0.0.1:3300"
export CLIENT_DB="/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/data/private_browser_client.db"
export PROJECT_ROOT="/Users/lining/Documents/Browser_virtualization/Private_Browser_Client"

export TEST_SLOT_ID="slot001"
export IMPORT_ARCHIVE="/Users/lining/Documents/Browser_virtualization/318275706305908736_tk_319725200528642048-backup-1781688927.tar.gz"
```

## 5. 测试前检查

### 5.1 检查 Client 是否健康

目的：

- 确认 3300 服务已启动
- 避免后面所有报错其实只是 Client 没起来

命令：

```bash
curl -s "$CLIENT_BASE/health" | jq
```

通过标准：

- 返回 `code=1000`
- `data.ok=true`
- `data.status=healthy`

### 5.2 检查导入包文件是否存在

目的：

- 确认本地路径没写错
- 确认手上真的是要测的那个包

命令：

```bash
ls -lh "$IMPORT_ARCHIVE"
```

通过标准：

- 文件存在
- 大小正常，不是 0B 或异常小文件

### 5.3 检查压缩包结构

目的：

- 在真正导入前，先肉眼看一遍 tar 结构
- 避免把结构错误的包直接推到 API 再猜原因

命令：

```bash
tar -tzf "$IMPORT_ARCHIVE" | sed -n '1,80p'
```

通过标准：

- 能正常列出内容
- 顶层只有一个根目录
- 能看到 `profile.json`、`binding.json`、`browser-data/profile` 等关键文件

## 6. 测试前清理口径

这一步非常重要。

导入测试最容易失败在“历史数据没清干净”，不是代码问题，而是现场不干净。

你至少要确认两件事：

- `slot001` 可以被本轮测试独占使用
- 同一个 `envId` 没有以 `created/running/stopped/backed_up` 之类状态残留在当前 Client

### 6.1 检查 slot 当前状态

命令：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
```

说明：

- 如果返回不存在，说明还没创建 slot，后面直接走 create
- 如果已存在，理想状态应是 `waiting`
- 如果是 `occupied`，先不要导入，说明这个 slot 正在被别的环境使用

### 6.2 检查同 envId 是否已在当前 Client 落库

命令：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,backup_path,last_error,updated_at
FROM browser_envs
WHERE env_id='318275706305908736_tk_319725200528642048';
"
```

说明：

- 如果查不到，说明这是“全新导入”场景
- 如果已经存在：
  - `backed_up`：这更像 restore 场景，不是 import 场景
  - `created/stopped/running`：说明当前 Client 已经有这条资产，再导入会撞重复记录

建议：

- 本文档默认测试“真正 import”
- 所以如果当前库里已经有同一个 `envId`，建议先清理旧资产，再开始本轮

## 7. Step 1: create slot

目的：

- 为本轮导入准备一个可运行资源位
- 后续 `run` 会明确指定这个 `slot001`

先说明清楚：

- 这一步不是“无论如何都必须重新创建”
- 如果 `slot001` 已经存在，并且当前是可复用的 `waiting` 状态，就应该直接跳过这一步，继续后面的 `import-package`
- 不要因为文档里先写了 create，就误以为“看到已存在 slot 也一定要强行重建”

命令：

```bash
curl -i -s -X POST "$CLIENT_BASE/api/v1/edge/slots" \
  -H "Content-Type: application/json" \
  -d "{
    \"slotId\": \"$TEST_SLOT_ID\"
  }"
```

通过标准：

- 首次创建通常返回成功
- `status=waiting`
- `containerName`、`cdpPort`、`vncPort` 都已生成

补充检查：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
```

重点看：

- `status=waiting`
- `currentPackageId` 为空
- `currentRunId` 为空

如果已经存在怎么办：

- 如果 `GET /slots/slot001` 已经返回一条 `status=waiting` 的 slot：
  说明资源位已经准备好了
  本轮测试应直接跳过 `create slot`，继续走 `import-package`
- 如果 `status=occupied`：
  说明这个 slot 正在被别的环境占用
  不能继续本轮测试，应先 stop 或换一个新的 slotId
- 如果是残留脏数据，例如数据库里有 slot，但容器事实异常：
  先做 slot 收口或销毁重建，再继续测试

如果失败：

- `数据状态冲突` 通常表示这个 `slotId` 已经存在
- 先查当前 slot 状态
- 如果它已经是 `waiting`，就直接复用，不要把“已存在 waiting slot”误判成失败
- 只有你明确要做“从 0 新建 slot”测试时，才需要先销毁后重建

## 8. Step 2: import-package

目的：

- 把外部 `tgz` 正式导入成当前 Client 本机的环境包资产

命令：

```bash
IMPORT_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/import-package" \
  -F "file=@$IMPORT_ARCHIVE")"

printf '%s\n' "$IMPORT_RESP" | jq
export IMPORT_TASK_ID="$(printf '%s' "$IMPORT_RESP" | jq -r '.data.taskId')"
echo "$IMPORT_TASK_ID"
```

你要看什么：

- 同步响应只代表“任务已接收”
- 真正成功与否要看 SSE

SSE 命令：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$IMPORT_TASK_ID/events"
```

理想通过标准：

- 最终出现 `event: task.completed`
- 最后一条是 `finalize_success`

本轮重点关注阶段：

- `extract_to_staging`
  说明上传包已解压到临时目录
- `prepare_import_package`
  说明开始准备导入为正式环境资产
- `finalize_success`
  说明正式索引和目录落地成功

常见失败含义：

- `validate_archive_structure_failed`
  包结构不合规，例如多根目录、缺根目录
- `load_profile_failed`
  `profile.json` 缺失或无法解析
- `create_index_failed`
  常见是同 `envId` 已存在

## 9. Step 3: 取出 envId 和 envPath

当前实现里，`import-package` 的同步接单响应不直接带 `envId`，所以需要手工取一次。

### 9.1 从 SQLite 取最新 envId

命令：

```bash
export ENV_ID="$(sqlite3 "$CLIENT_DB" "SELECT env_id FROM browser_envs ORDER BY updated_at DESC LIMIT 1;")"
echo "$ENV_ID"
```

### 9.2 取出导入后的 envPath

命令：

```bash
export ENV_PATH_REL="$(sqlite3 "$CLIENT_DB" "SELECT env_path FROM browser_envs WHERE env_id='$ENV_ID';")"
export ENV_PATH_ABS="$PROJECT_ROOT/$ENV_PATH_REL"
echo "$ENV_PATH_ABS"
```

说明：

- 后面所有目录检查都基于这个 `ENV_PATH_ABS`

## 10. Step 4: 导入后检查

目的：

- 确认导入动作真的已经落到正式资产目录和 SQLite

命令：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,user_id,rpa_type,status,env_sequence,cdp_port,vnc_port,env_path,last_error
FROM browser_envs
WHERE env_id='$ENV_ID';
"
```

```bash
find "$ENV_PATH_ABS" -maxdepth 3 | sort | sed -n '1,80p'
```

通过标准：

- `status=created`
- `env_sequence` 已重新分配
- `cdp_port/vnc_port` 已按当前服务器重分配
- 正式目录已落在：
  `data/browser-envs/users/{userId}/{rpaType}/{envId}`

重点说明：

- `import-package` 成功后，只表示资产导入完成
- 此时还没有运行
- `slot001` 仍应保持空白 `waiting`

可顺手检查：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
```

预期：

- `status=waiting`
- 不应该因为 import 就自动占用 slot

## 11. Step 5: run

目的：

- 把刚导入的环境显式挂到 `slot001` 上运行

命令：

```bash
RUN_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/run" \
  -H "Content-Type: application/json" \
  -d "{
    \"slotId\": \"$TEST_SLOT_ID\",
    \"forceRecreate\": false
  }")"

printf '%s\n' "$RUN_RESP" | jq
export RUN_TASK_ID="$(printf '%s' "$RUN_RESP" | jq -r '.data.taskId')"
echo "$RUN_TASK_ID"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$RUN_TASK_ID/events"
```

通过标准：

- 最终 `task.completed`
- `finalize_success`

再检查运行事实：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,container_status,last_error,last_started_at
FROM browser_envs
WHERE env_id='$ENV_ID';
"
```

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
```

通过标准：

- `browser_envs.status=running`
- `container_status=running`
- slot 状态为 `occupied`
- `currentPackageId=$ENV_ID`

## 12. Step 6: WebVNC / 连接信息检查

目的：

- 验证 run 后对外连接入口存在

命令：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID/cdp-info" | jq
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID/vnc-info" | jq
```

手动页面：

```text
http://127.0.0.1:3300/web-vnc.html?slot=slot001
```

通过标准：

- `cdp-info` 能返回
- `vnc-info` 能返回
- `web-vnc.html?slot=slot001` 页面可以访问

补充说明：

- 页面能打开，只能说明入口和 ws 代理存在
- 是否能看到真实桌面，还取决于当前运行镜像里是否真的提供 VNC

## 13. Step 7: stop / ending

目的：

- 验证正式 ending 收口
- 验证 slot 是否在结束后恢复成空白 waiting 容器

命令：

```bash
curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/stop" \
  -H "Content-Type: application/json" \
  -d '{"timeoutSeconds":10}' | jq
```

通过标准：

- `browser_envs.status=stopped`
- `container_status=missing`
- slot 回到 `waiting`
- `currentPackageId` 为空
- `currentRunId` 为空

进一步验证“空白 waiting 容器”：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
docker inspect private-browser-slot-slot001 --format '{{json .Mounts}}'
```

通过标准：

- slot 仍有基础容器
- 但 `Mounts` 不应继续挂上一包的 `browser-data/profile`
- 也不应继续保留上一包的运行关系

## 14. Step 8: backup

目的：

- 验证导入后的环境仍能走正式备份链路

命令：

```bash
BACKUP_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/backup")"
printf '%s\n' "$BACKUP_RESP" | jq
export BACKUP_TASK_ID="$(printf '%s' "$BACKUP_RESP" | jq -r '.data.taskId')"
echo "$BACKUP_TASK_ID"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$BACKUP_TASK_ID/events"
```

通过标准：

- 最终 `task.completed`
- `status=backed_up`
- `backup_path` 非空
- 源环境目录已删除

继续检查：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,backup_path,backup_checksum,backup_size,backup_at,last_error
FROM browser_envs
WHERE env_id='$ENV_ID';
"
```

```bash
test -d "$ENV_PATH_ABS" && echo "env path still exists" || echo "env path removed"
```

额外通过标准：

- 如该 env 刚才运行过某个 slot，则该 slot 也必须仍是空白 `waiting`
- 不允许出现“env 已 backed_up，但 slot 还挂着旧包”的脏状态

## 15. Step 9: restore

目的：

- 验证从本机 `backup_path` 正式恢复资产

命令：

```bash
RESTORE_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/restore")"
printf '%s\n' "$RESTORE_RESP" | jq
export RESTORE_TASK_ID="$(printf '%s' "$RESTORE_RESP" | jq -r '.data.taskId')"
echo "$RESTORE_TASK_ID"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$RESTORE_TASK_ID/events"
```

通过标准：

- 最终 `task.completed`
- `status=created`
- 正式目录已恢复
- `backup_path` 已清空
- slot 仍保持 `waiting`

继续检查：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,backup_path,last_error,updated_at
FROM browser_envs
WHERE env_id='$ENV_ID';
"
```

```bash
test -d "$ENV_PATH_ABS" && echo "env path restored" || echo "env path missing"
```

## 16. Step 10: delete package

目的：

- 验证恢复后的资产仍能被正式删除

命令：

```bash
DELETE_RESP="$(curl -s -X DELETE "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/package")"
printf '%s\n' "$DELETE_RESP" | jq
export DELETE_TASK_ID="$(printf '%s' "$DELETE_RESP" | jq -r '.data.taskId')"
echo "$DELETE_TASK_ID"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$DELETE_TASK_ID/events"
```

通过标准：

- SQLite 中查不到该 `envId`
- 正式目录不存在
- backup 包不存在

检查命令：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status
FROM browser_envs
WHERE env_id='$ENV_ID';
"
```

```bash
test -d "$ENV_PATH_ABS" && echo "env path still exists" || echo "env path removed"
```

## 17. Step 11: destroy slot

目的：

- 收尾，删除本轮测试专用的 `slot001`

命令：

```bash
curl -s -X DELETE "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
```

通过标准：

- slot 已删除

检查命令：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
```

## 18. 常见失败解释

### 18.1 `import-package` 提示 duplicate record

含义：

- 同一个 `envId` 已经存在于当前 Client

怎么处理：

- 如果你想恢复已有资产，走 `restore`
- 如果你想验证“外部包重新导入”，先删除旧资产，再重新导入

### 18.2 `restore` 提示 backup 包不存在

含义：

- SQLite 记录了 `backup_path`
- 但那个备份包实际已经被移动、删除或不在原路径

怎么处理：

- 把备份包放回 SQLite 指向的位置
- 或先修正本机备份资产，再重新 restore

### 18.3 `restore` 提示 `envPath 已存在`

含义：

- 当前目标 env 目录已经存在
- 可能是上次失败导入留下的残留目录

怎么处理：

- 先确认是不是脏残留
- 如果是残留目录，先清掉，再重新 restore

## 19. 这条路线最终要收口的结论

本路线的通过，不只是“导入接口能调通”，而是下面这些都成立：

- 外部标准包能成功导入
- 导入后可正常建立本机资产索引
- 导入后可以 run
- run 后 slot 被临时占用
- stop / ending 后 slot 恢复成空白 waiting 容器
- backup 后 env 进入 `backed_up`
- restore 后 env 回到 `created`
- delete 后资产被彻底清理

如果当前 slot runtime 仍是占位镜像，WebVNC 的结论仍然只能写：

- 页面入口可访问
- 连接信息可返回

不要误写成：

- 真实桌面已经验证成功
