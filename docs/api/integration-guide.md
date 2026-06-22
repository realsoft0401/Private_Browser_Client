# Integration Guide

## 目标

这份文档给前端、Node Server 和联调同学直接使用。

只回答三件事：

1. 常见场景该调哪个接口
2. 哪些接口不能当成业务主入口
3. SSE 接口到底怎么用

## 一句话总原则

- 正式业务动作，默认先看 `browser-envs/*`
- 资源位管理，才看 `slots/*`
- Docker 排障，才看 `docker/*`
- 裸容器救援，才看 `containers/*`

## 1. 场景到接口的映射

### 创建设备上的新环境包

- `POST /api/v1/edge/browser-envs`

说明：

- 只创建资产
- 不自动 run

### 查看本机有哪些环境包

- `GET /api/v1/edge/browser-envs`

### 查看单个环境包详情

- `GET /api/v1/edge/browser-envs/{envId}`

### 运行一个环境包

- `POST /api/v1/edge/browser-envs/{envId}/run`

说明：

- 这是正式业务主入口
- 不要改用 `containers/{slotId}/start`

### 停止一个环境包

- `POST /api/v1/edge/browser-envs/{envId}/stop`

### 备份一个环境包

- `POST /api/v1/edge/browser-envs/{envId}/backup`

### 从本机 backup 恢复环境包

- `POST /api/v1/edge/browser-envs/{envId}/restore`

### 导入外部 tgz/tar.gz 包

- `POST /api/v1/edge/browser-envs/import-package`

### 彻底删除环境包资产

- `DELETE /api/v1/edge/browser-envs/{envId}/package`

### 只删除环境包关联镜像

- `DELETE /api/v1/edge/browser-envs/{envId}/del`

### 只想改代理模式

- `PATCH /api/v1/edge/browser-envs/{envId}/proxy-mode`

### 想拿到当前 slot 的 CDP 入口

- `GET /api/v1/edge/slots/{slotId}/cdp-info`

### 想看本机 slot 当前状态

- `GET /api/v1/edge/slots`
- `GET /api/v1/edge/slots/{slotId}`

### 想创建或销毁 slot

- `POST /api/v1/edge/slots`
- `DELETE /api/v1/edge/slots/{slotId}`

### 想看 WebVNC / CDP 地址

- `GET /api/v1/edge/slots/{slotId}/vnc-info`
- `GET /api/v1/edge/slots/{slotId}/cdp-info`

### 想确认 Docker 正不正常

- `GET /api/v1/edge/docker/status`

### 想看镜像列表

- `GET /api/v1/edge/docker/images`

### 想看项目相关容器列表

- `GET /api/v1/edge/docker/containers`

### 想提前拉镜像

- `POST /api/v1/edge/docker/pull-image`

### 想手工删镜像

- `POST /api/v1/edge/docker/remove-image`

### 想直接救援某个 slot 容器

- `POST /api/v1/edge/containers/{slotId}/start`
- `POST /api/v1/edge/containers/{slotId}/stop`
- `POST /api/v1/edge/containers/{slotId}/restart`

说明：

- 这是运维诊断能力
- 不等于业务动作成功

## 2. 哪些接口不能混用

### 不能用裸容器接口替代正式业务动作

禁止：

- 用 `containers/{slotId}/start` 替代 `browser-envs/{envId}/run`
- 用 `containers/{slotId}/stop` 替代 `browser-envs/{envId}/stop`

原因：

- 裸容器接口不保证包资产校验
- 不保证 slot 关系、包状态、runtimeProtection 正确回写

### 不能用镜像接口替代环境包资产动作

禁止：

- 用 `docker/remove-image` 替代 `browser-envs/{envId}/package`

原因：

- 镜像动作不是资产动作

### 不能只看容器 running 就认定业务成功

必须记住：

- 容器 `running` 不等于环境包可用
- 包主状态才是用户可见主状态

## 3. SSE 使用规则

### 哪些接口是 SSE 任务接口

当前这类接口只会立即返回：

- `taskId`
- `taskType`
- `resourceType`
- `resourceId`
- `eventsUrl`

典型包括：

- `browser-envs/{envId}/run`
- `browser-envs/{envId}/backup`
- `browser-envs/{envId}/restore`
- `browser-envs/{envId}/revalidate`
- `browser-envs/import-package`
- `browser-envs/{envId}/package`
- `docker/pull-image`
- `docker/remove-image`
- `containers/{slotId}/start|stop|restart`

### SSE 正确调用方式

1. 先调任务发起接口
2. 拿到 `taskId/eventsUrl`
3. 再订阅 `GET /api/v1/edge/tasks/{taskId}/events`
4. 只在收到 `task.completed` 时判成功
5. 收到 `task.failed` 或无法确认最终事实时判失败

### 不能犯的错误

- 不能把“任务已接单”当成“动作已成功”
- 不能把 SSE 断流默认当成功
- 不能跳过最终资源事实核对

## 4. Node Server 最小调用建议

### 业务动作前

- 先校验节点可达
- 先确认 `slot` 是否可用
- 先确认目标包状态允许动作

### 动作发起后

- 如果是同步接口，直接按响应处理
- 如果是任务接口，必须继续看 SSE

### 动作结束后

- 必要时重新读资源详情
- 不要只根据中间事件更新中心缓存

## 5. 前端最小展示建议

### 用户主状态看哪层

- 看 `browser-env` 主状态

### slot 页展示什么

- 展示 `waiting/loading/occupied/releasing`
- 展示 WebVNC / CDP 入口

### Docker 页展示什么

- 展示 Docker 状态
- 展示镜像列表
- 展示项目相关容器列表

### 不要混用

- 不要在环境包列表里直接拿容器状态替代包状态
- 不要在业务页默认暴露裸容器运维入口

## 6. 最终收口

给前端和 Node 的最实用判断方法只有一句：

- 只要动作涉及环境包资产、登录态、备份包、运行保护、包状态回写，就必须优先走 `browser-envs/*`

其余三层都只是辅助层，不是正式业务主入口。
