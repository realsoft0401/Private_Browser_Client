# Browser Env E2E 测试方案 A

## 1. 适用场景

这条路线测试的是“正常配置文件创建流”，也就是：

```text
create slot
-> create browser env
-> run
-> stop
-> patch proxy
-> backup
-> restore
-> revalidate
-> delete package
-> destroy slot
```

适用前提：

- 手上没有现成原始 `tgz` 包
- 希望验证当前新 Client 从零创建环境资产的完整链路
- 希望重点验证 `profile.json / binding.json / container.json / SQLite` 的本机生成与回写

这条路线不测：

- 外部原始 `tgz` 包导入
- 同 `envId` 导入冲突
- 导入包内部结构合法性

## 2. 本次测试目标

- 验证 slot 能创建并进入 `waiting`
- 验证 create 后环境包能进入 `created`
- 验证 run 能创建 task，并通过 SSE 收口
- 验证 stop 后 slot 和环境状态一起回落
- 验证 proxy 修改只改配置，不直接 run
- 验证 backup 后目录释放、索引保留
- 验证 restore 后目录恢复、索引回到 `created`
- 验证 revalidate 能把 `error` 收口回 `created`
- 验证 delete package 后目录、备份、索引都清理掉

## 3. 统一变量

本次回归测试对 `slotId` 的命名要求已经统一收口：

- 只允许 `slot001`、`slot002`、`slot003` 这种固定三位编号形式
- 不再使用 `slot-1`、`slot-e2e-001`、`slot-test-*` 这类历史临时命名
- 如果同一台机器要连续做多轮回归，建议按轮次顺延，例如：
  - 第 1 轮：`slot001`
  - 第 2 轮：`slot002`
  - 第 3 轮：`slot003`
- 所有 curl、SQLite、WebVNC、SSE 观察命令，都必须与当前轮次的 `TEST_SLOT_ID` 保持一致
- 如果数据库里还残留旧命名 slot，请先清理旧测试数据，避免把“历史脏数据问题”误判成“本次回归失败”

```bash
export CLIENT_BASE="http://127.0.0.1:3300"
export CLIENT_DB="/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/data/private_browser_client.db"
export PROJECT_ROOT="/Users/lining/Documents/Browser_virtualization/Private_Browser_Client"
export CLASH_FILE="/Users/lining/Documents/analysis_ins/proxy/ClashVerge.yaml"

export TEST_USER_ID="906090001"
export TEST_RPA_TYPE="tk"
export TEST_SLOT_ID="slot001"
export TEST_NAME="Browser-Env-Create-Flow-$(date +%Y%m%d-%H%M%S)"
export RUNTIME_IMAGE="crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64"

export CLASH_BASE64="$(base64 < "$CLASH_FILE" | tr -d '\n')"
```

补充建议：

- 单轮测试建议固定使用一个 slot，例如本方案默认 `slot001`
- 如果 `slot001` 已存在，请先执行销毁，或者改成本轮明确的新编号后再继续
- 不要在同一轮文档执行过程中混用多个 slotId，否则后续 `run / stop / destroy slot` 的观察结果会变得不可靠

## 4. 测试前检查

```bash
curl -s "$CLIENT_BASE/health" | jq
curl -s -o /dev/null -w '%{http_code}\n' "$CLIENT_BASE/swagger"
curl -s -o /dev/null -w '%{http_code}\n' "$CLIENT_BASE/openapi.yaml"
sqlite3 "$CLIENT_DB" ".tables"
```

通过标准：

- `/health` 成功
- `/swagger` 返回 `200`
- `/openapi.yaml` 返回 `200`
- SQLite 表已存在

## 5. 观察命令

### 5.1 browser_envs

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,user_id,rpa_type,status,container_status,monitor_status,backup_path,last_error,updated_at
FROM browser_envs
ORDER BY updated_at DESC;
"
```

### 5.2 slots

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT slot_id,status,current_package_id,current_run_id,container_name,container_status,cdp_port,vnc_port,last_error,updated_at
FROM slots
ORDER BY updated_at DESC;
"
```

