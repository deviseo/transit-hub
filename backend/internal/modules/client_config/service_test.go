package client_config

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	rows map[Workspace]Config
}

func (m *memoryStore) Get(_ context.Context, ws Workspace) (*Config, error) {
	if row, ok := m.rows[ws]; ok {
		return &row, nil
	}
	return nil, nil
}

func (m *memoryStore) GetByCode(_ context.Context, code string) (*Config, error) {
	for _, row := range m.rows {
		if row.Code == code {
			found := row
			return &found, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) Save(_ context.Context, config Config) error {
	m.rows[config.Workspace] = config
	return nil
}

type fakeSites struct{}

func (fakeSites) RequireCurrentID(_ context.Context, userID string) (string, error) {
	if userID == "nobody" {
		return "", errors.New("none")
	}
	return "acct-1", nil
}

func (fakeSites) Site(_ context.Context, _ string, adminAccountID string) (SiteInfo, error) {
	if adminAccountID != "acct-1" {
		return SiteInfo{}, errors.New("deleted")
	}
	return SiteInfo{Platform: "newapi", BaseURL: "https://relay.example.com"}, nil
}

type fakeGroups struct{}

func (fakeGroups) Groups(context.Context, string, string) ([]Group, error) {
	return []Group{{ID: "claude", Name: "claude"}}, nil
}

func newTestService() (*Service, *memoryStore) {
	store := &memoryStore{rows: map[Workspace]Config{}}
	service := NewService(store, fakeSites{}, fakeGroups{})
	codes := []string{"abcdefghij", "kmnpqrstuv"}
	service.newCode = func() string {
		code := codes[0]
		codes = codes[1:]
		return code
	}
	service.now = func() time.Time { return time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC) }
	return service, store
}

var ws = Workspace{UserID: "user-1", AdminAccountID: "acct-1"}

func dataURL(contentType string, size int) string {
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(make([]byte, size))
}

func TestDefaultsBeforeFirstSave(t *testing.T) {
	service, _ := newTestService()
	view, err := service.Get(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	if view.Enabled || view.Code != "" || view.ConfigPath != "" {
		t.Fatalf("unsaved config should be off without a code: %+v", view)
	}
	if view.Platform != "newapi" || view.Support == nil {
		t.Fatalf("site info missing: %+v", view)
	}
	if _, err := service.CurrentWorkspace(context.Background(), "nobody"); err == nil || err.Error() != ErrorNoCurrentAccount {
		t.Fatalf("expected no current account, got %v", err)
	}
}

func TestSaveGeneratesCodeAndPublishes(t *testing.T) {
	service, _ := newTestService()
	ctx := context.Background()
	view, err := service.Update(ctx, ws, UpdateRequest{
		Enabled: true,
		HubURL:  " https://hub.example.com/transit/ ",
		Brand:   Brand{Name: "  极速中转 ", Color: "#2563EB", Logo: dataURL("image/png", 16), Welcome: "欢迎"},
		Recommend: Recommend{
			ClaudeCode: ToolRecommend{Group: "claude"},
			Codex:      ToolRecommend{Group: "codex", Model: "gpt-6.1-sol"},
		},
		Notice:  "本周六维护",
		Support: []SupportLink{{Type: "telegram", Label: "TG 群", Value: "https://t.me/x"}, {Type: "qq", Label: "", Value: ""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Code != "abcdefghij" || view.ConfigPath != "/api/client/v1/sites/abcdefghij" {
		t.Fatalf("code not generated: %+v", view)
	}
	if view.HubURL != "https://hub.example.com/transit" || view.Brand.Name != "极速中转" || view.Brand.Color != "#2563eb" {
		t.Fatalf("values not normalized: %+v", view)
	}
	if len(view.Support) != 1 || view.NoticeUpdatedAt == nil {
		t.Fatalf("support or notice time wrong: %+v", view)
	}

	public, err := service.Public(ctx, "abcdefghij")
	if err != nil {
		t.Fatal(err)
	}
	if public.Site.BaseURL != "https://relay.example.com" || public.Site.Platform != "newapi" || public.Version != 1 {
		t.Fatalf("site wrong: %+v", public.Site)
	}
	if public.Brand.Name != "极速中转" || public.Brand.LogoURL != "/api/client/v1/sites/abcdefghij/logo" {
		t.Fatalf("brand wrong: %+v", public.Brand)
	}
	if public.Recommend.Codex.Model != "gpt-6.1-sol" || public.Notice.Content != "本周六维护" {
		t.Fatalf("recommend or notice wrong: %+v", public)
	}
	contentType, data, err := service.Logo(ctx, "abcdefghij")
	if err != nil || contentType != "image/png" || len(data) != 16 {
		t.Fatalf("logo: %v %q %d", err, contentType, len(data))
	}

	// 公告没改：公告时间不变；改了：更新。
	firstNotice := *view.NoticeUpdatedAt
	service.now = func() time.Time { return firstNotice.Add(time.Hour) }
	again, err := service.Update(ctx, ws, UpdateRequest{Enabled: true, Brand: Brand{Name: "极速中转"}, Notice: "本周六维护"})
	if err != nil {
		t.Fatal(err)
	}
	if !again.NoticeUpdatedAt.Equal(firstNotice) || again.Code != "abcdefghij" {
		t.Fatalf("unchanged notice keeps its time and the code: %+v", again)
	}
	changed, _ := service.Update(ctx, ws, UpdateRequest{Enabled: true, Notice: "维护结束"})
	if !changed.NoticeUpdatedAt.After(firstNotice) {
		t.Fatalf("changed notice gets a new time")
	}
	// 没设品牌名称时留空（TransitLink 显示站点自己的名称）；站点名称只用对外地址的域名。
	_, _ = service.Update(ctx, ws, UpdateRequest{Enabled: true, SiteURL: "https://api.relay.example.com/v1"})
	public, _ = service.Public(ctx, "abcdefghij")
	if public.Brand.Name != "" || public.Site.Name != "api.relay.example.com" || public.Site.BaseURL != "https://api.relay.example.com" || public.Brand.LogoURL != "" {
		t.Fatalf("fallbacks wrong: %+v", public)
	}
}

func TestResetCodeAndDisabledConfigsAreHidden(t *testing.T) {
	service, _ := newTestService()
	ctx := context.Background()
	_, _ = service.Update(ctx, ws, UpdateRequest{Enabled: true})
	view, err := service.ResetCode(ctx, ws)
	if err != nil || view.Code != "kmnpqrstuv" {
		t.Fatalf("reset: %v %+v", err, view)
	}
	if _, err := service.Public(ctx, "abcdefghij"); err == nil || err.Error() != ErrorNotFound {
		t.Fatalf("old code should stop working, got %v", err)
	}
	if _, err := service.Public(ctx, "kmnpqrstuv"); err != nil {
		t.Fatalf("new code works: %v", err)
	}
	_, _ = service.Update(ctx, ws, UpdateRequest{Enabled: false})
	if _, err := service.Public(ctx, "kmnpqrstuv"); err == nil || err.Error() != ErrorNotFound {
		t.Fatalf("disabled config is hidden, got %v", err)
	}
	for _, code := range []string{"", "short", "ABCDEFGHIJ", "abcdefghi0", strings.Repeat("a", 11)} {
		if _, err := service.Public(ctx, code); err == nil || err.Error() != ErrorNotFound {
			t.Fatalf("code %q should be rejected", code)
		}
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]struct {
		req  UpdateRequest
		want string
	}{
		"bad color":       {UpdateRequest{Brand: Brand{Color: "blue"}}, ErrorInvalid},
		"long name":       {UpdateRequest{Brand: Brand{Name: strings.Repeat("名", 41)}}, ErrorInvalid},
		"http logo":       {UpdateRequest{Brand: Brand{Logo: "http://cdn.example.com/logo.png"}}, ErrorInvalid},
		"svg logo":        {UpdateRequest{Brand: Brand{Logo: "data:image/svg+xml;base64,PHN2Zz4="}}, ErrorLogoType},
		"big logo":        {UpdateRequest{Brand: Brand{Logo: dataURL("image/png", maxLogoBytes+1)}}, ErrorLogoTooLarge},
		"bad hub url":     {UpdateRequest{HubURL: "ftp://hub.example.com"}, ErrorInvalid},
		"bad support":     {UpdateRequest{Support: []SupportLink{{Type: "fax", Label: "传真", Value: "123"}}}, ErrorInvalid},
		"support no link": {UpdateRequest{Support: []SupportLink{{Type: "qq", Label: "QQ 群"}}}, ErrorInvalid},
		"long notice":     {UpdateRequest{Notice: strings.Repeat("字", 2001)}, ErrorInvalid},
	}
	for name, tc := range cases {
		_, err := normalize(tc.req)
		if err == nil || err.Error() != tc.want {
			t.Errorf("%s: want %s, got %v", name, tc.want, err)
		}
	}
	tooMany := make([]SupportLink, maxSupportLinks+1)
	for i := range tooMany {
		tooMany[i] = SupportLink{Type: "url", Label: "链接", Value: "https://example.com"}
	}
	if _, err := normalize(UpdateRequest{Support: tooMany}); err == nil {
		t.Errorf("too many support links should be rejected")
	}
	if _, err := normalize(UpdateRequest{Brand: Brand{Logo: "https://cdn.example.com/logo.png", Color: "#ABCDEF"}}); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestRandomCodesAreValid(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		code := randomCode()
		if !validCode(code) || seen[code] {
			t.Fatalf("bad or repeated code %q", code)
		}
		seen[code] = true
	}
}
