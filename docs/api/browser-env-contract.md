# Browser Env Contract

## 环境包身份口径

- 环境包唯一标识统一叫 `envId`
- 格式固定为 `userId_rpaType_snowflakeId`
- 示例：`906090001_tk_324867594169356288`

## 环境包目录口径

- 正式环境目录固定为 `data/browser-envs/users/{userId}/{rpaType}/{envId}`
- 目录最后一层必须等于 `envId`
- 不能自动改名、不能自动 clone、不能自动覆盖同 `envId` 环境

示例：

```text
data/browser-envs/users/906090001/tk/906090001_tk_324867594169356288
```

## 配置加载统一口径

正式环境包生命周期动作必须统一按下面顺序读取配置：

1. 先查 SQLite 索引
2. 读取 `envPath`
3. 先读 `profile.json`
4. 校验 `profile.envId/userId/rpaType`
5. 校验 `profile.paths`
6. 再按 `profile.paths` 读取：
   - `binding.json`
   - `container.json`
   - `proxy/clash.yaml`
   - `proxy/proxy-runtime.json`
   - `fingerprint/snapshot.json`
   - `fingerprint/backup.json`
   - `fingerprint/runtime-config.json`
   - `browser-data/profile`

不能退回去的原则：

- 不能跳过 `profile.json` 直接读散文件
- 不能靠请求参数临时补 image / proxy / fingerprint
- 不能缺材料时自动猜测、自动补默认值、自动伪造登录态

## 固定配置口径

- 当前版本 `profile.environment.language` 固定为 `us-en`
- 创建环境包时由 Client 统一写入 `profile.json`
- 调用方不再自由传入 `language`
- 当前版本不提供单独修改 `language` 的业务接口
- `language` 属于环境配置，不属于运行状态

## 运行镜像统一口径

当前整体项目统一使用固定镜像：

```text
crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64
```

约束：

- 接口层不接受调用方自由传 `image`
- slot 常驻运行环境与后续 browser-env 运行链路，当前都沿用该统一镜像口径

## SSE 使用规则

- 只有接口存在明显多阶段、多过程、耗时较长、阶段性进度对前端或管理员有价值、且最终结果不能靠一次同步返回表达清楚时，才允许使用 SSE
- 如果同步 HTTP 已足够表达成功或失败，就应优先使用普通 HTTP
- 每个正式接口文档都必须单独写清 `SSE 说明`

## 当前正式接口边界

- `slots/*`：资源位当前态
- `browser-envs/*`：正式业务生命周期接口

收口原则：

- 后续正式 run / stop / backup / restore / revalidate / import-package / delete 都以 `browser-envs/*` 为准
- 不再保留 `packages/*` 作为对外正式接口面

## browser_envs.status 正式枚举

后续代码、SQLite、接口响应和文档统一只使用下面 6 个主状态：

- `created`
- `running`
- `stopped`
- `backed_up`
- `deleted`
- `error`

约束：

- 不再使用 `error-like`、`pending-like` 这类描述性占位状态
- `run` 失败后如果已经形成需要排障的异常事实，应统一收口到 `error`
- `stop` 成功后统一收口到 `stopped`
- `restore` / `import-package` 成功后统一回到 `created`

## container.json.status 正式枚举

`container.json` 和 SQLite 的容器运行摘要统一使用下面枚举：

- `created`
- `running`
- `exited`
- `missing`
- `error`

约束：

- 新创建但尚未运行的环境，容器摘要建议收口为 `missing`
- Docker 查不到历史容器时，不要伪装成 `exited`，应写真实摘要 `missing`
- 不能仅凭 `container.json.status=running` 判断业务可用

## 生命周期冲突口径

后续 run / stop / backup / restore / revalidate / import-package / delete 遇到并发生命周期冲突时，统一返回：

- `code=1003`
- `message=browser env lifecycle conflict` 或等价固定语义

典型场景：

- 同一 `envId` 正在并发 run / stop
- `running` 状态直接调用 delete / backup
- 资源释放未完成时又触发新的互斥动作

## 包与容器的边界

- 容器是环境包的运行载体，容器里实际承载的运行内容最终来自环境包资产。
- 但正式系统设计里，环境包仍然是资产真相源，容器只是环境包在某一时刻加载后的运行现场投影。
- 正式资产事实仍以 `profile.json`、`binding.json`、`proxy/`、`fingerprint/`、`browser-data/profile`、`envPath` 和 SQLite 索引为准，不以 Docker 容器本身为资产主来源。
- 容器可以被删除、重建、替换，环境包身份与资产不能反向依赖某一个容器实例来定义。
- 因此正式 `backup/restore/import-package/revalidate` 都必须以环境包资产为中心，而不能退化成“以容器现场为中心”的快照逻辑。

## container.json 统一口径

- `container.json` 保留，但只作为最近一次本机运行摘要文件
- 它不是资产身份来源，也不是配置真相源
- Docker 当前事实优先级高于 `container.json`
- `container.json` 缺失时，受控流程可以重建空运行摘要
- 详细边界见 [container-json-contract.md](/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/docs/api/container-json-contract.md)
