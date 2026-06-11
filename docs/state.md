# Private_Browser_Client 状态文档

## Client 健康状态

`/health` 只返回 Client 本机视角：

```text
healthy:
  HTTP API、data 目录、SQLite、Docker、statusSync 正常。

unhealthy:
  任何关键检查失败。
```

Client 不返回：

```text
offline
stale
verified
```

这些是 Node Server 的中心状态。

## 环境包主状态

`browser_envs.status` 是环境包资产生命周期主状态：

```text
created
running
stopped
backed_up
error
deleted
```

## Docker 事实状态

`container_status` 只表示 Docker 容器事实：

```text
running
exited
missing
unknown
```

不要用 Docker 快照直接覆盖环境包资产状态。

示例：

```text
status = backed_up
container_status = missing
```

这是正常状态，不是错误。

## 监控状态

`monitor_status` 只表示后台状态同步 Worker 对环境包的观察结果。

状态同步 Worker 只允许：

```text
读取 Docker 事实
更新运行态摘要
记录 last_error
```

禁止：

```text
删除 browser-data/profile
创建环境包
重建容器
修改 proxy/fingerprint/binding
替代 run/stop/backup/restore/delete
```

## 原子文件状态

环境包必须具备完整原子材料：

```text
profile.json
binding.json
proxy/
fingerprint/
browser-data/profile
```

缺少任何必需材料都不能带病 run。

## 网络指纹状态

容器 running 不等于环境可用。

run 必须完成：

```text
浏览器可达
CDP 可达
代理出口可用
timezone/网络指纹探测通过
runtimeProtection 可用
```

失败时应进入 error 或待排查状态，不能静默沿用旧结果。
