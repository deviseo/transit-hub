package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeStrategySettingsKeepsLegacyTemplatesAsText(t *testing.T) {
	settings := normalizeStrategySettings(StrategySettings{
		RefreshInterval:          minRefreshIntervalSeconds,
		BalanceTemplateFormat:    "unsupported",
		MultiplierTemplateFormat: "",
	})
	if settings.BalanceTemplateFormat != NotificationTemplateFormatText {
		t.Fatalf("expected invalid balance format to fall back to text, got %q", settings.BalanceTemplateFormat)
	}
	if settings.MultiplierTemplateFormat != NotificationTemplateFormatText {
		t.Fatalf("expected missing multiplier format to fall back to text, got %q", settings.MultiplierTemplateFormat)
	}
}

func TestHTMLNotificationConversionPreservesFormattingAndDropsScripts(t *testing.T) {
	source := `<h2>余额预警</h2><p><strong>余额</strong>：8.50</p><script>alert(1)</script>`
	markdown, err := htmlToMarkdown(source)
	if err != nil {
		t.Fatalf("convert notification HTML to Markdown: %v", err)
	}
	if !strings.Contains(markdown, "## 余额预警") || !strings.Contains(markdown, "**余额**") {
		t.Fatalf("expected headings and emphasis in converted Markdown, got %q", markdown)
	}
	if strings.Contains(markdown, "alert(1)") || strings.Contains(markdown, "script") {
		t.Fatalf("script content must not be included in converted Markdown: %q", markdown)
	}

	telegramHTML := telegramHTMLForChannel(`<p onclick="alert(1)"><strong>余额</strong> <a href="https://example.com">详情</a><a href="javascript:alert(1)">危险</a></p>`)
	if !strings.Contains(telegramHTML, "<b>余额</b>") || !strings.Contains(telegramHTML, `<a href="https://example.com">详情</a>`) {
		t.Fatalf("expected supported Telegram HTML formatting, got %q", telegramHTML)
	}
	if strings.Contains(telegramHTML, "onclick") || strings.Contains(telegramHTML, "javascript:") {
		t.Fatalf("unsafe Telegram HTML attributes must be removed: %q", telegramHTML)
	}
}

func TestDingtalkHTMLNotificationUsesMarkdownPayload(t *testing.T) {
	var payload struct {
		MessageType string `json:"msgtype"`
		Markdown    struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"markdown"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewService(server.Client(), nil)
	err := service.sendDingtalkMessage(context.Background(), server.URL, "", notificationMessage{
		Content: `<h2>余额预警</h2><p><strong>8.50</strong></p>`,
		Format:  NotificationTemplateFormatHTML,
	})
	if err != nil {
		t.Fatalf("send DingTalk HTML notification: %v", err)
	}
	if payload.MessageType != "markdown" || payload.Markdown.Title != "Transit Hub" {
		t.Fatalf("unexpected DingTalk rich payload: %#v", payload)
	}
	if !strings.Contains(payload.Markdown.Text, "## 余额预警") || !strings.Contains(payload.Markdown.Text, "**8.50**") {
		t.Fatalf("unexpected DingTalk Markdown content: %q", payload.Markdown.Text)
	}
}

func TestWecomMarkdownNotificationUsesMarkdownPayload(t *testing.T) {
	var payload struct {
		MessageType string `json:"msgtype"`
		Markdown    struct {
			Content string `json:"content"`
		} `json:"markdown"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewService(server.Client(), nil)
	err := service.sendWecomMessage(context.Background(), server.URL, notificationMessage{
		Content: "## 余额预警\n\n**8.50**",
		Format:  NotificationTemplateFormatMarkdown,
	})
	if err != nil {
		t.Fatalf("send WeCom Markdown notification: %v", err)
	}
	if payload.MessageType != "markdown" || payload.Markdown.Content != "## 余额预警\n\n**8.50**" {
		t.Fatalf("unexpected WeCom Markdown payload: %#v", payload)
	}
}

func TestFeishuRichNotificationUsesMarkdownCard(t *testing.T) {
	var payload struct {
		MessageType string `json:"msg_type"`
		Card        struct {
			Body struct {
				Elements []struct {
					Tag     string `json:"tag"`
					Content string `json:"content"`
				} `json:"elements"`
			} `json:"body"`
		} `json:"card"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewService(server.Client(), nil)
	err := service.sendFeishuMessage(context.Background(), server.URL, "", notificationMessage{
		Content: "**倍率已变更**",
		Format:  NotificationTemplateFormatMarkdown,
	})
	if err != nil {
		t.Fatalf("send Feishu Markdown notification: %v", err)
	}
	if payload.MessageType != "interactive" || len(payload.Card.Body.Elements) != 1 {
		t.Fatalf("unexpected Feishu card payload: %#v", payload)
	}
	if payload.Card.Body.Elements[0].Tag != "markdown" || payload.Card.Body.Elements[0].Content != "**倍率已变更**" {
		t.Fatalf("unexpected Feishu Markdown element: %#v", payload.Card.Body.Elements[0])
	}
}
