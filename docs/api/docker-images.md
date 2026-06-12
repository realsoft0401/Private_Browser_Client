# Edge API 设计：`GET /api/v1/edge/docker/images`

## 1. 功能目标

`GET /api/v1/edge/docker/images` 用于列出当前 Edge 节点本机 Docker 镜像摘要。

## 2. 业务边界

- 只返回镜像摘要
- 不写数据库
- 不创建任务
- 不根据镜像自动判断哪个环境可运行

## 3. 请求与响应

```http
GET /api/v1/edge/docker/images
```

返回重点：

- `id`
- `repoTags`
- `repoDigests`
- `created`
- `size`

## 4. 前置校验

- Docker 必须可达

## 5. 状态流转

- 只读，不改状态

## 6. 成功判定

- 成功列出镜像摘要数组

## 7. 失败判定

- Docker API 不可达
- 镜像列表读取失败

## 8. 日志字段

- `dockerApiUrl`
- `imagesCount`
- `error`

## 9. 联调验收标准

- 拉取镜像后可在本接口看到新增标签
- 不返回与业务无关的中心字段
