package httpjson

import (
	"encoding/json"
	"errors"
	"net/http"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

func Write(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	Write(w, status, ErrorResponse{Message: message})
}

func Decode(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("invalid JSON body")
	}
	return nil
}
