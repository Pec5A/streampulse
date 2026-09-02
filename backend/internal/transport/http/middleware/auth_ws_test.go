package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/infrastructure/auth"
)

const wsTestSecret = "test-secret-at-least-32-bytes-long"

// echoIdentity records what the middleware injected into the context.
func echoIdentity(gotUserID, gotRole *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotUserID, _ = UserID(r.Context())
		*gotRole, _ = UserRole(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuthWS_AcceptsTokenFromQueryParameter(t *testing.T) {
	// This is the whole reason the middleware exists: a browser cannot set
	// an Authorization header on a WebSocket upgrade.
	jwtManager := auth.NewJWTManager(wsTestSecret, time.Hour)
	token, err := jwtManager.Generate("user-1", "broadcaster")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var gotUserID, gotRole string
	h := RequireAuthWS(jwtManager)(echoIdentity(&gotUserID, &gotRole))

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+url.QueryEscape(token), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if gotUserID != "user-1" {
		t.Errorf("user id = %q, want user-1", gotUserID)
	}
	if gotRole != "broadcaster" {
		t.Errorf("role = %q, want broadcaster", gotRole)
	}
}

func TestRequireAuthWS_AcceptsTokenFromHeader(t *testing.T) {
	// Native clients can still use the header; the query parameter is a
	// fallback, not a replacement.
	jwtManager := auth.NewJWTManager(wsTestSecret, time.Hour)
	token, err := jwtManager.Generate("user-2", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var gotUserID, gotRole string
	h := RequireAuthWS(jwtManager)(echoIdentity(&gotUserID, &gotRole))

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if gotUserID != "user-2" {
		t.Errorf("user id = %q, want user-2", gotUserID)
	}
}

func TestRequireAuthWS_HeaderWinsOverQueryParameter(t *testing.T) {
	// A stale link carrying an old ?token= must not override the credential
	// the client explicitly sent.
	jwtManager := auth.NewJWTManager(wsTestSecret, time.Hour)
	headerToken, err := jwtManager.Generate("header-user", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	queryToken, err := jwtManager.Generate("query-user", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var gotUserID, gotRole string
	h := RequireAuthWS(jwtManager)(echoIdentity(&gotUserID, &gotRole))

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+url.QueryEscape(queryToken), nil)
	req.Header.Set("Authorization", "Bearer "+headerToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotUserID != "header-user" {
		t.Errorf("user id = %q, want header-user", gotUserID)
	}
}

func TestRequireAuthWS_Rejects(t *testing.T) {
	jwtManager := auth.NewJWTManager(wsTestSecret, time.Hour)
	other := auth.NewJWTManager("a-completely-different-secret-32b!!", time.Hour)
	foreignToken, err := other.Generate("user-1", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	expired := auth.NewJWTManager(wsTestSecret, -time.Hour)
	expiredToken, err := expired.Generate("user-1", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	cases := map[string]struct {
		target string
		header string
	}{
		"no credential at all":        {target: "/ws"},
		"empty token parameter":       {target: "/ws?token="},
		"garbage token":               {target: "/ws?token=not-a-jwt"},
		"token signed by another key": {target: "/ws?token=" + url.QueryEscape(foreignToken)},
		"expired token":               {target: "/ws?token=" + url.QueryEscape(expiredToken)},
		"malformed header":            {target: "/ws", header: "Basic abc"},
		"bearer with no value":        {target: "/ws", header: "Bearer "},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			h := RequireAuthWS(jwtManager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if called {
				t.Error("the protected handler ran despite a rejected credential")
			}
		})
	}
}

func TestRequireAuth_DoesNotAcceptAQueryToken(t *testing.T) {
	// The query-parameter fallback must stay confined to the WebSocket
	// route. If this test ever fails, every authenticated endpoint has
	// started accepting credentials in the URL.
	jwtManager := auth.NewJWTManager(wsTestSecret, time.Hour)
	token, err := jwtManager.Generate("user-1", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	called := false
	h := RequireAuth(jwtManager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/streams?token="+url.QueryEscape(token), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d — RequireAuth must stay header-only", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Error("RequireAuth accepted a token from the query string")
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]struct {
		header string
		want   string
	}{
		"valid":            {"Bearer abc.def.ghi", "abc.def.ghi"},
		"empty":            {"", ""},
		"no scheme":        {"abc.def.ghi", ""},
		"wrong scheme":     {"Basic abc", ""},
		"case sensitive":   {"bearer abc", ""},
		"scheme only":      {"Bearer", ""},
		"scheme and space": {"Bearer ", ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := bearerToken(tc.header); got != tc.want {
				t.Errorf("bearerToken(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}
