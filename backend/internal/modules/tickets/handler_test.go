package tickets

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"transithub/backend/internal/shared/authctx"
)

// withIsolatedTempDir 把 TMPDIR/TMP/TEMP 都指向一个专属临时目录（t.TempDir() 会在测试结束后
// 自动整体清理，不触碰真实系统临时目录），并返回该目录路径供测试断言其内容。
// mime/multipart.ReadForm 在文件部分超过 maxMemory 时会调用 os.CreateTemp("", "multipart-")，
// 也就是写入 os.TempDir()；三个环境变量分别对应 Unix（TMPDIR）和 Windows（TMP/TEMP）下
// os.TempDir() 的取值来源，同时设置以保证测试在任意平台都能命中隔离目录。
func withIsolatedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	return dir
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected multipart temp files to be cleaned up, found leftover entries: %v", names)
	}
}

// buildOversizedMultipartRequest 构造一个 manualEmail/title/body + 单张超过 maxMultipartMemory
// 的图片字段的 multipart 请求，确保 mime/multipart 在解析时会把图片部分溢出写入磁盘临时文件
// （不为了测试而调低 maxMultipartMemory，用真实的生产阈值触发溢出路径）。
func buildOversizedMultipartRequest(t *testing.T) *http.Request {
	t.Helper()
	oversizedImage := gifSignatureBytes(maxMultipartMemory + 1<<20)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("manualEmail", "user@example.com"); err != nil {
		t.Fatalf("write field manualEmail: %v", err)
	}
	if err := writer.WriteField("title", "help"); err != nil {
		t.Fatalf("write field title: %v", err)
	}
	if err := writer.WriteField("body", "see attached"); err != nil {
		t.Fatalf("write field body: %v", err)
	}
	part, err := writer.CreateFormFile("images", "big.gif")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(oversizedImage); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/embed/tickets", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// TestParseMultipartTicketRequest_CleansUpTempFiles 覆盖整改要求一的成功路径：图片字段超过
// maxMultipartMemory 触发 mime/multipart 把内容溢出写入系统临时目录后，
// parseMultipartTicketRequest 返回时必须已经清理掉这些临时文件。
func TestParseMultipartTicketRequest_CleansUpTempFiles(t *testing.T) {
	spillDir := withIsolatedTempDir(t)
	req := buildOversizedMultipartRequest(t)
	recorder := httptest.NewRecorder()

	parsedReq, uploads, err := parseMultipartTicketRequest(recorder, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsedReq.ManualEmail != "user@example.com" || parsedReq.Title != "help" || parsedReq.Body != "see attached" {
		t.Fatalf("expected form fields to be parsed, got %+v", parsedReq)
	}
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}

	assertDirEmpty(t, spillDir)
}

// TestCreateEmbedTicket_CleansUpTempFilesOnValidationFailure 覆盖整改要求一的失败路径：
// 走完整的 HTTP handler（不是直接调用 parser），workspace 配置为 0（关闭图片上传），
// Service 层会以"图片数量超限"拒绝请求；即便业务校验失败，multipart 临时文件也必须已经
// 在 parseMultipartTicketRequest 返回时被清理（不会因为后续 Service 拒绝而遗留）。
// 不依赖真实 PostgreSQL/Redis/Sub2API：Service 由内存假实现（fakeTicketRepository 等，定义在
// service_test.go，同包共用）构造。
func TestCreateEmbedTicket_CleansUpTempFilesOnValidationFailure(t *testing.T) {
	spillDir := withIsolatedTempDir(t)

	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 0)

	req := buildOversizedMultipartRequest(t)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	recorder := httptest.NewRecorder()

	handler := &Handler{service: svc}
	handler.createEmbedTicket(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when image uploads are disabled, got %d: %s", recorder.Code, recorder.Body.String())
	}

	assertDirEmpty(t, spillDir)
}

// TestCreateEmbedTicket_MultipartPersistsCategoryAndPriority 覆盖 multipart 创建路径：
// category/priority 必须和 manualEmail/title/body 一样作为表单字段被解析并落库返回。
func TestCreateEmbedTicket_MultipartPersistsCategoryAndPriority(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 3)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("manualEmail", "user@example.com"); err != nil {
		t.Fatalf("write field manualEmail: %v", err)
	}
	if err := writer.WriteField("title", "help"); err != nil {
		t.Fatalf("write field title: %v", err)
	}
	if err := writer.WriteField("body", "see attached"); err != nil {
		t.Fatalf("write field body: %v", err)
	}
	if err := writer.WriteField("category", "通用问题"); err != nil {
		t.Fatalf("write field category: %v", err)
	}
	if err := writer.WriteField("priority", "普通"); err != nil {
		t.Fatalf("write field priority: %v", err)
	}
	part, err := writer.CreateFormFile("images", "a.gif")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(gifSignatureBytes(16)); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/embed/tickets", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	recorder := httptest.NewRecorder()

	handler := &Handler{service: svc}
	handler.createEmbedTicket(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var detail EmbedTicketDetail
	if err := json.NewDecoder(recorder.Body).Decode(&detail); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if detail.Category != "通用问题" || detail.Priority != "普通" {
		t.Fatalf("expected category/priority to be parsed and persisted, got %+v", detail)
	}
}

// TestCreateEmbedTicket_JSONPersistsCategoryAndPriority 覆盖旧的 JSON 创建路径：新增的
// category/priority 字段必须和其余字段一样通过 json 标签被解析并落库返回。
func TestCreateEmbedTicket_JSONPersistsCategoryAndPriority(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	payload := `{"manualEmail":"user@example.com","title":"help","body":"body","category":"通用问题","priority":"普通"}`
	req := httptest.NewRequest(http.MethodPost, "/api/embed/tickets", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	recorder := httptest.NewRecorder()

	handler := &Handler{service: svc}
	handler.createEmbedTicket(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var detail EmbedTicketDetail
	if err := json.NewDecoder(recorder.Body).Decode(&detail); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if detail.Category != "通用问题" || detail.Priority != "普通" {
		t.Fatalf("expected category/priority to be parsed and persisted, got %+v", detail)
	}
}

func TestRegisterRoutes_AdminTicketSub2apiProfileDoesNotConflictWithAttachments(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = &Ticket{
		ID:             "t1",
		UserID:         "user1",
		AdminAccountID: "account1",
		ManualEmail:    "a@example.com",
		Title:          "help",
		Sub2apiUserID:  "42",
		Sub2apiEmail:   "sub2api@example.com",
		Sub2apiRole:    "member",
		Sub2apiSrcHost: "https://web.example.com",
		Sub2apiSrcURL:  "https://web.example.com/custom/abc",
	}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})
	mux := http.NewServeMux()

	RegisterRoutes(mux, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/t1/sub2api-user-profile", nil)
	req = req.WithContext(authctx.WithUserID(req.Context(), "user1"))
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected profile route to return 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var profile Sub2apiUserProfileResponse
	if err := json.NewDecoder(recorder.Body).Decode(&profile); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	if profile.Sub2apiUserID != "42" || profile.Sub2apiEmail != "sub2api@example.com" {
		t.Fatalf("unexpected profile response: %+v", profile)
	}
}
