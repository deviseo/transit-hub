package profit

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/bizclock"
)

const (
	// priceImpactWindowDays 是估算调价影响所用的天数：截至昨天的完整自然日。
	priceImpactWindowDays = 7
	// maxPriceEstimates：一次同步里最多为这么多个分组查询用量（上游整体调价时避免请求过多），其余只记录不估算。
	maxPriceEstimates = 20
	// priceChangeListDays / priceChangeListLimit：利润页列出的调价记录范围。
	priceChangeListDays  = 30
	priceChangeListLimit = 50
	// priceInsightDays 内的涨价会出现在提示里。
	priceInsightDays = 7
)

// AssessPriceChanges 估算一个上游站点上分组调价对每天成本的影响并记录下来，返回的结果与 changes 一一对应。
// 只读：按分组查询自己账号在上游近 priceImpactWindowDays 天的消费，不会修改任何价格。
// 站点不存在时返回 nil（不记录）。
func (s *Service) AssessPriceChanges(ctx context.Context, ws Workspace, siteID string, changes []PriceChangeInput) []PriceChange {
	if len(changes) == 0 || s.upstreams == nil {
		return nil
	}
	now := s.now()
	today := calendarDay(now)
	from, to := today.AddDate(0, 0, -priceImpactWindowDays), today.AddDate(0, 0, -1)
	groups := make([]upstream.GroupInfo, 0, len(changes))
	for _, change := range changes {
		if change.OldMultiplier > 0 && len(groups) < maxPriceEstimates {
			groups = append(groups, change.Group)
		}
	}
	consumes, err := s.upstreams.GroupConsumesInRange(ctx, ws.UserID, siteID, groups, formatDay(from), formatDay(to), bizclock.Name)
	if err != nil {
		log.Printf("[profit] price change: load site failed admin_account_id=%s site_id=%s err=%v", ws.AdminAccountID, siteID, err)
		return nil
	}
	results := make([]PriceChange, 0, len(changes))
	for _, change := range changes {
		record := estimatePriceChange(change, consumes)
		record.DetectedAt = now
		if s.store != nil {
			saved, err := s.store.InsertPriceChange(ctx, ws, record)
			if err != nil {
				log.Printf("[profit] save price change failed admin_account_id=%s site_id=%s err=%v", ws.AdminAccountID, siteID, err)
			} else {
				record = saved
			}
		}
		results = append(results, record)
	}
	return results
}

// estimatePriceChange 计算单个分组调价的影响：日均成本 = 窗口内消费 ÷ 天数 × 充值倍率，每天影响 = 日均成本 × (新 ÷ 旧 − 1)。
func estimatePriceChange(change PriceChangeInput, consumes upstream.SiteGroupConsumes) PriceChange {
	record := PriceChange{
		SiteID:        consumes.SiteID,
		SiteName:      consumes.SiteName,
		BaseURL:       consumes.BaseURL,
		UpstreamKey:   upstreamKey(consumes.BaseURL),
		GroupID:       change.Group.ID,
		GroupName:     change.Group.Name,
		OldMultiplier: change.OldMultiplier,
		NewMultiplier: change.NewMultiplier,
		RechargeRate:  consumes.RechargeRate,
		WindowDays:    priceImpactWindowDays,
	}
	consume, queried := consumes.Consumes[upstream.GroupKey(change.Group)]
	if queried {
		value := consume
		record.WindowConsume = &value
	}
	switch {
	case consumes.RechargeRate <= 0:
		record.EstimateStatus = EstimateNoRate
	case !queried || change.OldMultiplier <= 0 || math.IsNaN(consume) || math.IsInf(consume, 0):
		record.EstimateStatus = EstimateFailed
	case consume <= 0:
		record.EstimateStatus = EstimateNoUsage
		record.AvgDailyCost, record.DailyImpact = floatPtr(0), floatPtr(0)
	default:
		average := consume / priceImpactWindowDays * consumes.RechargeRate
		impact := average * (change.NewMultiplier/change.OldMultiplier - 1)
		record.EstimateStatus = EstimateOK
		record.AvgDailyCost, record.DailyImpact = &average, &impact
	}
	return record
}

