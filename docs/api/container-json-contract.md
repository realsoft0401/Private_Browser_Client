# container.json 最终定位

## 目标

这份文档只收口 `container.json` 的职责边界，避免后续再把它和 `profile.json`、`binding.json`、Docker 实时事实混用。

## 最终定位

`container.json` 保留，但它已经降级为：

- 最近一次 run / stop 的本机运行摘要文件

它不是：

- 环境包身份真相源
- 配置真相源
- 登录态真相源
- 资产完整性主依据
- 容器运行最终事实的最高优先级来源

## 它负责保存什么

按当前项目口径，`container.json` 只应保存运行摘要，例如：

- `envId`
- `containerName`
- `containerId`
- `image`
- `status`
- `ports`
- `docker.apiUrl`
- `docker.deviceArch`
- `labels`
- `createdAt`
- `startedAt`
- `stoppedAt`
- `updatedAt`

这些字段的意义是：

- 辅助排障
- 辅助详情展示
- 辅助最近一次运行位置追踪
- 在 Docker 不可即时读取时提供最近一次已知摘要

## 它不负责什么

`container.json` 不能承担下面这些职责：

- 不能定义环境包身份
- 不能替代 `profile.json`
- 不能替代 `binding.json`
- 不能作为 run 是否允许的唯一依据
- 不能覆盖 Docker 当前真实状态
- 不能在 import/restore 后保留旧节点的运行事实继续沿用

## 优先级口径

运行事实优先级必须固定为：

1. Docker 当前真实事实
2. SQLite 运行态摘要
3. `container.json` 最近一次运行摘要

也就是说：

- Docker 与 `container.json` 冲突时，以 Docker 为准
- 之后再回写 `container.json` 和 SQLite 摘要

## 缺失时怎么处理

`container.json` 缺失本身，不等于环境包资产损坏。

受控流程下允许：

- `import-package` 时重建空运行摘要
- `revalidate` 时重建空运行摘要
- `restore` 后按需要回写新的运行摘要

但前提是下面这些原子材料必须完整：

- `profile.json`
- `binding.json`
- `proxy/`
- `fingerprint/`
- `browser-data/profile`

所以收口规则是：

- 缺 `container.json`：可以修
- 缺原子材料：不能按普通生命周期动作继续

## 与 backup / restore / import 的关系

### backup

标准完整包可以包含 `container.json`，因为它能保留最近一次运行摘要供审计和排障使用。

### import-package

导入时即使 tar 内包含旧 `container.json`，也必须：

- 清空旧 `containerId`
- 清空旧运行状态
- 清空旧运行时间
- 按当前节点重新生成运行摘要

### restore

restore 恢复的是环境包资产，不是复活旧容器现场。

因此 restore 后：

- 可以恢复 `container.json` 这个摘要文件本身
- 但真实运行态仍要以下一次 run 后的新事实为准

## 与 profile.json 的边界

- `profile.json`：身份、配置、路径、恢复入口
- `binding.json`：绑定事实、运行保护、身份一致性
- `container.json`：最近一次本机运行摘要

这三者不能串位。

尤其不能退回旧误区：

- 把运行状态写回 `profile.json`
- 让 `container.json` 反向定义资产身份
- 仅凭 `container.json.status=running` 就判断环境可用

## 最终收口

`container.json` 是运行摘要文件，不是资产主文件。

它可以被刷新、清空旧运行态、受控重建，但不能被提升为环境包身份或配置真相源。
