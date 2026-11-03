package dashboard

import (
	"testing"
	"time"
)

// TestCalendarDateKeepsLocalCalendarDay 锁定趋势查询的「今天」取本地日历日：
// 北京时间 2026-11-02 01:30 对应 UTC 仍是 11-01，若按 UTC 取日期，会把昨天（11-01）的快照
// 当成「今天」排除掉，导致趋势图在每天 0–8 点缺少前一天的数据。
func TestCalendarDateKeepsLocalCalendarDay(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*60*60)

	got := calendarDate(time.Date(2026, 11, 2, 1, 30, 0, 0, shanghai))
	want := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("calendarDate = %s, want %s", got, want)
	}

	got = calendarDate(time.Date(2026, 11, 1, 23, 59, 59, 0, time.UTC))
	want = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("calendarDate(UTC input) = %s, want %s", got, want)
	}
}
