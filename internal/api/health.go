package api

import (
	"net/http"

	"github.com/Tragidra/logstruct/internal/storage"
)

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type readyHandler struct{ repo storage.Repository }

func newReadyHandler(repo storage.Repository) *readyHandler { return &readyHandler{repo: repo} }

func (h *readyHandler) readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "storage unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
