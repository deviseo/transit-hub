package reports

import (
	"log"
	"net/http"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

// RegisterRoutes 注册经营日报的预览与测试发送接口（作用于当前工作区）。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/reports/daily/preview", handler.preview)
	mux.HandleFunc("POST /api/reports/daily/test", handler.sendTest)
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	report, err := h.service.Preview(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, report)
}

type sendTestRequest struct {
	BotIDs []string `json:"botIds"`
}

type sendTestResponse struct {
	Report DailyReport `json:"report"`
	Sent   int         `json:"sent"`
}

func (h *Handler) sendTest(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto sendTestRequest
	if r.ContentLength != 0 {
		if err := httpjson.Decode(r, &dto); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
			return
		}
	}
	report, sent, err := h.service.SendTest(r.Context(), userID, dto.BotIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, sendTestResponse{Report: report, Sent: sent})
}

// writeError 不返回 401：前端会把 401 当作 TransitHub 登录过期。
func writeError(w http.ResponseWriter, err error) {
	if !IsRequestError(err) {
		log.Printf("[daily-report] internal error err=%v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, ErrorRequest)
		return
	}
	status := http.StatusBadRequest
	if err.Error() == ErrorNoCurrentAccount {
		status = http.StatusConflict
	}
	httpjson.WriteError(w, status, err.Error())
}
