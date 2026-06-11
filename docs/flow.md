# Private_Browser_Client 流程文档

## 定位

Client 是边缘服务，只管理本机 Docker、本机浏览器环境包和本机短期任务。

Client 不保存平台用户，不做主账号权限判断，不管理其它 Client。

## 启动流程

```text
1. 读取 Settings/config-docker.yaml。
2. 初始化 SQLite。
3. 初始化环境包目录和本机状态同步 Worker。
4. 启动 HTTP API。
5. 启动 UDP discovery beacon。
6. 周期性同步 Docker 容器事实到本机 SQLite。
```

## UDP discovery 流程

```text
1. Client 按 discovery.interval_seconds 广播 beacon。
2. beacon 只包含非敏感服务摘要。
3. Node Server 收到后进行注册和 verify。
4. Client 不接收 Node Server 分配的 clientId，也不保存中心身份。
```

beacon 禁止包含：

```text
用户数据
环境包状态
proxy 明文
fingerprint raw
Cookies
Local Storage
IndexedDB
Session Storage
Login Data
browser-data 路径
```

## 环境包生命周期流程

```text
create:
  创建环境包目录、profile/binding/proxy/fingerprint/browser-data/profile。

run:
  校验环境包原子文件。
  校验 Docker API。
  创建或启动浏览器容器。
  校验 CDP/VNC/代理/网络指纹。
  返回 taskId 和 SSE eventsUrl。

stop:
  停止本机浏览器容器。
  保留 browser-data/profile。
  更新 SQLite 运行态摘要。

backup:
  归档环境包资产。
  成功后释放运行目录。

restore:
  从本机 backup_path 恢复。
  不自动 run。

import-package:
  上传标准环境包。
  校验 profile/binding/proxy/fingerprint/browser-data。
  重新分配本机端口。
  不自动 run。

del:
  删除环境包关联的本机 Docker 镜像。
  只读取 profile.runtime.image，不删除环境包目录、登录态目录或 SQLite 索引。

package:
  彻底删除环境包、登录态目录、已停止容器和 SQLite 索引。
  不删除 Docker 镜像。
```

## Docker 部署流程

Linux 商用节点需要 UDP 自动发现时使用 host 网络：

```text
--network host
/Business/data:/app/data
/Business/data/config-docker-host.yaml:/app/Settings/config-docker.yaml:ro
```

host 网络下 Docker API 使用：

```text
http://127.0.0.1:2375
```
