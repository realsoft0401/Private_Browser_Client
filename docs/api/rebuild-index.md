# Edge API 设计：`POST /api/v1/edge/browser-envs-rebuild/{envId}`

## 1. 功能目标

`POST /api/v1/edge/browser-envs-rebuild/{envId}` 用于从当前节点已有环境包目录重建单个 SQLite 索引。

## 2. 业务边界

- 只重建 SQLite 索引
- 不启动 Docker
- 不拉镜像
- 不创建新环境包资产

## 3. 请求与响应

```http
POST /api/v1/edge/browser-envs-rebuild/{envId}
```

返回重点：

- 重建后的索引摘要
- 重新确认的端口
- 当前状态结论

## 4. 前置校验

- 候选目录必须原子完整
- 不能已有同名 SQLite 索引
- Docker 不能存在身份冲突

## 5. 状态流转

- 成功后恢复索引可见性
- 不自动进入 `running`

## 6. 成功判定

- 索引重建成功
- 本机端口冲突检查通过

## 7. 失败判定

- 已存在索引
- Docker 身份冲突
- 原子材料不完整

## 8. 日志字段

- `envId`
- `ports`
- `status`
- `error`

## 9. 联调验收标准

- 重建后列表接口可见
- 不会因为重建成功就自动创建容器
