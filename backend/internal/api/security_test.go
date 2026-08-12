package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerTokenRequired(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	RequireToken("secret", next).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr = httptest.NewRecorder()
	RequireToken("secret", next).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestLocalCORSAllowsPackagedFileOriginPreflightOnly(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	req := httptest.NewRequest(http.MethodOptions, "/api/dashboard", nil)
	req.Header.Set("Origin", "null")
	rr := httptest.NewRecorder()
	LocalCORS(next).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent || rr.Header().Get("Access-Control-Allow-Origin") != "null" {
		t.Fatalf("status=%d headers=%v", rr.Code, rr.Header())
	}

	req = httptest.NewRequest(http.MethodOptions, "/api/dashboard", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr = httptest.NewRecorder()
	LocalCORS(next).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("unexpected external origin status %d", rr.Code)
	}
}
