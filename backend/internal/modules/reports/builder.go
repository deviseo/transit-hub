package reports

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"transithub/backend/internal/modules/dashboard"
	"transithub/backend/internal/modules/profit"
	"transithub/backend/internal/modules/sales"
	"transithub/backend/internal/modules/user_activity"
	"transithub/backend/internal/shared/bizclock"
)

// SnapshotReader 读取仪表盘的每日快照（营收、成本、毛利、上游余额，均为人民币）。
type SnapshotReader interface {
	ListRange(ctx context.Context, userID, adminAccountID string, today time.Time, days int) ([]dashboard.DailySnapshot, error)
	IsFinalized(ctx context.Context, userID, adminAccountID string, date time.Time) (bool, error)
}

// SalesReader 读取销售分析（入账、入账用户）。
type SalesReader interface {
	Summary(ctx context.Context, ws sales.Workspace, query sales.RangeQuery) (sales.SummaryResponse, error)
}

// ActivityReader 读取用户分析概览（活跃、新注册、沉默、余额不足）。
type ActivityReader interface {
	Summary(ctx context.Context, ws user_activity.Workspace, seriesDays int) (user_activity.SummaryResponse, error)
}

// WorkspaceNamer 返回工作区的显示名称（用于日报标题）。
type WorkspaceNamer interface {
	WorkspaceName(ctx context.Context, userID string, adminAccountID string) string
}

// PriceChangeReader 读取利润分析记录的上游分组调价（含成本影响估算）。
type PriceChangeReader interface {
	PriceChangesBetween(ctx context.Context, ws profit.Workspace, since time.Time, until time.Time) ([]profit.PriceChange, error)
}

// VIPReader 生成日报里的【大户】一段（关注多少人、谁快用完、谁用完了）；没有关注的人时返回空。
type VIPReader interface {
	VIPSection(ctx context.Context, userID string, adminAccountID string) (string, error)
}

// Builder 用本地已有的数据生成经营日报，不访问站点。
type Builder struct {
	snapshots SnapshotReader
	sales     SalesReader
	activity  ActivityReader
	names     WorkspaceNamer
	prices    PriceChangeReader
	vip       VIPReader
}

func NewBuilder(snapshots SnapshotReader, sales SalesReader, activity ActivityReader, names WorkspaceNamer) *Builder {
	return &Builder{snapshots: snapshots, sales: sales, activity: activity, names: names}
}

// SetVIP 让日报加上【大户】一段。
func (b *Builder) SetVIP(reader VIPReader) {
	b.vip = reader
}

// SetPriceChanges 让日报的【上游】部分列出昨天的上游分组调价。
func (b *Builder) SetPriceChanges(reader PriceChangeReader) {
	b.prices = reader
}

// DailyReport 是一份经营日报（纯文本，各机器人渠道都能正常显示）。
type DailyReport struct {
	Date      string `json:"date"`
	Workspace string `json:"workspace"`
	Text      string `json:"text"`
}

const runwayWarningDays = 3

var channelNames = map[string]string{
	sales.ChannelPayment:   "在线充值",
	sales.ChannelRedeem:    "兑换码",
	sales.ChannelAdminAdd:  "管理员加款",
	sales.ChannelAffiliate: "邀请返利转入",
	sales.ChannelCredit:    "余额增加",
}

var weekdayNames = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// Build 生成 now 所在业务日的前一天（昨天）的日报。
func (b *Builder) Build(ctx context.Context, userID string, adminAccountID string, now time.Time) (DailyReport, error) {
	local := now.In(bizclock.Location())
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	date := yesterday.Format(bizclock.DateLayout)
	report := DailyReport{Date: date}
	if b.names != nil {
		report.Workspace = b.names.WorkspaceName(ctx, userID, adminAccountID)
	}

	var sections []string
	business, upstreamLine, err := b.businessSection(ctx, userID, adminAccountID, today, yesterday)
	if err != nil {
		return DailyReport{}, err
	}
	if business != "" {
		sections = append(sections, business)
	}
	salesSection, err := b.salesSection(ctx, userID, adminAccountID, date)
	if err != nil {
		return DailyReport{}, err
	}
	if salesSection != "" {
		sections = append(sections, salesSection)
	}
	usersSection, err := b.usersSection(ctx, userID, adminAccountID)
	if err != nil {
		return DailyReport{}, err
	}
	if usersSection != "" {
		sections = append(sections, usersSection)
	}
	if b.vip != nil {
		vipSection, err := b.vip.VIPSection(ctx, userID, adminAccountID)
		if err != nil {
			return DailyReport{}, err
		}
		if vipSection != "" {
			sections = append(sections, vipSection)
		}
	}
	priceLines, err := b.priceChangeLines(ctx, userID, adminAccountID, yesterday)
	if err != nil {
		return DailyReport{}, err
	}
	if len(priceLines) > 0 {
		if upstreamLine == "" {
			upstreamLine = "【上游】"
		}
		upstreamLine += "\n" + strings.Join(priceLines, "\n")
	}
	if upstreamLine != "" {
		sections = append(sections, upstreamLine)
	}

	header := fmt.Sprintf("📊 经营日报 · %s（%s）", date, weekdayNames[yesterday.Weekday()])
	if report.Workspace != "" {
		header += "\n工作区：" + report.Workspace
	}
	if len(sections) == 0 {
		sections = append(sections, "暂无可汇总的数据：请先在仪表盘登录自己的站点，登录后「用户分析」和「利润分析」会在后台自动采集。")
	}
	report.Text = header + "\n\n" + strings.Join(sections, "\n\n")
	return report, nil
}

