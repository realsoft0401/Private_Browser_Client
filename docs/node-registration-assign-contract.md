# Private_Browser_Client Node Registration Assign 协议

## 1. 文档目标

这份文档只服务当前 `Client <-> Node Server` 的第一阶段绑定协同：

```text
Node 发现 Client
  -> Node bind 生成 clientId
  -> Node 下发 clientId 给 Client
  -> Client 写入本地 node-registration.json
```

它不讨论：

- Env 生命周期
- slot/package 运行流程
- 平台额度
- verify/health 完整放行链路

## 2. Client 侧统一边界

### 2.1 Client 不生成 clientId

`clientId` 不是 Client 本机生成的设备号。

它只能来自：

- Node Server 绑定成功后的下发结果

Client 不能：

- 自己创建 `clientId`
- discovered 阶段假装自己已有 `clientId`
- 用本地文件里的 `clientId` 反向覆盖 Node 真相

### 2.2 本地 JSON 只是留痕缓存

`data/node-registration.json` 的职责只有三个：

1. 重启后保留上一次 Node 下发结果
2. 方便联调时确认“这台 Client 上次拿到的中心身份是什么”
3. 在 Node push 成功后给本机接口一个明确可查的落地点

它不是：

- 中心真相源
- 绑定关系真相源
- 是否允许业务放行的最终依据

### 2.3 当前有效身份仍以 Node 为准

这句必须固定：

> Client 本地 `node-registration.json` 只能证明“Node 曾经给我下发过这个结果”，不能单独证明“当前中心绑定仍然有效”。

## 3. Client 需要提供的接口

第一阶段建议 Client 只配合两个接口：

### 3.1 `GET /api/v1/edge/node-registration`

作用：

- 回显当前 Node 注册相关配置
- 回显本地 JSON 缓存
- 回显实时远端查询结果

### 3.2 `POST /api/v1/edge/node-registration/assign`

作用：

- 专门接收 Node Server 下发的 `clientId`
- 把结果写入本地 `data/node-registration.json`

注意：

- 这不是 Client 自注册接口
- 这不是 discovered 接口
- 这是“中心已决定，边缘留痕”的受控赋值接口

## 4. assign 接口定义

## `POST /api/v1/edge/node-registration/assign`

### 4.1 功能目标

接收 Node 下发的中心绑定结果，并写入本地 JSON 文件。

### 4.2 负责什么

- 校验 `X-Edge-API-Key`
- 校验请求字段
- 写入本地 `data/node-registration.json`
- 返回当前本地缓存结果

### 4.3 不负责什么

- 不校验账号是否真的有权绑定
- 不生成 `clientId`
- 不决定是否允许运行业务
- 不把本地结果反向同步成 Node 真相

## 4.4 Header 校验规则

第一阶段按 old 方式收口，不重新设计新 token：

```text
X-Edge-API-Key: <edge-api-key>
```

Client 必须：

1. 读取 `X-Edge-API-Key`
2. 按 old 既有 Edge API Key 方式校验
3. Header 缺失或不匹配时直接拒绝

这样做的原因是：

- Node -> Client 正式受控调用继续共用一套老口径
- assign 不会变成一条单独裸露的改文件接口
- 后续 bind/push/其他 Edge 动作的安全边界一致

## 5. 请求体建议

```json
{
  "clientId": "9060901190001",
  "accountId": "906090119",
  "source": "node-bind",
  "assignedAt": 1781609100
}
```

### 5.1 字段说明

- `clientId`
  - Node 生成的中心设备身份
  - 必填
- `accountId`
  - 当前绑定账号
  - 必填
- `source`
  - 本次下发来源
  - 建议保留，便于后续排障
- `assignedAt`
  - Node 侧完成绑定并下发时的时间戳
  - 建议保留

## 6. 本地 JSON 文件结构建议

文件路径：

```text
Private_Browser_Client/data/node-registration.json
```

建议结构：

```json
{
  "clientId": "9060901190001",
  "mainAccountId": "906090119",
  "nodeServerBaseUrl": "http://127.0.0.1:3400",
  "nodeName": "liningdeMacBook-Air.local",
  "baseUrl": "http://192.168.10.220:3300",
  "clientIp": "192.168.10.220",
  "dockerApiUrl": "http://192.168.10.220:2375",
  "source": "node-bind",
  "registeredAt": 1781609100,
  "updatedAt": 1781609100
}
```

### 6.1 为什么保留这些字段

- `clientId`
  - 本地最核心留痕
- `mainAccountId`
  - 记录这次下发绑定到了哪个账号
- `nodeServerBaseUrl`
  - 记录是谁下发的