### 5.3 runtime_relations

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT run_id,package_id,slot_id,status,last_error,started_at,updated_at
FROM runtime_relations
ORDER BY updated_at DESC;
"
```

## 6. Step 1: create slot

```bash
curl -i -s -X POST "$CLIENT_BASE/api/v1/edge/slots" \
  -H "Content-Type: application/json" \
  -d "{
    \"slotId\": \"$TEST_SLOT_ID\"
  }"
```

通过标准：

- HTTP `201`
- `code=1000`
- `status=waiting`

继续检查：

```bash
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
```

## 7. Step 2: create browser env

```bash
CREATE_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs" \
  -H "Content-Type: application/json" \
  -d "{
    \"userId\": \"$TEST_USER_ID\",
    \"rpaType\": \"$TEST_RPA_TYPE\",
    \"name\": \"$TEST_NAME\",
    \"runtime\": {
      \"image\": \"$RUNTIME_IMAGE\",
      \"startupUrl\": \"https://www.tiktok.com\",
      \"shmSize\": \"1g\"
    },
    \"environment\": {
      \"timezone\": \"Asia/Shanghai\",
      \"screen\": {
        \"width\": 1440,
        \"height\": 900,
        \"depth\": 24
      }
    },
    \"proxy\": {
      \"enabled\": true,
      \"type\": \"clash\",
      \"configBase64\": \"$CLASH_BASE64\"
    }
  }")"

printf '%s\n' "$CREATE_RESP" | jq
export ENV_ID="$(printf '%s' "$CREATE_RESP" | jq -r '.data.envId')"
export ENV_PATH_REL="$(printf '%s' "$CREATE_RESP" | jq -r '.data.envPath')"
export ENV_PATH_ABS="$PROJECT_ROOT/$ENV_PATH_REL"
```

通过标准：

- `envId` 非空
- `status=created`
- `container_status=missing`
- `profile.json` / `binding.json` / `container.json` 存在

继续检查：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,container_status,monitor_status,env_path
FROM browser_envs
WHERE env_id='$ENV_ID';
"

find "$ENV_PATH_ABS" -maxdepth 3 | sort
jq '.environment.language' "$ENV_PATH_ABS/profile.json"
```

## 8. Step 3: run

```bash
RUN_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/run" \
  -H "Content-Type: application/json" \
  -d "{
    \"slotId\": \"$TEST_SLOT_ID\",
    \"forceRecreate\": false
  }")"

printf '%s\n' "$RUN_RESP" | jq
export RUN_TASK_ID="$(printf '%s' "$RUN_RESP" | jq -r '.data.taskId')"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$RUN_TASK_ID/events"
```

通过标准：

- 最终出现 `task.completed`
- `browser_envs.status=running`
- `runtime_relations` 有当前记录
- slot 状态为 `occupied`

继续检查：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,container_status,last_error,last_started_at
FROM browser_envs
WHERE env_id='$ENV_ID';
"

curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID/cdp-info" | jq
curl -s "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID/vnc-info" | jq
```

补充说明：

- `vnc-info` 成功返回，只说明 WebVNC 连接信息存在
- `http://127.0.0.1:3300/web-vnc.html?slot=$TEST_SLOT_ID` 页面可访问，只说明页面入口与 `ws` 代理链路已挂载
- 是否能看到真实浏览器桌面画面，还取决于当前 `slot runtime` 镜像是否真正提供 VNC 服务
- 如果当前测试环境仍使用占位镜像，例如 `alpine + sleep infinity`，这里的通过结论应写成：
  - `webVNC 页面可访问`
  - `vnc-info 可返回`
  - `但当前不验证真实桌面画面`

## 9. Step 4: stop

```bash
curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/stop" \
  -H "Content-Type: application/json" \
  -d "{}" | jq
```

通过标准：

- `browser_envs.status=stopped`
- `container_status=missing`
- slot 回到 `waiting`
- slot 当前态必须与刚停止的包彻底断开：
  `current_package_id` 为空，`current_run_id` 为空
