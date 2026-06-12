# Edge API 设计：`POST /api/v1/edge/browser-envs/{envId}/backup`

## 1. 功能目标

`POST /api/v1/edge/browser-envs/{envId}/backup` 用于把本机环境包备份为标准包，并释放运行目录。

## 2. 业务边界

- 生成标准 `.tar.gz`
- 成功后删除源环境目录
- 保留 SQLite 索引为 `backed_up`
- 不删除浏览器镜像

## 3. 请求与响应

```http
POST /api/v1/edge/browser-envs/{envId}/backup
```

同步返回结果，不创建 Edge task。

返回重点：

- `envId`
- `status=backed_up`
- `backupPath`
- `checksum`

## 4. 前置校验

- 环境包必须存在
- 不能处于 `running`
- 原子材料必须完整

## 5. 状态流转

- 备份成功后进入 `backed_up`
- `containerStatus=missing` 可以是正常结果

## 6. 成功判定

- 备份包生成成功
- checksum 校验成功
- 源环境目录释放成功
- SQLite 索引保留

## 7. 失败判定

- 环境包仍在运行
- 打包失败
- checksum 校验失败
- 删除源目录失败

## 8. 日志字段

- `envId`
- `backupPath`
- `checksum`
- `error`

## 9. 联调验收标准

- 备份后无法直接 run，必须先 restore
- 备份后镜像仍保留
- 备份失败时不应删除源环境目录