- `nodeName/baseUrl/clientIp/dockerApiUrl`
  - 记录下发时这台 Client 的本机事实，便于重启后排障
- `source`
  - 区分是 bind 直接下发，还是后续补推
- `registeredAt/updatedAt`
  - 记录首次与最近一次写入时间

## 7. 写入规则

### 7.1 成功写入条件

只有下面条件满足时才允许写入：

1. `clientId` 非空
2. `accountId` 非空
3. 当前服务配置已完成基础初始化

### 7.2 覆盖规则

第一阶段建议采用：

- 同一 `clientId` 重复下发
  - 允许覆盖更新时间
- 不同 `clientId` 下发到同一台 Client
  - 允许写入，但必须明确记录是覆盖写
  - 同时在日志里说明“中心重新下发了新的 clientId”

原因：

- 真相源在 Node
- Client 只能接收结果，不应在本地阻止中心修正

但要注意：

- Client 可以记录告警日志
- 不能因为本地已有旧值就拒绝 Node 的新下发

## 8. 成功响应建议

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "written": true,
    "cachePath": "/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/data/node-registration.json",
    "registration": {
      "clientId": "9060901190001",
      "mainAccountId": "906090119",
      "nodeServerBaseUrl": "http://127.0.0.1:3400",
      "nodeName": "liningdeMacBook-Air.local",
      "baseUrl": "http://192.168.10.220:3300",
      "clientIp": "192.168.10.220",
      "dockerApiUrl": "http://192.168.10.220:2375",
      "source": "node-bind",
      "registeredAt": 1781609100,
      "updatedAt": 1781609100
    }
  }
}
```

## 9. 失败场景建议

### 9.1 `X-Edge-API-Key` 缺失或不匹配

```json
{
  "code": 1006,
  "message": "assign clientId failed: X-Edge-API-Key 无效"
}
```

### 9.2 `clientId` 为空

```json
{
  "code": 1002,
  "message": "assign clientId failed: clientId 不能为空"
}
```

### 9.3 `accountId` 为空

```json
{
  "code": 1002,
  "message": "assign clientId failed: accountId 不能为空"
}
```

### 9.4 本地文件写入失败

```json
{
  "code": 1004,
  "message": "assign clientId failed: write node registration cache failed: permission denied"
}
```

错误信息要保留可修复性，不能只写“写入失败”。

## 10. `GET /api/v1/edge/node-registration` 返回建议

这个接口建议至少返回三层信息：

### 10.1 配置层

- `enabled`
- `configReady`
- `configMessage`
- `serverBaseUrl`
- `baseUrl`
- `clientIp`
- `dockerApiUrl`

### 10.2 本地缓存层

- `cacheStatus`
- `cacheMessage`
- `cachedRegistration`

### 10.3 实时远端层

- `lookupStatus`
- `lookupMessage`
- `registration`

这样联调时可以很清楚地区分：

- 这是本地文件里有什么
- 这是 Node 现在实时查询返回什么

## 11. 与 Node 协同口径

Node 和 Client 需要明确同一套规则：

### 11.1 Node 端负责

- discovered
- bind
- 生成 `clientId`
- 下发 `clientId`
- 中心真相记录

### 11.2 Client 端负责

- 接收 `clientId`
- 写入本地 JSON
- 提供本地缓存查询

### 11.3 不允许混淆

不允许把下面两件事混成一件事：

- Node bind 成功
- Client 本地 JSON 已写入

因为它们是两个不同层次：

- bind 成功：中心真相成功
- JSON 已写入：边缘协同成功

## 12. 日志建议

assign 接口至少应记录这些字段：

```text
stage=assign_client_id
clientId
accountId
source
cachePath
result
error
```

目的：

- 后续排障一眼知道有没有真正写到本地
- 知道是不是路径权限、字段缺失、还是配置未初始化

## 13. 联调验收标准

Client 侧只要满足下面 5 点，就算第一阶段配套完成：

1. `POST /api/v1/edge/node-registration/assign` 可调用
2. 成功后本地生成 `data/node-registration.json`
3. JSON 内容字段齐全
4. `GET /api/v1/edge/node-registration` 能读到 `cachedRegistration`
5. 缺失或错误 `X-Edge-API-Key` 时会被拒绝
6. Node 再次 push 同一 `clientId` 时不会报错

## 14. 当前最终收口

这份协议的核心只有三句话：

1. Client 不生成 `clientId`。
2. Client 只把 Node 下发结果写进本地 `node-registration.json`。
3. 这个 JSON 是留痕缓存，不是中心真相源。

只要这三句不被破坏，Client 端这条协同链路就不会再乱。
