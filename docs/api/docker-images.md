# GET /api/v1/edge/docker/images

## 当前状态

- 正式协议已收口
- 当前新 Client 代码暂未实现

## 功能目标

列出当前 Client 本机 Docker 镜像摘要。

## 业务边界

- 只返回镜像摘要
- 不写数据库
- 不创建任务
- 不根据镜像自动判断哪个环境一定可运行

## 前置校验

- Docker 必须可达

## 请求与响应

### 请求

```http
GET /api/v1/edge/docker/images
```

### 成功响应

```json
{
  "code": 1000,
  "message": "success",
  "data": [
    {
      "id": "sha256:abc123",
      "repoTags": [
        "crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge:1.1-amd64"
      ],
      "repoDigests": [],
      "created": 1718400000,
      "size": 1234567890
    }
  ]
}
```

## 状态流转

- 只读接口，不改状态

## SSE 说明

- 本接口不用 SSE

## 任务编排

- 不创建 task

## 成功判定

- 能列出镜像摘要数组

## 失败判定

- Docker API 不可达
- 镜像列表读取失败

## 日志字段

- `action=list-docker-images`
- `dockerApiUrl`
- `imagesCount`
- `error`

## 联调验收标准

- 拉取镜像后可在本接口看到新增标签
- 不返回中心侧字段
