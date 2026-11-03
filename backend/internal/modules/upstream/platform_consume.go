package upstream

import (
	"net/url"
	"strings"
	"time"
)

// FetchSelfConsumeOnDate 读取当前登录账号在上游站点某一天（date 为 YYYY-MM-DD，按 timezone 切分自然日）的
// 实际消费（美元口径），供仪表盘历史快照定稿使用：直接按日期查询，不依赖缓存里「今日消费」的取值时间。
//   - sub2api：/api/v1/usage/stats（不带 api_key_id 即该账号全部 key 的合计，包括已删除的 key）；
//   - new-api：/api/log/self/stat?type=2（该账号的消费日志合计；上游关闭了消费日志时为 0）。
func (s *PlatformService) FetchSelfConsumeOnDate(session Session, date string, timezone string) (float64, error) {
	if !session.IsAuthenticated() {
		return 0, newRequestError(ErrorAuth, session.Platform)
	}
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		return 0, newRequestError(ErrorRequest, session.Platform)
	}
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), loc)
	if err != nil {
		return 0, newRequestError(ErrorRequest, session.Platform)
	}
	switch session.Platform {
	case PlatformNewAPI:
		start := day.Unix()
		end := day.AddDate(0, 0, 1).Unix() - 1
		statURL := session.BaseURL + "/api/log/self/stat?type=2&start_timestamp=" + strconvInt(start) + "&end_timestamp=" + strconvInt(end)
		response, err := s.httpClient.requestJSON(statURL, newAPIAuthOptions(session))
		if err != nil {
			return 0, err
		}
		value := quotaToUSDWithUnit(firstNumber(dataRecord(response.Payload), []string{"quota", "used_quota", "usedQuota"}), session.QuotaPerUnit)
		if value == nil {
			return 0, nil
		}
		return *value, nil
	case PlatformSub2API:
		values := url.Values{}
		values.Set("start_date", day.Format("2006-01-02"))
		values.Set("end_date", day.Format("2006-01-02"))
		values.Set("timezone", loc.String())
		response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/usage/stats?"+values.Encode(), requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType})
		if err != nil {
			return 0, err
		}
		return sub2APIUsageStatsCost(dataRecord(response.Payload)), nil
	default:
		return 0, newRequestError(ErrorNotFound, session.Platform)
	}
}
