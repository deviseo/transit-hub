package group_rate_campaigns

import (
	"math"
	"strings"
	"time"
)

// multiplierEpsilon 是判断两个倍率是否"实质相同"的容差，用于跳过无意义的远端调用。
const multiplierEpsilon = 1e-9

// multiplierChanged 判断目标倍率与当前倍率是否存在实质差异。
func multiplierChanged(a, b float64) bool {
	return math.Abs(a-b) > multiplierEpsilon
}

// applyAdjustment 是活动调价的核心纯函数：按 set/multiply/add 三种模式，基于原倍率计算活动倍率。
// 不接受 NaN/Inf；set 结果必须 >= 0；multiply 系数必须 > 0；add 计算后结果不能 < 0。
func applyAdjustment(mode AdjustmentMode, value float64, original float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, ErrInvalidAdjustment
	}
	switch mode {
	case AdjustmentSet:
		if value < 0 {
			return 0, ErrInvalidAdjustment
		}
		return value, nil
	case AdjustmentMultiply:
		if value <= 0 {
			return 0, ErrInvalidAdjustment
		}
		return original * value, nil
	case AdjustmentAdd:
		result := original + value
		if result < 0 {
			return 0, ErrInvalidAdjustment
		}
		return result, nil
	default:
		return 0, ErrInvalidAdjustment
	}
}

// validateCreateRequest 校验创建活动请求的静态规则（不依赖分组数据的部分）。
// 新口径：活动调价只允许手动选择分组，且每个分组必须单独提供固定活动倍率；
// all/type/currentFilter 选择方式和 multiply/add 调价方式只保留给历史活动只读展示，新建请求一律拒绝。
func validateCreateRequest(req CreateCampaignRequest, now time.Time) error {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 80 {
		return ErrInvalidName
	}

	if req.Selection.Mode != SelectionManual {
		return ErrEmptySelection
	}
	if req.Adjustment.Mode != AdjustmentSet {
		return ErrInvalidAdjustment
	}
	if err := validateManualGroupRates(req.Selection.Groups); err != nil {
		return err
	}

	effectiveStart := now
	switch req.Schedule.StartMode {
	case StartNow, StartDraft:
		// now: 立即执行，effectiveStart 就是 now；draft: 尚未确定开始时间，以 now 作为兜底基线。
	case StartScheduled:
		if req.Schedule.StartAt == nil || !req.Schedule.StartAt.After(now) {
			return ErrInvalidSchedule
		}
		effectiveStart = *req.Schedule.StartAt
	default:
		return ErrInvalidSchedule
	}

	switch req.Schedule.EndMode {
	case EndManual:
		// 手动结束，无需 endAt。
	case EndScheduled:
		if req.Schedule.EndAt == nil || !req.Schedule.EndAt.After(effectiveStart) {
			return ErrInvalidSchedule
		}
	default:
		return ErrInvalidSchedule
	}

	if req.Notify.Enabled && len(req.Notify.BotIDs) == 0 {
		return ErrNoNotifyBots
	}
	return nil
}

// validateManualGroupRates 校验手动选择分组请求：分组名不能为空、不能重复，
// 且每个分组都必须提供合法的固定活动倍率（有限数字，且 >= 0）。
func validateManualGroupRates(groups []SelectionGroupRef) error {
	if len(groups) == 0 {
		return ErrEmptySelection
	}
	seen := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		name := strings.TrimSpace(g.GroupName)
		if name == "" {
			return ErrEmptySelection
		}
		if _, dup := seen[name]; dup {
			return ErrDuplicateGroup
		}
		seen[name] = struct{}{}
		if g.CampaignMultiplier == nil {
			return ErrInvalidAdjustment
		}
		value := *g.CampaignMultiplier
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return ErrInvalidAdjustment
		}
	}
	return nil
}

// renderTemplate 用 {varName} 占位符渲染通知文案，未知变量原样保留。
func renderTemplate(tpl string, vars map[string]string) string {
	if tpl == "" {
		return ""
	}
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(tpl)
}

// buildSummary 从活动的分组明细统计开启/恢复的成功与失败数量。
func buildSummary(items []CampaignItem) Summary {
	summary := Summary{Total: len(items)}
	for _, item := range items {
		switch item.ApplyStatus {
		case ItemApplied:
			summary.Applied++
		case ItemFailed:
			summary.ApplyFailed++
		}
		switch item.RestoreStatus {
		case ItemRestored, ItemUnchanged:
			summary.Restored++
		case ItemFailed:
			summary.RestoreFailed++
		}
	}
	return summary
}
