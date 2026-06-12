# Edge API 设计：`GET /api/v1/edge/browser-envs/{envId}`

## 1. 功能目标

`GET /api/v1/edge/browser-envs/{envId}` 用于读取单个本机环境包的详情。

## 2. 业务边界

- 从 SQLite 索引和本机文件读取
- 返回 profile/binding/container 等摘要
- 不返回 proxy YAML 明文
- 不返回 fingerprint raw

## 3. 请求与响应

```http
GET /api/v1/edge/browser-envs/{envId}
```

返回重点：

- `index`
- `profile`
- `binding`
- `proxy`
- `fingerprint`
- `consistency`
- `files`
- `vnc`

## 4. 前置校验

- `envId` 必须存在

## 5. 状态流转

- 只读，不改状态

## 6. 成功判定

- 成功读取索引和本机文件
- 能给出一致性检查结果

## 7. 失败判定

- `envId` 不存在
- 原子文件缺失且无法读取详情
- 文件解析失败

## 8. 日志字段

- `envId`
- `status`
- `consistency.errors`
- `error`

## 9. 联调验收标准

- 运行中环境能返回 `vncUrl/vncWsUrl/webVncUrl`
- 文件缺失时能明确落到一致性错误
- 不返回敏感原文
