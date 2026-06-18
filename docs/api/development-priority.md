# Development Priority

## 目标

这份文档只回答一件事：

当前 `Private_Browser_Client` 后续接口实现，应该按什么顺序做，为什么这么排。

## 总原则

- 先打通正式业务主链路
- 再补查询与回读能力
- 再补资产增强能力
- 最后补 Docker 运维和裸容器诊断能力

原因：

- 前端和 Node 真正依赖的是 `browser-envs/*`
- 查询能力是为了让任务和业务主链路可验证、可排障
- 运维层很重要，但它不是业务主入口，不应抢在主链路前面

## P0

### 目标

先把“能创建、能运行、能停止、能观察任务、能删除资产”这条正式主链路彻底稳定。

### 当前状态

这一层大部分已经有代码实现，但仍然属于最高优先级维护层。

### 接口

- `POST /api/v1/edge/browser-envs`
- `POST /api/v1/edge/browser-envs/{envId}/run`
- `POST /api/v1/edge/browser-envs/{envId}/stop`
- `DELETE /api/v1/edge/browser-envs/{envId}/package`
- `GET /api/v1/edge/tasks/{taskId}/events`
- `GET /api/v1/edge/slots`
- `POST /api/v1/edge/slots`
- `GET /api/v1/edge/slots/{slotId}`
- `GET /api/v1/edge/slots/{slotId}/vnc-info`
- `GET /api/v1/edge/slots/{slotId}/cdp-info`

### 验收目标

- create -> run -> webvnc/cdp -> stop -> waiting slot 回空白
- package delete 行为明确
- SSE 事件链路稳定
- slot 和 package 解耦逻辑稳定

## P1

### 目标

补齐最基本的查询与回读接口，让 Node Server 和前端能看到完整事实，而不是只靠 task 事件猜。

### 接口

- `GET /api/v1/edge/browser-envs`
- `GET /api/v1/edge/browser-envs/{envId}`
- `GET /api/v1/edge/tasks/{taskId}`
- `GET /api/v1/edge/browser-envs/{envId}/cdp-test`

### 为什么排在这里

- 没有这些接口，前端和 Node 在任务结束后很难稳定回读事实
- 这批接口不直接改变资产，但会直接影响联调效率和排障能力

### 验收目标

- 列表能作为主查询入口
- 详情能看到 index/profile/binding/container 一致性摘要
- task detail 能支持刷新后继续查看任务
- cdp-test 能单独判断 CDP 是否可用

## P2

### 目标

补齐正式资产增强能力，让包真正能完成导入、备份、恢复和受控修复。

### 接口

- `POST /api/v1/edge/browser-envs/{envId}/backup`
- `POST /api/v1/edge/browser-envs/{envId}/restore`
- `POST /api/v1/edge/browser-envs/import-package`
- `POST /api/v1/edge/browser-envs/{envId}/revalidate`
- `DELETE /api/v1/edge/browser-envs/{envId}/del`

### 为什么排在这里

- 这是正式业务必需能力
- 但前提是 P0/P1 已经让主链路和查询回读稳定
- 特别是 backup/restore/import-package 都会直接影响资产边界和目录真相

### 验收目标

- backup 后 slot 回到空白 waiting
- restore 后恢复到 `created`
- import-package 能保留 `envId/userId/rpaType` 并重新分配本机端口
- `/del` 只删镜像，不删资产

## P3

### 目标

补齐代理修改相关能力。

### 接口

- `PATCH /api/v1/edge/browser-envs/{envId}/proxy`
- `PATCH /api/v1/edge/browser-envs/{envId}/proxy-mode`

### 为什么排在这里

- 这两条能力重要，但必须建立在主生命周期、查询回读和资产动作稳定之后
- 否则改代理后的重建、校验和回写会把主链路复杂度进一步放大

### 验收目标

- 非运行态同步更新配置
- 运行态按需任务化重建
- `binding.version`、`runtimeProtection`、`proxyRuntime` 回写正确

## P4

### 目标

补齐 Docker 事实与运维层。

### 接口

- `GET /api/v1/edge/docker/status`
- `GET /api/v1/edge/docker/images`
- `GET /api/v1/edge/docker/containers`
- `POST /api/v1/edge/docker/pull-image`
- `POST /api/v1/edge/docker/remove-image`

### 为什么排在这里

- 这些接口主要服务运维、管理员和排障
- 它们不是前端业务主入口
- 文档要有，但实现顺序不应先于正式资产主链路

### 验收目标

- Docker 状态可读
- 镜像列表、容器列表可读
- pull/remove-image 的 SSE 链路清晰

## P5

### 目标

补齐裸容器救援接口。

### 接口

- `POST /api/v1/edge/containers/{slotId}/start`
- `POST /api/v1/edge/containers/{slotId}/stop`
- `POST /api/v1/edge/containers/{slotId}/restart`

### 为什么排最后

- 这是运维救援层，不是业务主链路
- 如果太早实现，最容易被前端或上层系统误当成正式业务入口

### 验收目标

- 只作用于 slot 容器
- 文档、Swagger、错误信息都明确它不是 browser-env 主入口
- SSE 任务过程可见

## 推荐开发顺序

1. 持续稳定 P0 已实现主链路
2. 开始做 P1 查询与回读
3. 再做 P2 资产增强动作
4. 再做 P3 代理修改
5. 最后做 P4 Docker 运维和 P5 裸容器救援

## 当前最建议立刻开始的下一批

如果现在正式开写代码，我建议下一批顺序直接按下面来：

1. `GET /api/v1/edge/browser-envs`
2. `GET /api/v1/edge/browser-envs/{envId}`
3. `GET /api/v1/edge/tasks/{taskId}`
4. `GET /api/v1/edge/browser-envs/{envId}/cdp-test`
5. `POST /api/v1/edge/browser-envs/{envId}/backup`
6. `POST /api/v1/edge/browser-envs/{envId}/restore`
7. `POST /api/v1/edge/browser-envs/import-package`

## 最终收口

后续只要出现“这个接口到底先不先做”的讨论，就按下面判断：

- 能不能直接提升正式业务主链路完整度
- 能不能让 Node / 前端稳定回读事实
- 会不会影响资产真相源
- 它是不是只是运维辅助层

如果只是运维辅助层，就默认排在正式 `browser-envs/*` 后面。
