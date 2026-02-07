package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bartlettc22/paperless-llm-processor/internal/paperless"
)

type DocumentsHandler struct {
	Client *paperless.Client
}

func (h *DocumentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.Client == nil {
		http.Error(w, "paperless-ngx not configured (set PAPERLESS_URL and PAPERLESS_TOKEN)", http.StatusServiceUnavailable)
		return
	}

	docs, err := h.Client.ListDocuments(r.Context())
	if err != nil {
		http.Error(w, "failed to list documents: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docs)
}
