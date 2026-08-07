# Priority 写回减负 B 阶段验证与收口

## 当前状态

已实施。A 阶段的全局生产比较器、workspace 租约和逐目标两阶段 checkpoint 已在 `V2.0.4` 收口；B 在其上增加 workspace 级排序签名、pending 合并和写回观测，不改变探活频率或 A 的 priority 区间。本次代码版本为 `V2.0.6`。

## 实际改动

- 策略新增 `prioritySyncPreset`：默认 `minWriteIntervalSeconds=30`、`maxPendingAgeSeconds=300`、`driftAction=alert_only`、`readMode=inventory_snapshot`；前台可调整最短写回间隔。
- 新增 `connection_health_priority_workspace_sync_states`，持久化已应用/最新 pending 签名、时间戳、决策、抑制原因、错误与写回/漂移计数；逐目标主站实际值仍使用原 `connection_health_priority_sync_states`。
- 签名只包含 health-managed / multiplier-only 托管归属和稳定生产顺序，不包含原始延迟、页面顺序或单独健康颜色。排序输入不完整或不可用时抑制写回；人工 priority 漂移记录为 `alert_only`，不覆盖。
- workspace preset 按当前托管策略聚合，低于默认值的最短间隔仍按策略生效；Sub2API priority 不可读时不提交 applied 签名，恢复可读后重试；人工漂移后的新顺序只写非冲突目标并收敛 pending。
- 快速创建策略复用统一 upsert，四个 `priority_*` 字段与普通保存路径一致；workspace 删除清单同步清理新增状态表。
- 普通策略编辑保留未直接展示的 `maxPendingAgeSeconds`，仅在最短写回间隔提高时向上夹紧，避免无关编辑重置既有 preset。
- 健康汇总展示最近写回决策、最短间隔、成功/尝试写入、抑制次数和错误原因。

## 定向验证

- `cd backend && go test ./internal/modules/connection_health ./internal/modules/admin_accounts`：通过。
- `cd frontend && npm run typecheck`：通过。
- `cd frontend && npm test -- connection-health-priority-sync-preset.test.ts connection-health-production-rank.test.ts`：6/6 通过。
- `npm run build`：通过；仅有依赖内 VueUse PURE 注释警告。
- `git diff --check`：通过。
- `priority_sync_gate_test.go` 覆盖：同比例延迟变化的稳定顺序零写入、窗口内多次变化只写最终 pending、失败后重启恢复、人工 priority 漂移重复扫描不重复计数、漂移与其他目标写失败同轮可解释、漂移后的新顺序收敛、多策略最短间隔聚合、Sub2API 托管及解除托管时 priority 恢复可读后重试、pending 超龄在输入抑制及写失败期间保留。
- `group_priority_strategy_test.go` 覆盖快速创建策略 preset round-trip；`repository_delete_test.go` 覆盖新增 workspace 状态表清理登记。

## 观测口径与证据边界

- 受控测试窗口中，初始需要收敛的三目标排序产生 2 次写入；随后三个延迟同比例变化且顺序不变时新增写入为 0。窗口内两次顺序变化没有立即写入，窗口结束后只写最终的 3 个目标顺序。
- 最长 pending 等待在原 gate 测试窗口中为 30 秒；失败样本保留 pending 且 `appliedSignature` 为空；人工漂移样本记录 conflict 且新增写入为 0。超龄失败样本保留 `priority_pending_overdue` 原因，Sub2API priority 不可读样本不写 `appliedSignature`。
- 当前没有真实主站观察窗口和改造前线上基线，因此“实际减负比例、线上最长 pending、限流/错误改善”证据不足，不将定向测试结论表述为已证明线上减负。

## 必验事实

- 三个目标延迟同比例变化但稳定顺序不变时，排序签名不变且零写入。
- 顺序或托管所有权变化时产生最新 pending；30 秒窗口内多次变化最多形成一次最终写入。
- 写入失败、主站读取未知和调度站重启不能伪造 `applied`；恢复后仍以最新签名为准，Sub2API priority 从不可读恢复后即使排序或解除托管签名不再变化也会重试。
- 人工主站 priority 漂移触发告警，不自动覆盖。
- 策略默认值可读取、可调整，30 秒没有散落为业务常量。

## 收口条件

- 相关 Go 定向测试、受影响前端检查和 `git diff --check` 通过；代码修改后完成固定 5444/5555 重启核验，并确认 PostgreSQL/Redis 连接目标和关键数据计数不变。
- 至少提供一个观测窗口的写回次数对比、排序变化次数、最长 pending 等待和失败/漂移样本；没有基线则标记证据不足，不能声称减负已证明。