// businessSection 汇总昨天的营收、成本、毛利（来自仪表盘快照）以及上游余额与续航。
func (b *Builder) businessSection(ctx context.Context, userID, adminAccountID string, today, yesterday time.Time) (string, string, error) {
	if b.snapshots == nil {
		return "", "", nil
	}
	snapshots, err := b.snapshots.ListRange(ctx, userID, adminAccountID, today, 8)
	if err != nil {
		return "", "", err
	}
	byDate := map[string]dashboard.DailySnapshot{}
	for _, snapshot := range snapshots {
		byDate[snapshot.Date.Format(bizclock.DateLayout)] = snapshot
	}
	current, ok := byDate[yesterday.Format(bizclock.DateLayout)]
	if !ok {
		return "", "", nil
	}
	finalized, err := b.snapshots.IsFinalized(ctx, userID, adminAccountID, yesterday)
	if err != nil {
		return "", "", err
	}

	title := "【经营】"
	if !finalized {
		title = "【经营】（数据未定稿，可能不完整）"
	}
	revenue := "营收 " + formatCNY(current.TodayProfit)
	if previous, ok := byDate[yesterday.AddDate(0, 0, -1).Format(bizclock.DateLayout)]; ok && previous.TodayProfit > 0 {
		revenue += fmt.Sprintf("（较前日 %s）", formatSignedPercent((current.TodayProfit-previous.TodayProfit)/previous.TodayProfit))
	}
	profit := fmt.Sprintf("成本 %s · 毛利 %s", formatCNY(current.TodayPurchase), formatCNY(current.NetProfit))
	if current.TodayProfit > 0 {
		profit += fmt.Sprintf("（毛利率 %s）", formatPercent(current.NetProfit/current.TodayProfit))
	}
	business := strings.Join([]string{title, revenue, profit}, "\n")

	// 续航：上游余额 ÷ 近 7 天（有快照的日期）日均成本。
	costTotal, costDays := 0.0, 0
	for offset := 0; offset < 7; offset++ {
		if snapshot, ok := byDate[yesterday.AddDate(0, 0, -offset).Format(bizclock.DateLayout)]; ok {
			costTotal += snapshot.TodayPurchase
			costDays++
		}
	}
	upstream := "【上游】\n上游余额 " + formatCNY(current.UpstreamBalance)
	if costDays > 0 && costTotal > 0 {
		average := costTotal / float64(costDays)
		runway := current.UpstreamBalance / average
		line := fmt.Sprintf("按近 %d 天日均成本 %s 估算，约可用 %s 天", costDays, formatCNY(average), formatDays(runway))
		if runway < runwayWarningDays {
			line = "⚠️ " + line + "，请尽快充值上游"
		}
		upstream += "\n" + line
	}
	return business, upstream, nil
}

// maxReportPriceChanges：日报里最多列出的调价条数。
const maxReportPriceChanges = 5

