package upstream

import (
	"context"
	"log"
)

// SiteGroupConsumes 是一个上游站点上若干分组在一段时间内的消费（上游额度，未乘充值倍率），用于估算调价影响。
type SiteGroupConsumes struct {
	SiteID       string
	SiteName     string
	BaseURL      string
	RechargeRate float64
	// Consumes 以 GroupKey 为键；查询失败（或无法按分组查询）的分组不在其中。
	Consumes map[string]float64
}

// GroupKey 是分组在倍率比较和分组消费结果中的键（分组 ID + 名称）。
func GroupKey(group GroupInfo) string { return group.ID + "|" + group.Name }

// GroupConsumesInRange 查询站点上各分组在 [startDate, endDate]（按 timezone 切分自然日）的消费合计。
// 站点不存在或不属于该用户时返回错误；会话刷新或单个分组查询失败只记录日志，对应分组不出现在结果中。
func (s *Service) GroupConsumesInRange(ctx context.Context, userID string, siteID string, groups []GroupInfo, startDate string, endDate string, timezone string) (SiteGroupConsumes, error) {
	site, err := s.cache.Get(ctx, siteID)
	if err != nil {
		return SiteGroupConsumes{}, err
	}
	if site == nil || site.UserID != userID {
		return SiteGroupConsumes{}, newRequestError(ErrorNotFound, "")
	}
	result := SiteGroupConsumes{SiteID: site.ID, SiteName: site.Name, BaseURL: site.BaseURL, RechargeRate: site.RechargeRate, Consumes: map[string]float64{}}
	if site.Session == nil || len(groups) == 0 {
		return result, nil
	}
	session := *site.Session
	refreshed, err := s.platformService.RefreshSession(session)
	if err != nil {
		log.Printf("[upstream] group consume: refresh session failed site_id=%s err=%v", site.ID, err)
		return result, nil
	}
	s.writeBackRefreshedSession(ctx, site.ID, site.UserID, session, refreshed)
	for _, group := range groups {
		amount, err := s.platformService.FetchSelfGroupConsume(refreshed, group, startDate, endDate, timezone)
		if err != nil {
			log.Printf("[upstream] group consume failed site_id=%s group=%s err=%v", site.ID, group.Name, err)
			continue
		}
		result.Consumes[GroupKey(group)] = amount
	}
	return result, nil
}
