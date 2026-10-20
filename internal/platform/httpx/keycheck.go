package httpx

import (
	"context"
	"crypto/subtle"
	"net/http"
)

// KeyFunc validates a presented key and may enrich the context.
type KeyFunc func(ctx context.Context, key string) (context.Context, bool)

// RequireKey returns middleware that reads header and answers 401 when it is
// missing or rejected by fn. The response never reveals what was checked.
func RequireKey(header string, fn KeyFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(header)
			if key == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized", nil)
				return
			}
			ctx, ok := fn(r.Context(), key)
			if !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized", nil)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// StaticKey accepts exactly one expected key, compared in constant time.
func StaticKey(expected string) KeyFunc {
	return func(ctx context.Context, key string) (context.Context, bool) {
		return ctx, subtle.ConstantTimeCompare([]byte(key), []byte(expected)) == 1
	}
}
