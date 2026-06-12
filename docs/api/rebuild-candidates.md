# Edge API 设计：`GET /api/v1/edge/browser-envs-rebuild/candidates`

## 1. 功能目标

`GET /api/v1/edge/browser-envs-rebuild/candidates` 用于扫描当前节点上可重建 SQLite 索引的环境包候选。

## 2. 业务边界

- 只读扫描本机目录
- 不写 SQLite
- 不修复文件
- 不启动 Docker

## 3. 请求与响应

```http
GET /api/v1/edge/browser-envs-rebuild/candidates
```

返回重点：

- 候选 `envId`
- 原子文件完整性
- 当前是否已存在 SQLite 索引
- 是否存在 Docker 身份冲突

## 4. 前置校验

- data 目录必须可访问

## 5. 状态流转

- 只读，不改状态

## 6. 成功判定

- 成功返回候选扫描结果

## 7. 失败判定

- 目录扫描失败
- 文件读取失败

## 8. 日志字段

- `scannedCount`
- `candidateCount`
- `error`

## 9. 联调验收标准

- 能识别出“目录存在但 SQLite 索引缺失”的候选
- 不会在扫描阶段偷偷重建
