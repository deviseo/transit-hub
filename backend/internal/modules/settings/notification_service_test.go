package settings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestWecomNotificationReusesTextWebhookWithoutSigning(t *testing.T) {
	type webhookPayload struct {
		MessageType string `json:"msgtype"`
		Text        struct {
			Content string `json:"content"`
		} `json:"text"`
	}
	type receivedRequest struct {
		Method  string
		Query   url.Values
		Payload webhookPayload
	}

	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := receivedRequest{Method: r.Method, Query: r.URL.Query()}
		_ = json.NewDecoder(r.Body).Decode(&request.Payload)
		received <- request
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewService(server.Client(), nil)
	err := service.TestNotification(context.Background(), TestNotificationRequest{
		Channel: NotificationChannelWecom,
		Webhook: server.URL + "?key=wecom-test",
		Secret:  "must-not-be-used",
	})
	if err != nil {
		t.Fatalf("send WeCom test notification: %v", err)
	}

	request := <-received
	if request.Method != http.MethodPost {
		t.Fatalf("expected POST request, got %s", request.Method)
	}
	if request.Query.Get("key") != "wecom-test" {
		t.Fatalf("expected original webhook key, got %q", request.Query.Get("key"))
	}
	if request.Query.Has("timestamp") || request.Query.Has("sign") {
		t.Fatalf("WeCom webhook must not include DingTalk signature parameters: %v", request.Query)
	}
	if request.Payload.MessageType != "text" || request.Payload.Text.Content != testMessage {
		t.Fatalf("unexpected WeCom webhook payload: %#v", request.Payload)
	}
}

func TestQQNotificationFetchesTokenAndSendsDirectMessage(t *testing.T) {
	tokenRequests := 0
	messageRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/getAppAccessToken":
			tokenRequests++
			if r.Method != http.MethodPost {
				t.Errorf("expected QQ token POST request, got %s", r.Method)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode QQ token request: %v", err)
			}
			if payload["appId"] != "app-1" || payload["clientSecret"] != "secret-1" {
				t.Errorf("unexpected QQ token payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "token-1",
				"expires_in":   "7200",
			})
		case "/v2/users/user-open-id/messages":
			messageRequests++
			if r.Method != http.MethodPost {
				t.Errorf("expected QQ message POST request, got %s", r.Method)
			}
			if authorization := r.Header.Get("Authorization"); authorization != "QQBot token-1" {
				t.Errorf("unexpected QQ authorization header: %q", authorization)
			}
			if appID := r.Header.Get("X-Union-Appid"); appID != "app-1" {
				t.Errorf("unexpected QQ app ID header: %q", appID)
			}
			var payload struct {
				MessageType int    `json:"msg_type"`
				Content     string `json:"content"`
				Markdown    struct {
					Content string `json:"content"`
				} `json:"markdown"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode QQ message request: %v", err)
			}
			if messageRequests <= 2 {
				if payload.MessageType != 0 || payload.Content != testMessage {
					t.Errorf("unexpected QQ text message payload: %#v", payload)
				}
			} else if payload.MessageType != 2 || payload.Content != "**余额预警**" || payload.Markdown.Content != "**余额预警**" {
				t.Errorf("unexpected QQ Markdown message payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "message-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService(server.Client(), nil)
	service.qqAPIBaseURL = server.URL
	service.qqTokenURL = server.URL + "/app/getAppAccessToken"
	request := TestNotificationRequest{
		Channel:        NotificationChannelQQ,
		QQAppID:        " app-1 ",
		QQClientSecret: " secret-1 ",
		QQUserOpenID:   " user-open-id ",
	}
	if err := service.TestNotification(context.Background(), request); err != nil {
		t.Fatalf("send QQ test notification: %v", err)
	}
	if err := service.TestNotification(context.Background(), request); err != nil {
		t.Fatalf("send second QQ test notification: %v", err)
	}
	if err := service.sendQQMessage(context.Background(), "app-1", "secret-1", "user-open-id", notificationMessage{
		Content: "**余额预警**",
		Format:  NotificationTemplateFormatMarkdown,
	}); err != nil {
		t.Fatalf("send QQ Markdown notification: %v", err)
	}
	if tokenRequests != 1 {
		t.Fatalf("expected cached QQ token after one token request, got %d", tokenRequests)
	}
	if messageRequests != 3 {
		t.Fatalf("expected two text and one Markdown QQ direct messages, got %d", messageRequests)
	}
}

func TestQQNotificationRequiresCompleteConfiguration(t *testing.T) {
	service := NewService(nil, nil)
	err := service.TestNotification(context.Background(), TestNotificationRequest{
		Channel: NotificationChannelQQ,
		QQAppID: "app-1",
	})
	if !errors.Is(err, ErrMissingQQConfig) {
		t.Fatalf("expected ErrMissingQQConfig, got %v", err)
	}
}

func TestQQGroupDraftRemainsStoredButIsNotUsedAsUserOpenID(t *testing.T) {
	settings := normalizeNotificationChannelSettings(NotificationChannelSettings{
		QQ: []QQChannelSettings{{
			ID:          "qq-legacy",
			GroupOpenID: " legacy-group-open-id ",
		}},
	})
	if settings.QQ[0].GroupOpenID != "legacy-group-open-id" {
		t.Fatalf("expected legacy QQ group OpenID to be preserved, got %q", settings.QQ[0].GroupOpenID)
	}
	if settings.QQ[0].UserOpenID != "" {
		t.Fatalf("legacy QQ group OpenID must not become a user OpenID, got %q", settings.QQ[0].UserOpenID)
	}
}

func TestQQResponseRecognizesNonZeroErrorCodes(t *testing.T) {
	for _, responseBody := range [][]byte{
		[]byte(`{"code":11248,"message":"invalid request"}`),
		[]byte(`{"err_code":"40034025","message":"send failed"}`),
	} {
		if !qqResponseHasError(responseBody) {
			t.Fatalf("expected QQ response to be treated as an error: %s", responseBody)
		}
	}
	if qqResponseHasError([]byte(`{"code":0,"id":"message-1"}`)) {
		t.Fatal("expected zero QQ response code to be treated as success")
	}
}

func TestNotificationSettingsWithoutNewChannelsRemainCompatible(t *testing.T) {
	settings := DefaultNotificationChannelSettings()
	err := unmarshalNotificationChannelSettings([]byte(`{
		"dingtalk":[{"id":"ding-1","name":"existing","enabled":true,"webhook":"https://example.test/hook","secret":"secret"}],
		"feishu":[],
		"telegram":[]
	}`), &settings)
	if err != nil {
		t.Fatalf("unmarshal legacy notification settings: %v", err)
	}
	if len(settings.Dingtalk) != 1 || settings.Dingtalk[0].ID != "ding-1" || settings.Dingtalk[0].Secret != "secret" {
		t.Fatalf("existing DingTalk settings changed: %#v", settings.Dingtalk)
	}
	if settings.Wecom == nil || len(settings.Wecom) != 0 {
		t.Fatalf("expected an empty non-nil WeCom settings list, got %#v", settings.Wecom)
	}
	if settings.QQ == nil || len(settings.QQ) != 0 {
		t.Fatalf("expected an empty non-nil QQ settings list, got %#v", settings.QQ)
	}
}
