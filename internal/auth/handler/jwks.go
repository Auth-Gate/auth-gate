package handler

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/huy090202/auth-gate/internal/auth/token"
	"github.com/huy090202/auth-gate/internal/platform/httpx"
)

// NewJWKS returns the public jwks-role API. The key set is built on the first
// successful request and kept for the life of the process.
func NewJWKS(signers ...token.Signer) http.Handler {
	var (
		mu  sync.Mutex
		set map[string]any
	)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if set == nil {
			s, err := token.JWKS(r.Context(), signers...)
			if err != nil {
				slog.Error("build jwks", "err", err)
				httpx.WriteError(w, http.StatusInternalServerError, "internal", nil)
				return
			}
			set = s
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		httpx.WriteJSON(w, http.StatusOK, set)
	})
	return mux
}