// ImpactText 是调价影响的一句话说明（用于通知和日报）。
func (c PriceChange) ImpactText() string {
	switch c.EstimateStatus {
	case EstimateOK:
		if c.AvgDailyCost == nil || c.DailyImpact == nil {
			break
		}
		verb := "多"
		if *c.DailyImpact < 0 {
			verb = "省"
		}
		return fmt.Sprintf("按该分组近 %d 天日均成本 %s 估算，每天约%s %s", c.WindowDays, formatYuan(*c.AvgDailyCost), verb, formatYuan(math.Abs(*c.DailyImpact)))
	case EstimateNoUsage:
		return fmt.Sprintf("近 %d 天没有使用该分组，暂不影响成本", c.WindowDays)
	case EstimateNoRate:
		return "该上游站点未设置充值倍率，无法估算成本影响"
	}
	return "暂时无法估算（查询该分组的上游用量失败）"
}

// ShortImpactText 是调价影响的简短说明（用于日报）。
func (c PriceChange) ShortImpactText() string {
	switch c.EstimateStatus {
	case EstimateOK:
		if c.DailyImpact == nil {
			break
		}
		if *c.DailyImpact < 0 {
			return "每天约省 " + formatYuan(-*c.DailyImpact)
		}
		return "每天约多 " + formatYuan(*c.DailyImpact)
	case EstimateNoUsage:
		return fmt.Sprintf("近 %d 天未使用该分组", c.WindowDays)
	case EstimateNoRate:
		return "未设置充值倍率，无法估算"
	}
	return "影响暂无法估算"
}

// formatYuan 格式化人民币金额（千分位、两位小数）。
func formatYuan(value float64) string {
	negative := value < 0
	cents := int64(math.Round(math.Abs(value) * 100))
	digits := fmt.Sprintf("%d", cents/100)
	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	text := fmt.Sprintf("¥%s.%02d", grouped.String(), cents%100)
	if negative && cents > 0 {
		return "-" + text
	}
	return text
}

// recentPriceChanges 读取最近 priceChangeListDays 天的调价记录，最新的在前。
func (s *Service) recentPriceChanges(ctx context.Context, ws Workspace, now time.Time) ([]PriceChange, error) {
	return s.PriceChangesBetween(ctx, ws, now.Add(-priceChangeListDays*24*time.Hour), now.Add(time.Minute))
}

// PriceChangesBetween 返回 [since, until) 内检测到的调价记录（最新的在前，最多 priceChangeListLimit 条）。
func (s *Service) PriceChangesBetween(ctx context.Context, ws Workspace, since time.Time, until time.Time) ([]PriceChange, error) {
	changes, err := s.store.ListPriceChanges(ctx, ws, since, until, priceChangeListLimit)
	if err != nil {
		return nil, err
	}
	if changes == nil {
		changes = []PriceChange{}
	}
	for i := range changes {
		changes[i].UpstreamKey = upstreamKey(changes[i].BaseURL)
	}
	return changes, nil
}

// priceInsights：最近 priceInsightDays 天内的涨价（同一站点同一分组只看最新一次）；近期没有用量的分组不影响成本，不提示。
func priceInsights(changes []PriceChange, now time.Time) []Insight {
	insights := make([]Insight, 0)
	seen := map[string]bool{}
	cutoff := now.Add(-priceInsightDays * 24 * time.Hour)
	for _, change := range changes {
		key := change.SiteID + "|" + change.GroupID + "|" + change.GroupName
		if seen[key] || change.DetectedAt.Before(cutoff) {
			continue
		}
		seen[key] = true
		if change.OldMultiplier <= 0 || change.NewMultiplier <= change.OldMultiplier || change.EstimateStatus == EstimateNoUsage {
			continue
		}
		insights = append(insights, Insight{
			Kind:     InsightPriceIncrease,
			Severity: "warning",
			Subject:  change.SiteName + " / " + change.GroupName,
			Value:    change.DailyImpact,
			Extra:    floatPtr(change.NewMultiplier/change.OldMultiplier - 1),
		})
	}
	return insights
}
