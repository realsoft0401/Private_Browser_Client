# Interface Layer Boundary

## 目标

这份文档只回答一件事：

- `docker/*`
- `containers/*`
- `slots/*`
- `browser-envs/*`

这四层接口分别负责什么，不负责什么，哪些场景可以用，哪些场景不能混用。

## 一张总表

| 层级 | 代表接口 | 主职责 | 是否业务主入口 |
| --- | --- | --- | --- |
| Docker 事实层 | `GET /api/v1/edge/docker/*` | 查看本机 Docker 事实 | 否 |
| Docker 运维动作层 | `POST /api/v1/edge/docker/pull-image` `remove-image` | 拉取/删除本机镜像 | 否 |
| Slot 容器运维层 | `POST /api/v1/edge/containers/{slotId}/*` | 直接启停/重启 slot 容器 | 否 |
| Slot 资源层 | `GET/POST /api/v1/edge/slots/*` | 管理 slot 资源位、waiting/loading/occupied/releasing 状态 | 否 |
| Browser Env 资产层 | `POST /api/v1/edge/browser-envs/*` | 管理环境包资产与正式生命周期 | 是 |

## 1. `docker/*` 层

### 负责什么

- 返回本机 Docker 状态
- 返回本机镜像摘要
- 返回本项目相关容器摘要
- 在本机拉取镜像
- 在本机删除镜像

### 不负责什么

- 不决定哪个镜像应该被业务使用
- 不决定平台商业额度
- 不决定哪个环境包是否可以 run
- 不回写环境包生命周期主状态
- 不读取 `browser-data/profile`

### 使用场景

- 管理员确认 Docker 是否正常
- 管理员提前拉取运行镜像
- 管理员清理本机镜像
- 节点排障

### 禁止误用

- 不能把 `pull-image` 当成环境包 `run`
- 不能把 `remove-image` 当成环境包删除
- 不能只因为镜像存在就认定环境包可运行

## 2. `containers/*` 层

### 负责什么

- 直接对指定 `slotId` 对应容器执行 `start`
- 直接执行 `stop`
- 直接执行 `restart`

### 不负责什么

- 不读取环境包完整资产
- 不执行 `profile/binding/proxy/fingerprint/browser-data` 一致性校验
- 不保证 SQLite 的环境包运行态完整同步
- 不保证包侧主状态正确推进
- 不保证 timezone / proxyRuntime / runtimeProtection 校验

### 使用场景

- 内网运维诊断
- slot 容器异常救援
- Docker 层临时排障

### 禁止误用

- 不能当作前端正式业务主入口
- 不能替代 `browser-envs/{envId}/run`
- 不能替代 `browser-envs/{envId}/stop`
- 不能用它来完成 backup / restore / delete / import-package

## 3. `slots/*` 层

### 负责什么

- 创建 slot
- 查询 slot 当前态
- 返回 slot 视角的 CDP/VNC 连接信息
- 资源位重初始化
- 资源位销毁

### 不负责什么

- 不保存环境包完整配置
- 不承担环境包资产真相源
- 不直接替代 browser-env 生命周期

### 使用场景

- 平台先给当前 Client 分配几个槽位
- 管理员查看 slot 当前是否 waiting/occupied
- WebVNC 通过 `slot` 维度访问运行画面

### 核心原则

- `waiting` 必须是空白 slot 容器
- slot 和 package 是解耦的
- package 只是某一时刻被加载到某个 slot 上运行
- stop / ending / backup 后，slot 必须回到空白 waiting

## 4. `browser-envs/*` 层

### 负责什么

- 创建环境包
- 列表/详情查询
- 正式 run
- 正式 stop
- backup / restore
- import-package
- revalidate
- delete package
- proxy / proxy-mode 更新
- CDP 基础诊断

### 不负责什么

- 不做中心调度决策
- 不做平台额度决策
- 不做跨节点自动迁移
- 不允许调用方临时覆盖 image / proxy / fingerprint 真相

### 使用场景

- 一切正式业务生命周期动作
- 与环境包资产、登录态、备份包、运行保护有关的动作
- 需要最终回写包侧状态的动作

### 核心原则

- `browser-envs/*` 才是正式业务主入口
- 用户主状态以包状态为准，不以容器状态为准
- 资产真相源以环境包目录和 SQLite 索引为准
- 容器只是运行现场，不是资产主来源

## 5. 调用优先级

如果目标是正式业务动作，优先级固定如下：

1. 先看是否应该走 `browser-envs/*`
2. 只有资源管理问题才走 `slots/*`
3. 只有本机镜像/Docker 排障才走 `docker/*`
4. 只有容器救援或诊断才走 `containers/*`

## 6. 典型例子

### 例子 1：要运行一个包

正确：

- `POST /api/v1/edge/browser-envs/{envId}/run`

错误：

- 先 `containers/{slotId}/start` 再认为包已经运行

原因：

- 裸容器启动并不等于包资产校验、挂载、CDP、runtimeProtection 都成功

### 例子 2：要停止一个包

正确：

- `POST /api/v1/edge/browser-envs/{envId}/stop`

错误：

- 直接 `containers/{slotId}/stop` 后认为包已经完整收口

原因：

- 裸 stop 不能保证包侧运行摘要、slot 关系、资产状态都正确回写

### 例子 3：要删除一个环境包

正确：

- `DELETE /api/v1/edge/browser-envs/{envId}/package`

错误：

- `docker/remove-image`
- `containers/{slotId}/stop`

原因：

- 删除环境包是资产动作，不是镜像动作，也不是容器动作

### 例子 4：要释放磁盘里的镜像

正确：

- `POST /api/v1/edge/docker/remove-image`
- 或者按 env 语义走 `DELETE /api/v1/edge/browser-envs/{envId}/del`

边界区别：

- `remove-image` 直接按镜像名操作
- `/del` 按 env 的 `profile.runtime.image` 去删
- 两者都不是删除环境包资产

## 7. 最终收口

- `docker/*`：本机 Docker 事实与镜像运维层
- `containers/*`：本机 slot 容器救援层
- `slots/*`：本机资源位层
- `browser-envs/*`：正式业务资产层

后续所有新能力，默认都先问一句：

- 这是 Docker 事实问题？
- 这是容器救援问题？
- 这是 slot 资源问题？
- 还是正式环境包资产生命周期问题？

如果是最后一种，必须优先落到 `browser-envs/*`，不能偷懒退回裸容器接口。
