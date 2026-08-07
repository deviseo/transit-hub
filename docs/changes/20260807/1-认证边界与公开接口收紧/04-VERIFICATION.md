# 验证与收口

## 验证路线

- 定向测试：认证路由、全局鉴权路径、CORS 行为。
- 运行验证：固定前后端端口、健康检查、未登录响应、跨域响应头。
- 全量门禁：Go 全量测试、race、vet、前端测试、类型检查、构建、依赖审计、格式与 diff 检查。
- 二次 review：逐项对照 `01-SPEC.md`，检查行为回归、遗漏入口和测试有效性。

## 实际结果

- 认证路由、用户接口鉴权和 CORS 的定向回归均通过；废弃认证端点返回 404，真实邮箱密码登录入口仍保留。
- 固定开发服务已按原连接目标重启：前端 `5444`、后端 `5555` 均归属当前仓库，`GET /api/health` 返回 `ok`；PostgreSQL 数据库和关键数据计数与重启前一致，Redis 为 `PONG`。
- 全量验证通过：`go test -p 2 -count=1 ./...`、`go test -race -p 1 -count=1 ./...`、`go vet -p 2 ./...`、全仓 `gofmt -l`、`git diff --check`、部署脚本语法和两套 Compose 配置检查。
- 前端 `npm run typecheck`、`npm run test`、`npm run build` 及两种依赖审计均通过；审计结果为 0 vulnerabilities。

## 剩余风险与收口判定

- 已收口本变更定义的公开访问和跨域问题；未扩大为角色权限系统。`/api/users` 现在要求 TransitHub 登录态，但其返回范围仍是全用户列表，未来引入多用户运营角色时需单独设计授权边界。
