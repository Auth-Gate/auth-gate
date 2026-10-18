package handler

import (
	"net/http"
	"time"

	"github.com/huy090202/auth-gate/internal/auth/client"
	"github.com/huy090202/auth-gate/internal/auth/service"
	"github.com/huy090202/auth-gate/internal/platform/httpx"
)

// NewAdmin returns the admin-role API behind the static admin key.
// The origin-verify wrapper is applied in main.
func NewAdmin(svc *service.Service, adminKey string) http.Handler {
	h := &admin{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/clients", h.create)
	mux.HandleFunc("GET /api/clients/{id}", h.get)
	mux.HandleFunc("POST /api/clients/{id}/keys", h.rotate)
	mux.HandleFunc("DELETE /api/clients/{id}/subjects/{subjectId}", h.purge)
	return httpx.RequireKey("x-internal-access-key", httpx.StaticKey(adminKey))(mux)
}

type admin struct{ svc *service.Service }

type pinPolicyView struct {
	Length      int `json:"length"`
	MaxAttempts int `json:"maxAttempts"`
	LockoutSec  int `json:"lockoutSec"`
}

// clientView is the public view of a client. It deliberately has no key material.
type clientView struct {
	ClientID    string        `json:"clientId"`
	Name        string        `json:"name"`
	RPID        string        `json:"rpId"`
	Origins     []string      `json:"origins"`
	PinPolicy   pinPolicyView `json:"pinPolicy"`
	TokenTTLSec int           `json:"tokenTtlSec"`
	CreatedAt   string        `json:"createdAt"`
}

func (h *admin) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string         `json:"name"`
		RPID        string         `json:"rpId"`
		Origins     []string       `json:"origins"`
		PinPolicy   *pinPolicyView `json:"pinPolicy"`
		TokenTTLSec *int           `json:"tokenTtlSec"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		writeServiceError(w, err)
		return
	}
	in := client.CreateInput{Name: body.Name, RPID: body.RPID, Origins: body.Origins, TokenTTLSec: body.TokenTTLSec}
	if p := body.PinPolicy; p != nil {
		in.PinPolicy = &client.PinPolicy{Length: p.Length, MaxAttempts: p.MaxAttempts, LockoutSec: p.LockoutSec}
	}
	c, key, err := h.svc.CreateClient(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"clientId": c.ID, "accessKey": key})
}

func (h *admin) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.GetClient(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	origins := c.Origins
	if origins == nil {
		origins = []string{}
	}
	httpx.WriteJSON(w, http.StatusOK, clientView{
		ClientID: c.ID, Name: c.Name, RPID: c.RPID, Origins: origins,
		PinPolicy:   pinPolicyView{Length: c.PinPolicy.Length, MaxAttempts: c.PinPolicy.MaxAttempts, LockoutSec: c.PinPolicy.LockoutSec},
		TokenTTLSec: c.TokenTTLSec, CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *admin) rotate(w http.ResponseWriter, r *http.Request) {
	key, err := h.svc.RotateClientKey(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"accessKey": key})
}

func (h *admin) purge(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.PurgeSubject(r.Context(), r.PathValue("id"), r.PathValue("subjectId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
