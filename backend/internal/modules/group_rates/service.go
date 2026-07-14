package group_rates

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	return s.repository.EnsureSchema(ctx)
}

func (s *Service) SaveSiteSnapshot(ctx context.Context, userID string, adminAccountID string, siteID string, siteName string, sitePlatform string, groups []SnapshotGroup) error {
	now := time.Now()
	ownerID := strings.TrimSpace(userID)
	workspaceID := strings.TrimSpace(adminAccountID)
	if ownerID == "" || workspaceID == "" {
		return nil
	}
	trimmedSiteID := strings.TrimSpace(siteID)
	trimmedSiteName := strings.TrimSpace(siteName)

	// 构建经过验证和清洗的分组列表。
	type validatedGroup struct {
		groupID    string
		groupName  string
		platform   string
		groupType  string
		multiplier float64
	}
	var validated []validatedGroup
	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" || group.Multiplier == nil {
			continue
		}
		platform := strings.TrimSpace(sitePlatform)
		groupType := ""
		if group.Platform != nil {
			if gp := strings.TrimSpace(*group.Platform); gp != "" {
				groupType = gp
			}
		}
		if platform == "" {
			continue
		}
		validated = append(validated, validatedGroup{
			groupID:    strings.TrimSpace(group.ID),
			groupName:  name,
			platform:   platform,
			groupType:  groupType,
			multiplier: *group.Multiplier,
		})
	}

	// 获取该站点每个分组的最新快照，用于判断倍率是否变化以及检测已消失分组。
	existing, err := s.repository.LatestGroupKeysForSite(ctx, ownerID, workspaceID, trimmedSiteID)
	if err != nil {
		return err
	}
	existingMap := make(map[string]latestGroupKey, len(existing))
	for _, e := range existing {
		key := e.GroupID
		if key == "" {
			key = e.GroupName
		}
		existingMap[key] = e
	}

	// 拆分为"倍率未变 → 刷新时间戳"和"倍率变化或新分组 → 插入新行"两组。
	// 只有倍率真正变化时才写入新快照，使得 LEAD 窗口函数始终对比的是上一次不同的倍率，
	// 涨跌幅会持续展示直到发生下一次倍率变动，而非仅展示一个同步周期。
	var toInsert []snapshotRecord
	var toTouch []string
	incomingKeys := make(map[string]struct{}, len(validated))
	for _, g := range validated {
		key := g.groupID
		if key == "" {
			key = g.groupName
		}
		incomingKeys[key] = struct{}{}

		if prev, ok := existingMap[key]; ok && prev.Multiplier == g.multiplier && !prev.Deleted {
			toTouch = append(toTouch, prev.ID)
		} else {
			id, err := newSnapshotID()
			if err != nil {
				return err
			}
			toInsert = append(toInsert, snapshotRecord{
				ID:             id,
				UserID:         ownerID,
				AdminAccountID: workspaceID,
				SiteID:         trimmedSiteID,
				SiteName:       trimmedSiteName,
				GroupID:        g.groupID,
				GroupName:      g.groupName,
				Platform:       g.platform,
				Type:           g.groupType,
				Multiplier:     g.multiplier,
				CreatedAt:      now,
			})
		}
	}

	// 检测已消失的分组并标记为已删除。
	var toDelete []string
	for _, e := range existing {
		key := e.GroupID
		if key == "" {
			key = e.GroupName
		}
		if _, found := incomingKeys[key]; !found && !e.Deleted {
			toDelete = append(toDelete, e.ID)
		}
	}

	if err := s.repository.MarkDeleted(ctx, toDelete); err != nil {
		return err
	}
	if err := s.repository.TouchSnapshots(ctx, toTouch, trimmedSiteName, now); err != nil {
		return err
	}
	return s.repository.InsertSnapshots(ctx, toInsert)
}