- `waiting` 时的 slot 只能是空白基础服务容器：
  不能继续挂载上一包的 `browser-data/profile`
  不能继续继承上一包的代理、指纹或运行时环境变量

## 10. Step 5: patch proxy

```bash
curl -s -X PATCH "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/proxy" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": false
  }' | jq
```

通过标准：

- `profile.proxy.enabled=false`
- `binding.runtimeProtection.*=pending`
- `proxy/proxy-runtime.json.status=pending`

继续检查：

```bash
jq '.proxy.enabled' "$ENV_PATH_ABS/profile.json"
jq '.runtimeProtection' "$ENV_PATH_ABS/binding.json"
jq '.' "$ENV_PATH_ABS/proxy/proxy-runtime.json"
```

## 11. Step 6: backup

```bash
BACKUP_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/backup")"
printf '%s\n' "$BACKUP_RESP" | jq
export BACKUP_TASK_ID="$(printf '%s' "$BACKUP_RESP" | jq -r '.data.taskId')"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$BACKUP_TASK_ID/events"
```

通过标准：

- 最终 `task.completed`
- `status=backed_up`
- `backup_path` 非空
- 源目录已删除
- 如该 env 在 backup 前关联过某个 slot，则该 slot 也必须已经回到空白 `waiting`
- backup 后不允许出现“slot 仍挂着旧包但 env 已 backed_up”的脏状态

继续检查：

```bash
sqlite3 -header -column "$CLIENT_DB" "
SELECT env_id,status,backup_path,backup_checksum,backup_size,backup_at,last_error
FROM browser_envs
WHERE env_id='$ENV_ID';
"
```

## 12. Step 7: restore

```bash
RESTORE_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/restore")"
printf '%s\n' "$RESTORE_RESP" | jq
export RESTORE_TASK_ID="$(printf '%s' "$RESTORE_RESP" | jq -r '.data.taskId')"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$RESTORE_TASK_ID/events"
```

通过标准：

- 最终 `task.completed`
- `status=created`
- `backup_path` 已清空
- 环境目录恢复

## 13. Step 8: revalidate

先人工置错：

```bash
sqlite3 "$CLIENT_DB" "
UPDATE browser_envs
SET status='error', container_status='error', last_error='manual e2e revalidate injection', updated_at=strftime('%s','now')
WHERE env_id='$ENV_ID';
"
```

再发起：

```bash
REVALIDATE_RESP="$(curl -s -X POST "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/revalidate")"
printf '%s\n' "$REVALIDATE_RESP" | jq
export REVALIDATE_TASK_ID="$(printf '%s' "$REVALIDATE_RESP" | jq -r '.data.taskId')"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$REVALIDATE_TASK_ID/events"
```

通过标准：

- 最终 `task.completed`
- `status=created`
- `last_error` 已清空

## 14. Step 9: delete package

```bash
DELETE_RESP="$(curl -s -X DELETE "$CLIENT_BASE/api/v1/edge/browser-envs/$ENV_ID/package")"
printf '%s\n' "$DELETE_RESP" | jq
export DELETE_TASK_ID="$(printf '%s' "$DELETE_RESP" | jq -r '.data.taskId')"
```

SSE：

```bash
curl -N "$CLIENT_BASE/api/v1/edge/tasks/$DELETE_TASK_ID/events"
```

通过标准：

- 最终 `task.completed`
- SQLite 查不到该 `envId`
- 环境目录不存在

## 15. Step 10: destroy slot

```bash
curl -s -X DELETE "$CLIENT_BASE/api/v1/edge/slots/$TEST_SLOT_ID" | jq
```

通过标准：

- slot 列表查不到该 `slotId`
- SQLite 中也查不到该 `slotId`

## 16. 路线 A 的收口重点

- 这条路线验证的是“新建配置文件资产”的全链路
- 核心观察点是本机生成的 `profile/binding/container` 和索引是否一致
- 这条路线不需要 `import-package`
- 如果要测试原始 `tgz` 包，请改走路线 B
- 如果当前 slot runtime 仍是占位镜像，这条路线里的 WebVNC 只验证“页面与连接信息可访问”，不把“真实桌面可见”作为阻塞通过条件
