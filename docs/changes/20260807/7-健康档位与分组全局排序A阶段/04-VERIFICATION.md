# 健康档位与分组全局排序 A 阶段验证与收口

## 当前状态

已完成源码实现和定向验证；本次提交版本为 `V2.0.4`，提交后不推送。

## 必验事实

- 健康目标数量为 90 时覆盖 `10-99`；第 91 个仍为 `99`，不会占用 `100`。
- 恢复中、降级/观察、待探活、暂停/禁用分别落在 `100-999`、`1000-9999`、`10000-99999`、`100000`。
- 一个目标出现在多个分组时，其全局 rank 的排序来源与 `PriorityExpected` 使用同一套全局比较器，但两个字段保持分离；各分组内按同一 rank 排列，且只出现一次写回候选。
- 多分组总览按最小全局 rank，而不是上游返回的分组先后，排列分组。
- 页面不存在“容量已满”提示；末位并列不影响状态或可调度性。

## 验证方式与收口条件

- 运行 `connection_health` 和详情接口相关 Go 定向测试，运行受影响前端的类型/构建或行为检查，并执行 `git diff --check`。
- 代码修改后按项目规则重启 5444 和 5555；确认进程与健康检查正常。纯文档阶段不执行此步骤。
- 记录一次真实或受控数据下的全局 rank/分组展示对照。没有运行证据时必须保持“未收口”，不能因文档完整而标记完成。

## 实际实现与证据

- `backend/internal/modules/connection_health/priority_strategy.go` 将 Sub2API 托管区间更新为健康 `10-99`、恢复中 `100-999`、降级/观察 `1000-9999`、待探活 `10000-99999`、暂停/禁用 `100000`；同一健康档内超过容量时只在末位并列。
- 写回和详情 rank 共同使用 `compareHealthPriorityCandidates`；`AdminGroups` 按去重后的 target 生成 workspace 全局 `productionSortOrder`，同一 target 跨多个分组复用同一 rank，并返回分组 `minProductionRank`。分组和组内账号均按该 rank 排序，空组或账号读取失败的组排在末尾。
- 删除 `priorityCapacityLimited` 后端字段、前端类型、容量计算和“容量已满”展示；未修改 `schedulable`、探活频率或写回节流。
- 受控数据验证：`TestDesiredSub2APIManagedPriority_HealthUsesNinetySlotsAndClampsAtNinetyNine` 覆盖 90 个可区分槽位及第 91 个末位并列；`TestFinalizeAdminGroupProductionOrder_DeduplicatesTargetsAndOrdersGroupsByMinimumRank` 覆盖两个分组、跨组重复 target、完整比较器并列裁决和空组末尾。

## 实际命令结果

- `cd backend && go test ./internal/modules/connection_health`：通过。
- `npm run typecheck --prefix frontend`：通过。
- `npm test --prefix frontend`：通过，5 个测试文件、16 项测试。
- `npm run build --prefix frontend`：通过；Vite 构建完成，仅有依赖包 PURE 注释的既有警告。
- `git diff --check`：通过。

## 固定服务核验

- 重启前确认：前端为当前仓库 `frontend` 下 Vite，后端为当前仓库 `backend` 下 `go run ./cmd/api`；后端使用 PostgreSQL `127.0.0.1:5432/transithub`、用户 `postgres`，`REDIS_URL` 未设置并沿用默认 `127.0.0.1:6379/0`。
- 重启后仅有当前仓库的一个 `5444` 和一个 `5555` 监听；`GET http://127.0.0.1:5444/` 返回 HTTP 200，`GET http://127.0.0.1:5555/api/health` 返回 `{"status":"ok"}`。
- 重启后 PostgreSQL 仍为 `transithub` / `postgres`，Redis `PING` 返回 `PONG`；关键计数与重启前一致：`admin_accounts=2`、`connection_health_states=3`、`connection_health_priority_sync_states=0`。
- 受控数据证据来自上述定向 Go 测试；本轮未使用登录态手工截图或真实 workspace 的 admin-groups 展示，因此不把截图/运行态页面作为额外收口依据。

## 未解决风险

- 当前工作区仍有其他并行 dashboard 改动，未在本阶段拆分、回退或提交；已按提交边界排除并继续保留在工作区。
- 未进行需要登录态的浏览器人工验收；固定开发服务已保持运行，用户可在 `http://127.0.0.1:5444/` 继续审阅。
