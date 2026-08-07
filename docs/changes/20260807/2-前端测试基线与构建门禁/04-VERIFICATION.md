# 验证与收口

## 验证路线

- `npm run test` 与 `npm run test:coverage`：固定 Node 环境、最多 2 worker。
- `npm audit`：分别检查完整依赖和 `--omit=dev` 结果。
- `npm run build`：检查构建完成和 chunk 告警。
- `gofmt -l`：检查全仓 Go 格式。

## 实际结果

- `npm run test` 通过：2 个测试文件、8 个用例；配置使用 Node 环境、fork 进程池和最多 2 个 worker，未启动端口或浏览器。
- `npm run test:coverage` 通过：已测模块语句覆盖率 53.19%、分支 45.78%、函数 42.85%、行 54.65%。报告只用于本轮审查，已立即删除生成的 `frontend/coverage` 目录。
- `npm run typecheck` 和 `npm run build` 通过；ECharts 已独立为约 572 kB 的 vendor chunk，构建未出现原先的 DashboardView 体积告警。
- `npm audit --audit-level=moderate` 与 `npm audit --omit=dev --audit-level=moderate` 均返回 0 vulnerabilities；全仓 `gofmt -l` 无输出。

## 剩余风险与收口判定

- 测试基线可重复且资源受控，但只覆盖工具函数与认证 API；页面、路由、图表渲染和真实登录后的主要流程尚无自动化回归。该缺口应作为后续独立的浏览器测试变更处理，不混入本次修复。
