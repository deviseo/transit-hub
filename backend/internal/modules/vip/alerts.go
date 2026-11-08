package vip

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"transithub/backend/internal/modules/settings"
)

const (
	// alertInterval：多久检查一次（余额每小时同步一次，15 分钟能在同步后很快提醒）。
	alertInterval = 15 * time.Minute
	// renotifyAfter：同一个人一天最多提醒一次。
	renotifyAfter = 24 * time.Hour
	// maxAlertLines：一条提醒消息里最多列几个人。
	maxAlertLines = 10
	// maxBriefNames：日报里每类最多列几个人。
	maxBriefNames = 3
)

// Notifier 用指定工作区的机器人发送通知。
type Notifier interface {
	SendFormattedToWorkspaceBots(ctx context.Context, userID string, adminAccountID string, botIDs []string, message string, format settings.NotificationTemplateFormat)
}

// StartScheduler 每 15 分钟检查一次打开了提醒的工作区：大户快用完、用完了时提醒站长。
func (s *Service) StartScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(alertInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runAlerts(ctx)
			}
		}
	}()
}

func (s *Service) runAlerts(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[vip] alert check panic recovered: %v", r)
		}
	}()
	workspaces, err := s.store.AlertWorkspaces(ctx)
	if err != nil {
		log.Printf("[vip] load alert workspaces failed: %v", err)
		return
	}
	for _, ws := range workspaces {
		if err := s.alertWorkspace(ctx, ws); err != nil {
			log.Printf("[vip] alert check failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
		}
	}
}

// alertWorkspace 检查一个工作区：该提醒的发一条消息（列出这次要提醒的人），记下提醒过的和回升了的。
// 上次检查之后没有新的同步就跳过（数据没变）。
func (s *Service) alertWorkspace(ctx context.Context, ws Workspace) error {
	state, synced, err := s.store.SyncState(ctx, ws)
	if err != nil || !synced {
		return err
	}
	key := ws.UserID + "|" + ws.AdminAccountID
	if state.LastSyncedAt != nil {
		if last, ok := s.checked.Load(key); ok && last.(time.Time).Equal(*state.LastSyncedAt) {
			return nil
		}
	}
	summary, err := s.Summary(ctx, ws)
	if err != nil || !summary.Synced || !summary.Settings.AlertsEnabled || len(summary.Settings.BotIDs) == 0 {
		return err
	}
	if state.LastSyncedAt != nil {
		defer s.checked.Store(key, *state.LastSyncedAt)
	}
	states, err := s.store.AlertStates(ctx, ws)
	if err != nil {
		return err
	}
	now := s.now()
	notify := make([]Row, 0)
	notified := map[string]string{}
	recovered := make([]string, 0)
	for _, row := range summary.Items {
		previous, seen := states[row.SiteUserID]
		var last *alertState
		if seen {
			last = &previous
		}
		urgent := row.Status == StatusLow || row.Status == StatusEmpty
		if !urgent {
			if last != nil && !last.Recovered {
				recovered = append(recovered, row.SiteUserID)
			}
			continue
		}
		// 不在用的人（例如很久以前就用完了）不打扰。
		if !active(row, now) {
			continue
		}
		if shouldNotify(row.Status, last, now) {
			notify = append(notify, row)
			notified[row.SiteUserID] = row.Status
		}
	}
	if len(notify) > 0 {
		workspace := ""
		if s.names != nil {
			workspace = s.names.WorkspaceName(ctx, ws.UserID, ws.AdminAccountID)
		}
		s.notifier.SendFormattedToWorkspaceBots(ctx, ws.UserID, ws.AdminAccountID, summary.Settings.BotIDs,
			alertMessage(workspace, notify), settings.NotificationTemplateFormatText)
		log.Printf("[vip] alerted admin_account_id=%s users=%d bots=%d", ws.AdminAccountID, len(notify), len(summary.Settings.BotIDs))
	}
	if len(notified) == 0 && len(recovered) == 0 {
		return nil
	}
	return s.store.SaveAlertStates(ctx, ws, notified, now, recovered)
}

// shouldNotify：第一次快用完或用完了提醒；从快用完变成用完了再提醒一次；充值回升过、又降下来的，
// 离上次提醒满一天再提醒。一直没回升的不重复提醒（每日报告里会列出来）。
func shouldNotify(current string, last *alertState, now time.Time) bool {
	if current != StatusLow && current != StatusEmpty {
		return false
	}
	if last == nil {
		return true
	}
	if !last.Recovered {
		return current == StatusEmpty && last.State == StatusLow
	}
	return now.Sub(last.NotifiedAt) >= renotifyAfter
}

