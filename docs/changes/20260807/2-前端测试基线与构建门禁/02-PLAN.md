# 实施计划

1. 新增 Vitest 及 V8 coverage 依赖，配置 Node 环境、fork 进程池和最多 2 个 worker。
2. 把利润率测试迁移为 Vitest 测试，并新增真实 `loginWithEmail` 的成功与错误请求测试。
3. 先运行前端测试命令，确认测试基线可重复执行且无端口残留。
4. 只更新 `postcss` 和 `brace-expansion` 的解析版本，复跑 `npm audit`。
5. 在 `vite.config.ts` 为 ECharts 配置独立手动分包，运行生产构建确认 `DashboardView` 告警消失。
6. 格式化 review 检出的 Go 文件，运行 `gofmt -l`。
7. 在最终全量门禁和二次 review 中确认测试、构建和依赖结果。