// priceChangeLines 列出昨天（北京时间）检测到的上游分组调价及成本影响。
func (b *Builder) priceChangeLines(ctx context.Context, userID, adminAccountID string, yesterday time.Time) ([]string, error) {
	if b.prices == nil {
		return nil, nil
	}
	since := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, bizclock.Location())
	changes, err := b.prices.PriceChangesBetween(ctx, profit.Workspace{UserID: userID, AdminAccountID: adminAccountID}, since, since.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	lines := []string{fmt.Sprintf("昨日上游调价 %d 处：", len(changes))}
	// 记录按时间倒序返回，日报按发生顺序列出。
	for i := len(changes) - 1; i >= 0 && len(lines) <= maxReportPriceChanges; i-- {
		change := changes[i]
		lines = append(lines, fmt.Sprintf("· %s / %s：%sx → %sx，%s", change.SiteName, change.GroupName,
			formatMultiplier(change.OldMultiplier), formatMultiplier(change.NewMultiplier), change.ShortImpactText()))
	}
	if len(changes) > maxReportPriceChanges {
		lines = append(lines, fmt.Sprintf("……另有 %d 处，详见利润分析", len(changes)-maxReportPriceChanges))
	}
	return lines, nil
}

func formatMultiplier(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (b *Builder) salesSection(ctx context.Context, userID, adminAccountID, date string) (string, error) {
	if b.sales == nil {
		return "", nil
	}
	summary, err := b.sales.Summary(ctx, sales.Workspace{UserID: userID, AdminAccountID: adminAccountID}, sales.RangeQuery{From: date, To: date})
	if err != nil {
		return "", err
	}
	if !summary.Coverage.Synced {
		return "", nil
	}
	title := "【销售】"
	if !summary.Coverage.RechargeComplete {
		title = "【销售】（入账数据不完整）"
	}
	current := summary.Current
	line := fmt.Sprintf("入账 %s · %d 人", formatCNY(current.Credits), current.Payers)
	if current.NewPayers != nil {
		line += fmt.Sprintf("（首次入账 %d 人）", *current.NewPayers)
	}
	lines := []string{title, line}
	parts := make([]string, 0)
	for _, channel := range summary.Channels {
		// 站长选择不计入的渠道不列出来（入账合计里也没有它）。
		if channel.Amount <= 0 || channel.Excluded {
			continue
		}
		name := channelNames[channel.Channel]
		if name == "" {
			name = channel.Channel
		}
		parts = append(parts, name+" "+formatCNY(channel.Amount))
	}
	if len(parts) > 0 {
		lines = append(lines, strings.Join(parts, " · "))
	}
	return strings.Join(lines, "\n"), nil
}

func (b *Builder) usersSection(ctx context.Context, userID, adminAccountID string) (string, error) {
	if b.activity == nil {
		return "", nil
	}
	summary, err := b.activity.Summary(ctx, user_activity.Workspace{UserID: userID, AdminAccountID: adminAccountID}, 7)
	if err != nil {
		return "", err
	}
	if !summary.HasData {
		return "", nil
	}
	lines := []string{"【用户】"}
	if day, ok := summary.Periods["day"]; ok {
		line := fmt.Sprintf("活跃 %d 人", day.Current.ActiveUsers)
		if day.Previous != nil && day.Previous.ActiveUsers > 0 {
			line += fmt.Sprintf("（较前日 %s）", formatSignedPercent(float64(day.Current.ActiveUsers-day.Previous.ActiveUsers)/float64(day.Previous.ActiveUsers)))
		}
		separator := " · "
		if strings.HasSuffix(line, "）") {
			separator = "· "
		}
		line += fmt.Sprintf("%s新注册 %d 人", separator, day.Current.NewUsers)
		lines = append(lines, line)
	}
	inactive7 := -1
	for _, bucket := range summary.Inactive {
		if bucket.Days == 7 {
			inactive7 = bucket.Count
		}
	}
	reminder := make([]string, 0, 2)
	if inactive7 >= 0 {
		reminder = append(reminder, fmt.Sprintf("超过 7 天未使用 %d 人", inactive7))
	}
	reminder = append(reminder, fmt.Sprintf("近期活跃但余额不足 %d 人", summary.Money.LowBalanceActive))
	lines = append(lines, strings.Join(reminder, " · "))
	return strings.Join(lines, "\n"), nil
}

func formatCNY(value float64) string {
	negative := value < 0
	cents := int64(math.Round(math.Abs(value) * 100))
	whole := cents / 100
	digits := fmt.Sprintf("%d", whole)
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

func formatPercent(ratio float64) string {
	return fmt.Sprintf("%.1f%%", ratio*100)
}

func formatSignedPercent(ratio float64) string {
	if ratio > 0 {
		return "+" + formatPercent(ratio)
	}
	return formatPercent(ratio)
}

func formatDays(days float64) string {
	if days >= 100 {
		return fmt.Sprintf("%.0f", days)
	}
	return fmt.Sprintf("%.1f", days)
}
