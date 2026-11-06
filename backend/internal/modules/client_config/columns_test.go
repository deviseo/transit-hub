package client_config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// 加工具时 Recommend 的字段、fields() 和列名前缀要一起加，少一个存库就会错位。
func TestRecommendColumnsMatchTools(t *testing.T) {
	var recommend Recommend
	if fields := len(recommend.fields()); fields != reflect.TypeOf(recommend).NumField() || fields != len(recommendColumns) {
		t.Fatalf("Recommend has %d fields, fields() lists %d, %d column prefixes",
			reflect.TypeOf(recommend).NumField(), fields, len(recommendColumns))
	}
	if len(saveColumns) != 16+2*len(recommendColumns) {
		t.Fatalf("save columns: %v", saveColumns)
	}
	last := fmt.Sprintf("$%d::jsonb, now(), now())", len(saveColumns))
	if !strings.Contains(upsertSQL, last) || strings.Contains(upsertSQL, "user_id = EXCLUDED") ||
		!strings.Contains(upsertSQL, "openclaw_model = EXCLUDED.openclaw_model") {
		t.Fatalf("upsert SQL: %s", upsertSQL)
	}
	if !strings.HasSuffix(selectColumns, "ticket_categories, updated_at") {
		t.Fatalf("select columns: %s", selectColumns)
	}
}
