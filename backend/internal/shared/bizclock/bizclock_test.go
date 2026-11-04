package bizclock

import (
	"testing"
	"time"
)

func TestBusinessDayIgnoresProcessTimezone(t *testing.T) {
	// 2026-11-02 18:30 UTC 是北京时间 11-03 02:30：业务日期必须是 11-03。
	instant := time.Date(2026, 11, 2, 18, 30, 0, 0, time.UTC)
	if got := DateOf(instant); got != "2026-11-03" {
		t.Fatalf("DateOf = %s, want 2026-11-03", got)
	}
	if got := DateOf(instant.In(time.FixedZone("PST", -8*3600))); got != "2026-11-03" {
		t.Fatalf("DateOf must not depend on the input zone, got %s", got)
	}
	start := StartOfDay(instant)
	if start.Unix() != 1793635200 || start.Location() != Location() {
		t.Fatalf("StartOfDay = %v (%d), want 2026-11-03 00:00 +08:00 (1793635200)", start, start.Unix())
	}
	from, to := DayBounds(instant)
	if from != 1793635200 || to != 1793721599 {
		t.Fatalf("DayBounds = %d..%d", from, to)
	}
	parsed, err := ParseDate("2026-11-03")
	if err != nil || !parsed.Equal(start) {
		t.Fatalf("ParseDate = %v %v", parsed, err)
	}
	if _, err := ParseDate("11/03"); err == nil {
		t.Fatal("invalid dates must be rejected")
	}
	if Now().Location() != Location() {
		t.Fatal("Now must be in the business timezone")
	}
}
