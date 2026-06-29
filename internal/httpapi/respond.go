package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Pedro-0101/gix-server/internal/core"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// writeErr mapeia os erros sentinela do core para status HTTP.
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		http.Error(w, "não encontrado", http.StatusNotFound)
	case errors.Is(err, core.ErrNotImplemented):
		http.Error(w, "não implementado", http.StatusNotImplemented)
	default:
		http.Error(w, "erro interno", http.StatusInternalServerError)
	}
}
