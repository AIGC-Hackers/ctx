package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGetValidTokenRefreshIncludesClientIDAndPreservesRefreshToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveTokens(&TokenData{
		AccessToken:  "expired-token",
		RefreshToken: "existing-refresh",
		TokenType:    "bearer",
		ExpiresAt:    time.Now().Add(-time.Minute).UnixMilli(),
	}); err != nil {
		t.Fatalf("SaveTokens() error = %v", err)
	}

	var form url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/token" {
			t.Fatalf("path = %q, want /api/oauth/token", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-token","token_type":"bearer","expires_in":3600}`))
	}))
	defer server.Close()

	token, err := GetValidToken(server.URL)
	if err != nil {
		t.Fatalf("GetValidToken() error = %v", err)
	}
	if token != "fresh-token" {
		t.Fatalf("token = %q, want fresh-token", token)
	}
	if got := form.Get("grant_type"); got != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", got)
	}
	if got := form.Get("client_id"); got != clientID {
		t.Errorf("client_id = %q, want %q", got, clientID)
	}
	if got := form.Get("refresh_token"); got != "existing-refresh" {
		t.Errorf("refresh_token = %q, want existing-refresh", got)
	}

	tokens, err := LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens() error = %v", err)
	}
	if tokens.RefreshToken != "existing-refresh" {
		t.Fatalf("saved refresh_token = %q, want existing-refresh", tokens.RefreshToken)
	}
}

func TestGetValidTokenClearsInvalidGrant(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveTokens(&TokenData{
		AccessToken:  "expired-token",
		RefreshToken: "revoked-refresh",
		TokenType:    "bearer",
		ExpiresAt:    time.Now().Add(-time.Minute).UnixMilli(),
	}); err != nil {
		t.Fatalf("SaveTokens() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Refresh token expired"}`))
	}))
	defer server.Close()

	token, err := GetValidToken(server.URL)
	if err != nil {
		t.Fatalf("GetValidToken() error = %v", err)
	}
	if token != "" {
		t.Fatalf("token = %q, want empty fallback", token)
	}

	tokens, err := LoadTokens()
	if err == nil && (tokens.AccessToken != "" || tokens.RefreshToken != "") {
		t.Fatalf("tokens were not cleared: %+v", tokens)
	}
}

func TestGetValidTokenKeepsTokensOnTransientRefreshFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveTokens(&TokenData{
		AccessToken:  "expired-token",
		RefreshToken: "keep-refresh",
		TokenType:    "bearer",
		ExpiresAt:    time.Now().Add(-time.Minute).UnixMilli(),
	}); err != nil {
		t.Fatalf("SaveTokens() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
	}))
	defer server.Close()

	token, err := GetValidToken(server.URL)
	if err != nil {
		t.Fatalf("GetValidToken() error = %v", err)
	}
	if token != "" {
		t.Fatalf("token = %q, want empty fallback", token)
	}

	tokens, err := LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens() error = %v", err)
	}
	if tokens.RefreshToken != "keep-refresh" {
		t.Fatalf("refresh_token = %q, want keep-refresh", tokens.RefreshToken)
	}
}

func TestLoginUsesDeviceAuthorizationFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	oldSleep := sleep
	var sleeps []time.Duration
	sleep = func(d time.Duration) {
		sleeps = append(sleeps, d)
	}
	defer func() { sleep = oldSleep }()

	var codeForm url.Values
	var tokenForms []url.Values
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}

		switch r.URL.Path {
		case "/api/oauth/device/code":
			codeForm = r.PostForm
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"device_code":"device-123",
				"user_code":"ABCD-EFGH",
				"verification_uri":"https://context7.com/device",
				"verification_uri_complete":"https://context7.com/device?user_code=ABCD-EFGH",
				"expires_in":60,
				"interval":1
			}`))
		case "/api/oauth/device/token":
			tokenForms = append(tokenForms, r.PostForm)
			pollCount++
			w.Header().Set("Content-Type", "application/json")
			switch pollCount {
			case 1:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			case 2:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"slow_down"}`))
			case 3:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
			default:
				_, _ = w.Write([]byte(`{"access_token":"device-token","refresh_token":"device-refresh","token_type":"bearer","expires_in":3600}`))
			}
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	if err := Login(server.URL, true); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if got := codeForm.Get("client_id"); got != clientID {
		t.Fatalf("device code client_id = %q, want %q", got, clientID)
	}
	if codeForm.Get("hostname") == "" {
		t.Fatalf("device code hostname was empty")
	}
	if len(tokenForms) != 4 {
		t.Fatalf("token polls = %d, want 4", len(tokenForms))
	}
	for i, form := range tokenForms {
		if got := form.Get("grant_type"); got != deviceCodeGrant {
			t.Fatalf("poll %d grant_type = %q, want %q", i+1, got, deviceCodeGrant)
		}
		if got := form.Get("client_id"); got != clientID {
			t.Fatalf("poll %d client_id = %q, want %q", i+1, got, clientID)
		}
		if got := form.Get("device_code"); got != "device-123" {
			t.Fatalf("poll %d device_code = %q, want device-123", i+1, got)
		}
	}
	if len(sleeps) != 4 {
		t.Fatalf("sleep calls = %d, want 4", len(sleeps))
	}
	if sleeps[0] != time.Second || sleeps[1] != time.Second || sleeps[2] != 6*time.Second || sleeps[3] != 11*time.Second {
		t.Fatalf("sleep backoff = %v, want [1s 1s 6s 11s]", sleeps)
	}

	tokens, err := LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens() error = %v", err)
	}
	if tokens.AccessToken != "device-token" || tokens.RefreshToken != "device-refresh" {
		t.Fatalf("saved tokens = %+v, want device-token/device-refresh", tokens)
	}
}

func TestOAuthTokenErrorUsesServerDescription(t *testing.T) {
	err := (&oauthTokenError{
		StatusCode:       http.StatusBadRequest,
		Code:             "invalid_grant",
		ErrorDescription: "Refresh token expired",
	}).Error()

	if !strings.Contains(err, "Refresh token expired") || !strings.Contains(err, "invalid_grant") {
		t.Fatalf("error string = %q, want server description and code", err)
	}
}
