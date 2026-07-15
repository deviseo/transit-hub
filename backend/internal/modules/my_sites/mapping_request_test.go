package my_sites

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"transithub/backend/internal/shared/httpjson"
)

func TestMappingRequestAcceptsReturnedReadOnlyFields(t *testing.T) {
	runAt := time.Date(2026, time.July, 12, 18, 0, 0, 0, time.UTC)
	body, err := json.Marshal(struct {
		Mappings []GroupMapping `json:"mappings"`
	}{Mappings: []GroupMapping{{
		OwnGroup:        "vip",
		UpstreamTargets: []UpstreamGroupRef{{SiteID: "site-1", GroupName: "pro"}},
		LastAutoPricingRun: &AutoPricingRunStatus{
			Status:  "applied",
			Trigger: "manual",
			RanAt:   runAt,
		},
	}}})
	if err != nil {
		t.Fatalf("marshal response mapping: %v", err)
	}

	request := httptest.NewRequest(http.MethodPut, "/api/my-sites/mappings", bytes.NewReader(body))
	var payload struct {
		Mappings []MappingRequest `json:"mappings"`
	}
	if err := httpjson.Decode(request, &payload); err != nil {
		t.Fatalf("decode returned mapping for save: %v", err)
	}
	if len(payload.Mappings) != 1 || payload.Mappings[0].OwnGroup != "vip" {
		t.Fatalf("unexpected decoded mappings: %+v", payload.Mappings)
	}
}
