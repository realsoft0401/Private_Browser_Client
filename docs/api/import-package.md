# Edge API 设计：`POST /api/v1/edge/browser-envs/import-package`

## 1. 功能目标

`POST /api/v1/edge/browser-envs/import-package` 用于把标准 `.tar.gz` 环境包导入到当前 Edge 节点本机。

## 2. 业务边界

- 只导入本机环境包
- 保留原 `envId/userId/rpaType`
- 重新分配本机 `envSequence/CDP/VNC`
- 不自动 run
- 不自动拉镜像

## 3. 请求与响应

```http
POST /api/v1/edge/browser-envs/import-package
Content-Type: multipart/form-data
```

请求重点：

- `file`

返回重点：

- `envId`
- `userId`
- `rpaType`
- `envSequence`
- `ports`
- `envPath`
- `status=created`

## 4. 前置校验

- 必须是标准单根目录包
- `profile.json`、原子材料和 checksums 必须可校验
- 本机不能已存在同名 `envId`

## 5. 状态流转

- 导入成功后进入 `created`
- 运行态字段重置
- `proxyRuntime/runtimeProtection` 回到待重新验证口径

## 6. 成功判定

- 包校验通过
- 原子文件导入成功
- 本机端口和 `envSequence` 重分配成功
- SQLite 索引写入成功

## 7. 失败判定

- 包格式非法
- 单根目录校验失败
- checksums 不匹配
- 本机已存在同名 `envId`
- 索引写入失败

## 8. 日志字段

- `envId`
- `userId`
- `rpaType`
- `packageName`
- `error`

## 9. 联调验收标准

- 导入后不自动运行
- 同名 `envId` 冲突时返回 409
- 端口重分配后不与现有服务冲突
