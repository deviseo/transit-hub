package mass_email

import (
	"context"
	"errors"
	"html"
	"html/template"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"transithub/backend/internal/shared/httpjson"
)

// 退订：群发邮件可以附带退订链接（每个收件人一条，链接参数是 128 位随机的 item ID），
// 收件人确认后地址进入工作区的退订名单，之后所有群发都会跳过该地址。

const (
	// UnsubscribePlaceholder 写在模板 HTML 中时会被替换为当前收件人的退订链接；模板里没有时自动在末尾附加一段退订说明。
	UnsubscribePlaceholder    = "{{unsubscribe_url}}"
	unsubscribePath           = "/email-unsubscribe"
	maxPublicBaseURLLength    = 512
	maxSuppressionsPerRequest = 1000
	maxSuppressionSearchRunes = 100
	suppressionsBodyLimit     = 128 * 1024
)

// normalizePublicBaseURL 校验 TransitHub API 的公开访问地址（例如 https://th.example.com/api），为空表示不附带退订链接。
func normalizePublicBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len(value) > maxPublicBaseURLLength {
		return "", ErrInvalidPublicURL
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil ||
		parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(value, "\"'<> \t\r\n") {
		return "", ErrInvalidPublicURL
	}
	return parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.EscapedPath(), "/"), nil
}

func unsubscribeURL(publicBaseURL string, itemID string) string {
	if publicBaseURL == "" || itemID == "" {
		return ""
	}
	return publicBaseURL + unsubscribePath + "?id=" + url.QueryEscape(itemID)
}

// renderEmailHTML 为单个收件人生成邮件正文：替换模板中的退订占位符，或在没有占位符时追加退订说明。
// link 为空（批次未开启退订链接）时只移除占位符，其余内容保持模板原样。
func renderEmailHTML(templateHTML string, link string) string {
	escaped := html.EscapeString(link)
	if strings.Contains(templateHTML, UnsubscribePlaceholder) {
		return strings.ReplaceAll(templateHTML, UnsubscribePlaceholder, escaped)
	}
	if link == "" {
		return templateHTML
	}
	footer := `<div style="margin:24px 0 0;padding-top:12px;border-top:1px solid #e5e7eb;color:#9ca3af;font-size:12px;line-height:1.6;text-align:center;">` +
		`如不想再收到此类邮件，可<a href="` + escaped + `" style="color:#6b7280;text-decoration:underline;">点此退订</a>。` +
		` If you no longer wish to receive these emails, <a href="` + escaped + `" style="color:#6b7280;text-decoration:underline;">unsubscribe</a>.</div>`
	if index := strings.LastIndex(strings.ToLower(templateHTML), "</body>"); index >= 0 {
		return templateHTML[:index] + footer + templateHTML[index:]
	}
	return templateHTML + footer
}

// ListSuppressions 分页列出工作区的退订名单。
func (s *Service) ListSuppressions(ctx context.Context, userID string, adminAccountID string, search string, page int, pageSize int) (SuppressionPage, error) {
	if s.suppressions == nil {
		return SuppressionPage{Items: []Suppression{}, Page: 1, PageSize: defaultPageSize}, nil
	}
	page, pageSize = normalizePage(page, pageSize, maxItemsPageSize)
	search = strings.ToLower(strings.TrimSpace(search))
	if runes := []rune(search); len(runes) > maxSuppressionSearchRunes {
		search = string(runes[:maxSuppressionSearchRunes])
	}
	items, total, err := s.suppressions.ListSuppressions(ctx, userID, adminAccountID, search, page, pageSize)
	if err != nil {
		log.Printf("[mass-email] list suppressions failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
		return SuppressionPage{}, ErrPersistence
	}
	return SuppressionPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages(total, pageSize)}, nil
}