// displayName 是大户在消息里的名字：邮箱，其次用户名，再其次站点用户 ID。
func displayName(row Row) string {
	switch {
	case row.Email != "":
		return row.Email
	case row.Username != "":
		return row.Username
	default:
		return "#" + row.SiteUserID
	}
}

func formatCNY(value float64) string {
	return fmt.Sprintf("¥%.2f", value)
}

// daysText 是「不到 1 天」「约 1.5 天」「约 12 天」。
func daysText(days float64) string {
	switch {
	case days < 1:
		return "不到 1 天"
	case days < 10:
		// 一位小数，整数时不带「.0」：约 1.5 天、约 3 天。
		return "约 " + strconv.FormatFloat(math.Floor(days*10)/10, 'f', -1, 64) + " 天"
	default:
		return fmt.Sprintf("约 %.0f 天", math.Floor(days))
	}
}

// alertMessage 是发给站长的提醒（纯文本，各机器人渠道都能正常显示）。
func alertMessage(workspace string, rows []Row) string {
	lines := []string{"👑 大户余额提醒"}
	if workspace != "" {
		lines = append(lines, "工作区："+workspace)
	}
	for i, row := range rows {
		if i >= maxAlertLines {
			lines = append(lines, fmt.Sprintf("……另有 %d 人", len(rows)-maxAlertLines))
			break
		}
		var line string
		if row.Status == StatusEmpty {
			line = fmt.Sprintf("⛔ %s 余额用完了", displayName(row))
			if row.DailyAverage > 0 {
				line += fmt.Sprintf("（近 7 天日均 %s）", formatCNY(row.DailyAverage))
			}
		} else {
			line = "⚠️ " + displayName(row)
			if row.Balance != nil {
				line += " 余额 " + formatCNY(*row.Balance)
			}
			if row.DailyAverage > 0 && row.DaysLeft != nil {
				line += fmt.Sprintf("，近 7 天日均 %s，预计%s用完", formatCNY(row.DailyAverage), daysText(*row.DaysLeft))
			}
		}
		if row.Source.Note != "" {
			line += "（" + row.Source.Note + "）"
		}
		lines = append(lines, line)
	}
	lines = append(lines, "详情见 TransitHub「大户关注」。")
	return strings.Join(lines, "\n")
}

// Brief 是日报里的【大户】：关注多少人、谁快用完了、谁用完了。
type Brief struct {
	Synced  bool
	Watched int
	Low     []string
	Empty   []string
}

// Brief 给每日经营报告用（只算在用的人，和提醒的口径一致）。
func (s *Service) Brief(ctx context.Context, ws Workspace) (Brief, error) {
	summary, err := s.Summary(ctx, ws)
	if err != nil || !summary.Synced {
		return Brief{}, err
	}
	brief := Brief{Synced: true, Watched: summary.Counts.All, Low: []string{}, Empty: []string{}}
	now := s.now()
	for _, row := range summary.Items {
		if !active(row, now) {
			continue
		}
		switch row.Status {
		case StatusLow:
			name := displayName(row)
			if row.DaysLeft != nil && row.DailyAverage > 0 {
				name += "（" + daysText(*row.DaysLeft) + "）"
			}
			brief.Low = append(brief.Low, name)
		case StatusEmpty:
			brief.Empty = append(brief.Empty, displayName(row))
		}
	}
	return brief, nil
}

// ReportSection 是日报里的【大户】一段；没有关注的人时为空。
func (b Brief) ReportSection() string {
	if !b.Synced || b.Watched == 0 {
		return ""
	}
	if len(b.Low) == 0 && len(b.Empty) == 0 {
		return fmt.Sprintf("【大户】关注 %d 人，余额都还够用", b.Watched)
	}
	lines := []string{fmt.Sprintf("【大户】关注 %d 人 · 快用完 %d 人 · 已用完 %d 人", b.Watched, len(b.Low), len(b.Empty))}
	list := func(names []string) string {
		if len(names) <= maxBriefNames {
			return strings.Join(names, "、")
		}
		return strings.Join(names[:maxBriefNames], "、") + fmt.Sprintf(" 等 %d 人", len(names))
	}
	if len(b.Low) > 0 {
		lines = append(lines, "⚠️ 快用完："+list(b.Low))
	}
	if len(b.Empty) > 0 {
		lines = append(lines, "⛔ 已用完："+list(b.Empty))
	}
	return strings.Join(lines, "\n")
}
