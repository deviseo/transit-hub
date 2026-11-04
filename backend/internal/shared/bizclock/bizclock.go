// Package bizclock 定义 TransitHub 的业务时区：所有「今日」「某一天」的统计都按北京时间切分，
// 与服务进程或数据库会话的时区无关（开发机、未设置 TZ 的容器都能得到同样的日期边界）。
package bizclock

import "time"

// Name 是业务时区的 IANA 名称，也用作调用 sub2api 等平台接口时的 timezone 参数。
const Name = "Asia/Shanghai"

// DateLayout 是业务日期的字符串格式（YYYY-MM-DD）。
const DateLayout = "2006-01-02"

var location = loadLocation()

func loadLocation() *time.Location {
	loc, err := time.LoadLocation(Name)
	if err != nil {
		// 精简镜像可能缺少 tzdata。北京时间没有夏令时，固定 +08:00 与 Asia/Shanghai 等价。
		return time.FixedZone(Name, 8*3600)
	}
	return loc
}

// Location 返回业务时区。
func Location() *time.Location { return location }

// Now 返回业务时区下的当前时间。
func Now() time.Time { return time.Now().In(location) }

// DateOf 返回 t 所在的业务日期（YYYY-MM-DD）。
func DateOf(t time.Time) string { return t.In(location).Format(DateLayout) }

// Today 返回当前的业务日期（YYYY-MM-DD）。
func Today() string { return DateOf(time.Now()) }

// StartOfDay 返回 t 所在业务日的 00:00。
func StartOfDay(t time.Time) time.Time {
	t = t.In(location)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, location)
}

// DayBounds 返回 t 所在业务日的 [00:00:00, 23:59:59] 两端的 Unix 秒。
func DayBounds(t time.Time) (int64, int64) {
	start := StartOfDay(t)
	return start.Unix(), start.AddDate(0, 0, 1).Unix() - 1
}

// ParseDate 把 YYYY-MM-DD 解析为该业务日的 00:00。
func ParseDate(date string) (time.Time, error) {
	return time.ParseInLocation(DateLayout, date, location)
}
