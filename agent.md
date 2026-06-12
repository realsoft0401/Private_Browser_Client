# Private_Browser_Client 开发代理规范

## 构建与镜像约束

- `docker build` / `docker buildx build` 默认使用清华镜像源。
- Dockerfile 里的 Debian 包源默认使用 `mirrors.tuna.tsinghua.edu.cn`，除非明确说明要切回官方源或海外源。
- Go 依赖下载和镜像内 `go mod download` / `go build` 必须配置可用 `GOPROXY`，当前默认使用清华 Go 代理：
  - `https://mirrors.tuna.tsinghua.edu.cn/git/goproxy/,direct`
- 如果后续在 CI、海外网络或客户内网环境里需要切换源，必须通过 `--build-arg` 或环境变量显式覆盖，不能在构建链路里混用随机镜像源。

## 设计原因

- 当前项目的 Docker 构建经常依赖 Debian `apt` 和 Go module 下载，国内网络直连官方源容易超时或卡住。
- 统一使用清华源，是为了降低 `docker build`、`go mod download`、`go build` 在本地、测试机和发布前构建阶段的不确定性。
- 这条规则属于构建稳定性要求，不是临时调试动作；后续维护时不要随手改回多套默认源混用。

## 接口文档沉淀规则

- `Private_Browser_Client` 正式接口后续统一在 `docs/api/` 下提供逐接口 Markdown 文档，采用“一接口一文件”方式。
- `docs/openapi.yaml` 继续作为协议事实源；`docs/api/*.md` 负责把业务语义、状态机、任务编排、失败收口和管理员排障方式讲清楚。
- 后续新增或正式推进的 Edge 接口，除了更新 `docs/openapi.yaml`，还必须同步新增或更新对应 Markdown 文档。
- `docs/api/*.md` 至少应包含：
  - 功能目标
  - 业务边界
  - 请求与响应
  - 前置校验
  - 状态流转
  - 任务编排
  - 成功判定
  - 失败判定
  - 日志字段
  - 联调验收标准
- Docker、run/stop/backup/restore/revalidate/import-package、proxy 更新、VNC/CDP 诊断这类正式接口，不能只保留 Swagger；必须补齐逐接口文档，方便后续平台、实施、联调和管理员直接阅读。
