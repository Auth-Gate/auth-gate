package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireKeyAnswers401BeforeRoutingAndNeverSaysKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	wrapped := RequireKey("x-origin-verify", StaticKey("cf"))(RequireKey("x-internal-access-key", StaticKey("s3cret"))(mux))

	do := func(method, path string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		return rec
	}
	both := map[string]string{"x-origin-verify": "cf", "x-internal-access-key": "s3cret"}

	rec := do("GET", "/nope", nil)
	if rec.Code != 401 || rec.Body.String() != `{"error":"unauthorized"}`+"\n" {
		t.Fatalf("no headers: %d %q", rec.Code, rec.Body.String())
	}
	if c := do("GET", "/nope", both).Code; c != 404 {
		t.Errorf("unknown path = %d, want 404", c)
	}
	if c := do("GET", "/api/x", map[string]string{"x-origin-verify": "cf", "x-internal-access-key": "wrong"}).Code; c != 401 {
		t.Errorf("wrong key = %d, want 401", c)
	}
	if c := do("POST", "/api/x", both).Code; c != 405 {
		t.Errorf("wrong method = %d, want 405", c)
	}
	if c := do("GET", "/api/x", map[string]string{"x-internal-access-key": "s3cret"}).Code; c != 401 {
		t.Errorf("missing origin header = %d, want 401", c)
	}
}

func TestDecodeJSONRejectsMalformedAndWrongType(t *testing.T) {
	for _, body := range []string{`{"pin":123456}`, `{`} {
		var dst struct{ Pin string }
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		if err := DecodeJSON(req, &dst); !errors.Is(err, ErrBadJSON) {
			t.Errorf("body %q: err = %v, want ErrBadJSON", body, err)
		}
	}
}