func (s *Service) List(ctx context.Context, userID string, adminAccountID string, query ListQuery) (ListResult, error) {
	query = normalizeListQuery(query)
	records, err := s.repository.List(ctx, strings.TrimSpace(userID), strings.TrimSpace(adminAccountID), query)
	if err != nil {
		return ListResult{}, err
	}

	rows := make([]RateRow, 0, len(records.Items))
	for _, record := range records.Items {
		delta, deltaPercent := change(record.Multiplier, record.PreviousMultiplier)
		rows = append(rows, RateRow{
			SiteID:            record.SiteID,
			SiteName:          record.SiteName,
			GroupID:           record.GroupID,
			GroupName:         record.GroupName,
			Platform:          record.Platform,
			Type:              record.Type,
			Mapped:            record.Mapped,
			Deleted:           record.Deleted,
			CurrentMultiplier: record.Multiplier * record.RechargeRate,
			Delta:             delta,
			DeltaPercent:      deltaPercent,
			UpdatedAt:         record.CreatedAt,
		})
	}
	return ListResult{
		Items:      rows,
		Total:      records.Total,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalPages: totalPages(records.Total, query.PageSize),
		Types:      records.Types,
		Platforms:  records.Platforms,
	}, nil
}

func (s *Service) UpdateType(ctx context.Context, userID string, adminAccountID string, ref GroupRef, groupType string) error {
	return s.repository.UpdateType(ctx, strings.TrimSpace(userID), strings.TrimSpace(adminAccountID), normalizeGroupRef(ref), strings.TrimSpace(groupType))
}

// ListGroupNames 按 type/search/platform 从最新分组快照中筛出匹配的分组名集合，
// 供 group_rate_campaigns 模块（group_rate_campaigns.GroupTypeLookup）解析
// "按分组类型"/"当前筛选结果" 两种选择模式使用。
func (s *Service) ListGroupNames(ctx context.Context, userID string, adminAccountID string, search string, groupType string, platform string) ([]string, error) {
	return s.repository.ListDistinctGroupNames(ctx, strings.TrimSpace(userID), strings.TrimSpace(adminAccountID), search, groupType, platform)
}

func normalizeGroupRef(ref GroupRef) GroupRef {
	return GroupRef{SiteID: strings.TrimSpace(ref.SiteID), GroupName: strings.TrimSpace(ref.GroupName)}
}

func (s *Service) History(ctx context.Context, userID string, adminAccountID string, siteID string, groupName string, platform string) ([]HistoryRow, error) {
	records, err := s.repository.History(ctx, strings.TrimSpace(userID), strings.TrimSpace(adminAccountID), strings.TrimSpace(siteID), strings.TrimSpace(groupName), strings.TrimSpace(platform))
	if err != nil {
		return nil, err
	}

	rows := make([]HistoryRow, 0, len(records))
	for _, record := range records {
		delta, deltaPercent := change(record.Multiplier, record.PreviousMultiplier)
		rows = append(rows, HistoryRow{
			ID:                record.ID,
			SiteID:            record.SiteID,
			SiteName:          record.SiteName,
			GroupID:           record.GroupID,
			GroupName:         record.GroupName,
			Platform:          record.Platform,
			Type:              record.Type,
			Multiplier:        record.Multiplier,
			CurrentMultiplier: record.Multiplier * record.RechargeRate,
			Deleted:           record.Deleted,
			Delta:             delta,
			DeltaPercent:      deltaPercent,
			CreatedAt:         record.CreatedAt,
			UpdatedAt:         record.CreatedAt,
		})
	}
	return rows, nil
}

func normalizeListQuery(query ListQuery) ListQuery {
	query.Search = strings.TrimSpace(query.Search)
	query.Type = strings.TrimSpace(query.Type)
	query.Platform = strings.TrimSpace(query.Platform)
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 10
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	return query
}

func totalPages(total int, pageSize int) int {
	if total == 0 || pageSize <= 0 {
		return 0
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	return pages
}

func change(current float64, previous *float64) (*float64, *float64) {
	if previous == nil || *previous == 0 {
		return nil, nil
	}
	delta := current - *previous
	deltaPercent := delta / *previous * 100
	return &delta, &deltaPercent
}