// AddSuppressions 手动把地址加入退订名单（无效地址跳过并计数）。
func (s *Service) AddSuppressions(ctx context.Context, userID string, adminAccountID string, req AddSuppressionsRequest) (AddSuppressionsResult, error) {
	if s.suppressions == nil {
		return AddSuppressionsResult{}, ErrSegmentUnavailable
	}
	if len(req.Emails) == 0 || len(req.Emails) > maxSuppressionsPerRequest {
		return AddSuppressionsResult{}, ErrInvalidSuppression
	}
	keys := make([]string, 0, len(req.Emails))
	seen := map[string]struct{}{}
	invalid := 0
	for _, raw := range req.Emails {
		key, ok := suppressionKey(raw)
		if !ok {
			invalid++
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return AddSuppressionsResult{Invalid: invalid}, ErrInvalidSuppression
	}
	added, err := s.suppressions.AddSuppressions(ctx, userID, adminAccountID, keys, SuppressionReasonManual)
	if err != nil {
		log.Printf("[mass-email] add suppressions failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
		return AddSuppressionsResult{}, ErrPersistence
	}
	return AddSuppressionsResult{Added: added, Invalid: invalid}, nil
}

// RemoveSuppression 把地址移出退订名单（例如用户主动要求恢复接收）。
func (s *Service) RemoveSuppression(ctx context.Context, userID string, adminAccountID string, email string) error {
	if s.suppressions == nil {
		return ErrNotFound
	}
	key, ok := suppressionKey(email)
	if !ok {
		return ErrInvalidSuppression
	}
	removed, err := s.suppressions.RemoveSuppression(ctx, userID, adminAccountID, key)
	if err != nil {
		log.Printf("[mass-email] remove suppression failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
		return ErrPersistence
	}
	if !removed {
		return ErrNotFound
	}
	return nil
}

// UnsubscribeTarget 返回退订链接对应的收件地址（打码）；链接无效时返回 ErrNotFound。
func (s *Service) UnsubscribeTarget(ctx context.Context, itemID string) (string, error) {
	item, err := s.unsubscribeItem(ctx, itemID)
	if err != nil {
		return "", err
	}
	return maskEmail(item.RecipientEmail), nil
}

// Unsubscribe 把退订链接对应的地址加入其工作区的退订名单（重复退订是幂等的）。
func (s *Service) Unsubscribe(ctx context.Context, itemID string) (string, error) {
	item, err := s.unsubscribeItem(ctx, itemID)
	if err != nil {
		return "", err
	}
	key := item.NormalizedEmail
	if key == "" {
		key = normalizeEmailKey(item.RecipientEmail)
	}
	if _, err := s.suppressions.AddSuppressions(ctx, item.UserID, item.AdminAccountID, []string{key}, SuppressionReasonUnsubscribe); err != nil {
		log.Printf("[mass-email] unsubscribe failed item_id=%s err=%v", item.ID, err)
		return "", ErrPersistence
	}
	return maskEmail(item.RecipientEmail), nil
}

func (s *Service) unsubscribeItem(ctx context.Context, itemID string) (BatchItem, error) {
	if s.suppressions == nil || !validItemID(itemID) {
		return BatchItem{}, ErrNotFound
	}
	item, err := s.suppressions.GetItemByID(ctx, itemID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return BatchItem{}, ErrNotFound
		}
		log.Printf("[mass-email] load unsubscribe item failed err=%v", err)
		return BatchItem{}, ErrPersistence
	}
	return item, nil
}

// validItemID 只接受 newID 生成的 32 位十六进制 ID，避免无效请求访问数据库。
func validItemID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func suppressionKey(raw string) (string, bool) {
	email := strings.TrimSpace(raw)
	if email == "" || len(email) > 320 || strings.ContainsAny(email, "\r\n") {
		return "", false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" || parsed.Address != email {
		return "", false
	}
	return normalizeEmailKey(parsed.Address), true
}

// maskEmail 只显示邮箱首字母和域名，避免退订页面泄露完整地址。
func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	local := []rune(email[:at])
	return string(local[0]) + "***" + email[at:]
}

// ---- 公开的退订页面（不需要 TransitHub 登录；不在 protectedPath 中）----

type publicHandler struct {
	service *Service
}

// RegisterPublicRoutes 注册收件人点击的退订页面：GET 显示确认按钮（避免邮件安全扫描预取链接时误退订），POST 执行退订。
func RegisterPublicRoutes(mux *http.ServeMux, service *Service) {
	handler := &publicHandler{service: service}
	mux.HandleFunc("GET /api"+unsubscribePath, handler.page)
	mux.HandleFunc("POST /api"+unsubscribePath, handler.submit)
}

type unsubscribePageData struct {
	Title    string
	Message  string
	Button   string
	ShowForm bool
}

var unsubscribePageTemplate = template.Must(template.New("unsubscribe").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>{{.Title}}</title>
<style>
body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;background:#f5f5f4;color:#1c1917}
main{max-width:440px;margin:12vh auto;padding:32px;background:#fff;border-radius:16px;box-shadow:0 1px 3px rgba(0,0,0,.08)}
h1{font-size:20px;margin:0 0 12px}
p{font-size:14px;line-height:1.7;color:#57534e;margin:0 0 20px;white-space:pre-line}
button{width:100%;padding:12px;border:0;border-radius:10px;background:#1c1917;color:#fff;font-size:15px;cursor:pointer}
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<p>{{.Message}}</p>
{{if .ShowForm}}<form method="post"><button type="submit">{{.Button}}</button></form>{{end}}
</main>
</body>
</html>`))

func (h *publicHandler) page(w http.ResponseWriter, r *http.Request) {
	masked, err := h.service.UnsubscribeTarget(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		writeUnsubscribeError(w, err)
		return
	}
	writeUnsubscribePage(w, http.StatusOK, unsubscribePageData{
		Title:    "退订邮件 / Unsubscribe",
		Message:  "确认后，" + masked + " 将不再收到该站点发送的群发邮件。\n" + masked + " will no longer receive bulk emails from this site.",
		Button:   "确认退订 / Unsubscribe",
		ShowForm: true,
	})
}

func (h *publicHandler) submit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	masked, err := h.service.Unsubscribe(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		writeUnsubscribeError(w, err)
		return
	}
	writeUnsubscribePage(w, http.StatusOK, unsubscribePageData{
		Title:   "已退订 / Unsubscribed",
		Message: masked + " 已退订，之后不会再收到该站点的群发邮件。\n" + masked + " has been unsubscribed.",
	})
}

func writeUnsubscribeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeUnsubscribePage(w, http.StatusNotFound, unsubscribePageData{
			Title:   "链接无效 / Invalid link",
			Message: "退订链接无效或已失效。\nThis unsubscribe link is invalid or has expired.",
		})
		return
	}
	writeUnsubscribePage(w, http.StatusInternalServerError, unsubscribePageData{
		Title:   "请稍后再试 / Please try again later",
		Message: "暂时无法处理退订请求，请稍后重试。\nWe could not process your request right now. Please try again later.",
	})
}

func writeUnsubscribePage(w http.ResponseWriter, status int, data unsubscribePageData) {
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Robots-Tag", "noindex, nofollow")
	header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.WriteHeader(status)
	if err := unsubscribePageTemplate.Execute(w, data); err != nil {
		log.Printf("[mass-email] render unsubscribe page failed err=%v", err)
	}
}

// ---- 退订名单管理接口（需要登录，挂在 /api/mass-email 下）----

func (h *Handler) listSuppressions(w http.ResponseWriter, r *http.Request) {
	userID, adminAccountID, ok := h.workspace(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	result, err := h.service.ListSuppressions(r.Context(), userID, adminAccountID, query.Get("search"), intQuery(query.Get("page"), 1),
		intQuery(firstNonEmpty(query.Get("page_size"), query.Get("pageSize")), defaultPageSize))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) addSuppressions(w http.ResponseWriter, r *http.Request) {
	userID, adminAccountID, ok := h.workspace(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, suppressionsBodyLimit)
	var req AddSuppressionsRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, string(ErrInvalidSuppression))
		return
	}
	result, err := h.service.AddSuppressions(r.Context(), userID, adminAccountID, req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) removeSuppression(w http.ResponseWriter, r *http.Request) {
	userID, adminAccountID, ok := h.workspace(w, r)
	if !ok {
		return
	}
	if err := h.service.RemoveSuppression(r.Context(), userID, adminAccountID, r.URL.Query().Get("email")); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
