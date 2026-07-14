package health

import (
	"net/http"
	"time"

	"transithub/backend/internal/shared/httpjson"
)

type response struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", check)
}

func check(w http.ResponseWriter, r *http.Request) {
	httpjson.Write(w, http.StatusOK, response{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}
