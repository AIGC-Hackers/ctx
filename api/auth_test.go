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
