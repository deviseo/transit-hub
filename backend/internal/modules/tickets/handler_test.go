package tickets

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transithub/backend/internal/shared/authctx"
)

type httpCase struct {
	t   *testing.T
	mux *http.ServeMux
}

func (c httpCase) do(request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c.mux.ServeHTTP(recorder, request)
	return recorder
}

func jsonBody(t *testing.T, value any) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(data)
}

func decode[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	return value
}

func multipartTicket(t *testing.T, fields map[string]string, images map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		_ = writer.WriteField(key, value)
	}
	for name, data := range images {
		part, err := writer.CreateFormFile("images", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(data)
	}
	_ = writer.Close()
	return &body, writer.FormDataContentType()
}

func TestTicketRoutesEndToEnd(t *testing.T) {
	env := newTestEnv()
	mux := http.NewServeMux()
	RegisterRoutes(mux, env.service)
	c := httpCase{t: t, mux: mux}

	// 换取会话
	response := c.do(httptest.NewRequest(http.MethodPost, "/api/client/v1/sites/abcdefghij/tickets/session",
		jsonBody(t, CreateSessionRequest{Credential: Credential{Kind: "bearer", Token: "user-token"}, ClientInfo: "TransitLink 0.2.0"})))
	if response.Code != http.StatusOK {
		t.Fatalf("session: %d %s", response.Code, response.Body)
	}
	token := decode[SessionResponse](t, response).SessionToken

	// 没有会话
	if response := c.do(httptest.NewRequest(http.MethodGet, "/api/client/v1/tickets", nil)); response.Code != http.StatusUnauthorized ||
		!strings.Contains(response.Body.String(), ErrorSessionExpired) {
		t.Fatalf("missing session: %d %s", response.Code, response.Body)
	}
	authorized := func(request *http.Request) *http.Request {
		request.Header.Set("Authorization", "Bearer "+token)
		return request
	}

	// 带图片新建工单（multipart）
	body, contentType := multipartTicket(t, map[string]string{"title": "截图问题", "body": "见附件", "category": "接口调用"},
		map[string][]byte{"screen.png": pngImage(t)})
	request := authorized(httptest.NewRequest(http.MethodPost, "/api/client/v1/tickets", body))
	request.Header.Set("Content-Type", contentType)
	response = c.do(request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}
	env.waitEvent(t)
	created := decode[ClientTicketDetail](t, response)
	if created.Category != "接口调用" || len(created.Messages) != 1 || len(created.Messages[0].Attachments) != 1 {
		t.Fatalf("created: %+v", created)
	}

	// 下载附件：类型固定、禁止猜测
	attachmentID := created.Messages[0].Attachments[0].ID
	response = c.do(authorized(httptest.NewRequest(http.MethodGet, "/api/client/v1/tickets/attachments/"+attachmentID, nil)))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(response.Header().Get("Content-Disposition"), "screen.png") {
		t.Fatalf("attachment: %d %v", response.Code, response.Header())
	}

	// JSON 回复、不存在的工单、太大的图片
	response = c.do(authorized(httptest.NewRequest(http.MethodPost, "/api/client/v1/tickets/"+created.ID+"/messages", jsonBody(t, ReplyRequest{Body: "补充"}))))
	if response.Code != http.StatusOK || len(decode[ClientTicketDetail](t, response).Messages) != 2 {
		t.Fatalf("reply: %d %s", response.Code, response.Body)
	}
	env.waitEvent(t)
	if response := c.do(authorized(httptest.NewRequest(http.MethodGet, "/api/client/v1/tickets/missing", nil))); response.Code != http.StatusNotFound {
		t.Fatalf("missing ticket: %d", response.Code)
	}
	big, bigType := multipartTicket(t, map[string]string{"body": "大图"}, map[string][]byte{"big.png": append(pngImage(t), make([]byte, maxImageBytes)...)})
	request = authorized(httptest.NewRequest(http.MethodPost, "/api/client/v1/tickets/"+created.ID+"/messages", big))
	request.Header.Set("Content-Type", bigType)
	if response := c.do(request); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), ErrorImageTooLarge) {
		t.Fatalf("big image: %d %s", response.Code, response.Body)
	}

	// 站长：登录的用户来自 authctx（由受保护路径的中间件设置）
	admin := func(request *http.Request) *http.Request {
		return request.WithContext(authctx.WithUserID(context.Background(), "owner-1"))
	}
	// 用户在站长回复前追问，工单仍是「待回复」(open)。
	response = c.do(admin(httptest.NewRequest(http.MethodGet, "/api/tickets?status=open&page=1", nil)))
	if response.Code != http.StatusOK || decode[AdminListResponse](t, response).Total != 1 {
		t.Fatalf("admin list: %d %s", response.Code, response.Body)
	}
	if response := c.do(admin(httptest.NewRequest(http.MethodGet, "/api/tickets/counts", nil))); response.Code != http.StatusOK ||
		decode[StatusCounts](t, response) != (StatusCounts{Open: 1}) {
		t.Fatalf("counts: %d %s", response.Code, response.Body)
	}
	response = c.do(admin(httptest.NewRequest(http.MethodPost, "/api/tickets/"+created.ID+"/messages", jsonBody(t, ReplyRequest{Body: "已处理"}))))
	if response.Code != http.StatusOK || decode[AdminTicketDetail](t, response).Status != StatusReplied {
		t.Fatalf("admin reply: %d %s", response.Code, response.Body)
	}
	response = c.do(admin(httptest.NewRequest(http.MethodPut, "/api/tickets/"+created.ID+"/status", jsonBody(t, StatusRequest{Status: StatusClosed}))))
	if response.Code != http.StatusOK || decode[AdminTicketDetail](t, response).Status != StatusClosed {
		t.Fatalf("admin status: %d %s", response.Code, response.Body)
	}
	if response := c.do(admin(httptest.NewRequest(http.MethodGet, "/api/tickets/attachments/"+attachmentID, nil))); response.Code != http.StatusOK {
		t.Fatalf("admin attachment: %d", response.Code)
	}
	if response := c.do(httptest.NewRequest(http.MethodGet, "/api/tickets", nil)); response.Code != http.StatusUnauthorized {
		t.Fatalf("admin routes need a login: %d", response.Code)
	}
	// 用户追问已关闭的工单
	response = c.do(authorized(httptest.NewRequest(http.MethodPost, "/api/client/v1/tickets/"+created.ID+"/messages", jsonBody(t, ReplyRequest{Body: "?"}))))
	if response.Code != http.StatusConflict {
		t.Fatalf("closed: %d %s", response.Code, response.Body)
	}
}
