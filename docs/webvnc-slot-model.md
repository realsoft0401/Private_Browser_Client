# WebVNC Slot Model

## 1. 当前结论

新的 `WebVNC` 不再按 old 的 `envId/package` 视角实现，而是按 `slot` 视角实现。

## 2. 推荐入口

- `/web-vnc.html?slot=1`
- `/web-vnc.html?slot=2`

## 3. 语义

- 页面展示的是某个 slot 当前承载的浏览器画面
- 不是某个 package 天然绑定的固定浏览器
- package 运行到哪个 slot，就通过哪个 slot 查看

## 4. 状态边界

- `waiting`：当前没有运行实例
- `loading`：暂不视为稳定可连接
- `occupied`：可作为正常查看态
- `releasing`：不再视为稳定可连接

## 5. 设计原则

- WebVNC 连接对象是 slot
- package 和 slot 的运行关系由运行态决定
- 不再继续沿用 old 的 `web-vnc.html?envId=...`
