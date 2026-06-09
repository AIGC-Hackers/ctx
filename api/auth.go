package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ethan-huo/ctx/config"
)

const (
	clientID                         = "2veBSofhicRBguUT"
	deviceCodeGrant                  = "urn:ietf:params:oauth:grant-type:device_code"
	defaultDevicePollIntervalSeconds = 5
)

type TokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type tokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type oauthTokenError struct {
	StatusCode       int
	Code             string
	ErrorDescription string
	Operation        string
}

type deviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval,omitempty"`
}

type devicePollStatus string

const (
	devicePollApproved  devicePollStatus = "approved"
	devicePollPending   devicePollStatus = "pending"
	devicePollSlowDown  devicePollStatus = "slow_down"
	devicePollDenied    devicePollStatus = "denied"
	devicePollExpired   devicePollStatus = "expired"
	devicePollTransient devicePollStatus = "transient"
)

type devicePollResult struct {
	Status       devicePollStatus
	Tokens       *TokenData
	ErrorMessage string
}

var sleep = time.Sleep

func (e *oauthTokenError) Error() string {
	operation := e.Operation
	if operation == "" {
		operation = "refresh failed"
	}
	if e.ErrorDescription != "" {
		if e.Code != "" {
			return fmt.Sprintf("%s: HTTP %d: %s (%s)", operation, e.StatusCode, e.ErrorDescription, e.Code)
		}
		return fmt.Sprintf("%s: HTTP %d: %s", operation, e.StatusCode, e.ErrorDescription)
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: HTTP %d: %s", operation, e.StatusCode, e.Code)
	}
	return fmt.Sprintf("%s: HTTP %d", operation, e.StatusCode)
}

func LoadTokens() (*TokenData, error) {
	creds, err := config.LoadCredentials()
	if err != nil {
		return nil, err
	}
	c := creds.Ctx7
	if c.AccessToken == "" {
		return nil, fmt.Errorf("not logged in to Context7")
	}
	return &TokenData{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		TokenType:    c.TokenType,
		ExpiresIn:    c.ExpiresIn,
		ExpiresAt:    c.ExpiresAt,
		Scope:        c.Scope,
	}, nil
}

func SaveTokens(t *TokenData) error {
	if t.ExpiresAt == 0 && t.ExpiresIn > 0 {
		t.ExpiresAt = time.Now().UnixMilli() + t.ExpiresIn*1000
	}
	return config.UpdateCredentials(func(creds *config.Credentials) {
		creds.Ctx7 = config.Ctx7Creds{
			AccessToken:  t.AccessToken,
			RefreshToken: t.RefreshToken,
			TokenType:    t.TokenType,
			ExpiresIn:    t.ExpiresIn,
			ExpiresAt:    t.ExpiresAt,
			Scope:        t.Scope,
		}
	})
}

func ClearTokens() error {
	return config.UpdateCredentials(func(creds *config.Credentials) {
		creds.Ctx7 = config.Ctx7Creds{}
	})
}

func IsTokenExpired(t *TokenData) bool {
	if t.ExpiresAt == 0 {
		return false
	}
	return time.Now().UnixMilli() > t.ExpiresAt-60000
}

// GetValidToken returns a valid access token, refreshing if needed.
func GetValidToken(baseURL string) (string, error) {
	if key := os.Getenv("CONTEXT7_API_KEY"); key != "" {
		return key, nil
	}

	tokens, err := LoadTokens()
	if err != nil {
		return "", nil
	}

	if !IsTokenExpired(tokens) {
		return tokens.AccessToken, nil
	}

	if tokens.RefreshToken == "" {
		fmt.Fprintf(os.Stderr, "Context7 token expired and no refresh token available. Run: ctx auth login ctx7\n")
		return "", nil
	}

	newTokens, err := refreshToken(baseURL, tokens.RefreshToken)
	if err != nil {
		var tokenErr *oauthTokenError
		if errors.As(err, &tokenErr) && tokenErr.Code == "invalid_grant" {
			_ = ClearTokens()
		}
		fmt.Fprintf(os.Stderr, "Context7 token refresh failed: %v — falling back to anonymous. Run: ctx auth login ctx7\n", err)
		return "", nil
	}
	if newTokens.RefreshToken == "" {
		// OAuth refresh responses may omit refresh_token when the existing one remains valid.
		newTokens.RefreshToken = tokens.RefreshToken
	}
	if err := SaveTokens(newTokens); err != nil {
		return "", err
	}
	return newTokens.AccessToken, nil
}

func refreshToken(baseURL, refresh string) (*TokenData, error) {
	resp, err := http.PostForm(baseURL+"/api/oauth/token", url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refresh},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var tokenErr tokenErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&tokenErr); err == nil {
			return nil, &oauthTokenError{
				StatusCode:       resp.StatusCode,
				Code:             tokenErr.Error,
				ErrorDescription: tokenErr.ErrorDescription,
			}
		}
		return nil, &oauthTokenError{StatusCode: resp.StatusCode}
	}

	var t TokenData
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Login runs Context7's current device authorization flow, matching the upstream CLI.
func Login(baseURL string, noBrowser bool) error {
	authorization, err := startDeviceAuthorization(baseURL)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	printDeviceAuthorization(authorization)
	target := authorization.VerificationURIComplete
	if target == "" {
		target = authorization.VerificationURI
	}

	if noBrowser {
		fmt.Println("Open the link above in any browser to continue.")
	} else {
		fmt.Println("Opening browser for login...")
		openBrowser(target)
	}

	fmt.Println("Waiting for authorization...")

	deadline := time.Now().Add(time.Duration(authorization.ExpiresIn) * time.Second)
	interval := authorization.Interval
	if interval == 0 {
		interval = defaultDevicePollIntervalSeconds
	}
	intervalDuration := time.Duration(interval) * time.Second

	for time.Now().Before(deadline) {
		sleep(intervalDuration)
		result, err := pollDeviceToken(baseURL, authorization.DeviceCode)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		switch result.Status {
		case devicePollApproved:
			if result.Tokens == nil {
				return fmt.Errorf("login failed: device authorization approved without tokens")
			}
			if err := SaveTokens(result.Tokens); err != nil {
				return err
			}
			return nil
		case devicePollPending:
			continue
		case devicePollSlowDown, devicePollTransient:
			// RFC 8628 requires reducing poll frequency on slow_down; upstream also
			// applies the same backoff to transient 5xx/network errors.
			intervalDuration += 5 * time.Second
			continue
		case devicePollDenied:
			return fmt.Errorf("authorization denied")
		case devicePollExpired:
			return fmt.Errorf("code expired; run login again")
		default:
			if result.ErrorMessage != "" {
				return fmt.Errorf("login failed: %s", result.ErrorMessage)
			}
			return fmt.Errorf("login failed: unexpected device poll status %q", result.Status)
		}
	}

	return fmt.Errorf("code expired without approval")
}

func startDeviceAuthorization(baseURL string) (*deviceAuthorization, error) {
	params := url.Values{"client_id": {clientID}}
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		params.Set("hostname", hostname)
	}

	resp, err := http.PostForm(baseURL+"/api/oauth/device/code", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, tokenEndpointError(resp, "device authorization failed")
	}

	var authorization deviceAuthorization
	if err := json.NewDecoder(resp.Body).Decode(&authorization); err != nil {
		return nil, err
	}
	return &authorization, nil
}

func pollDeviceToken(baseURL, deviceCode string) (*devicePollResult, error) {
	resp, err := http.PostForm(baseURL+"/api/oauth/device/token", url.Values{
		"grant_type":  {deviceCodeGrant},
		"client_id":   {clientID},
		"device_code": {deviceCode},
	})
	if err != nil {
		return &devicePollResult{Status: devicePollTransient, ErrorMessage: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var t TokenData
		if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
			return nil, err
		}
		return &devicePollResult{Status: devicePollApproved, Tokens: &t}, nil
	}

	var tokenErr tokenErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&tokenErr)
	if resp.StatusCode >= 500 {
		return &devicePollResult{
			Status:       devicePollTransient,
			ErrorMessage: firstNonEmpty(tokenErr.ErrorDescription, tokenErr.Error, fmt.Sprintf("HTTP %d", resp.StatusCode)),
		}, nil
	}

	switch tokenErr.Error {
	case "authorization_pending":
		return &devicePollResult{Status: devicePollPending}, nil
	case "slow_down":
		return &devicePollResult{Status: devicePollSlowDown}, nil
	case "access_denied":
		return &devicePollResult{Status: devicePollDenied}, nil
	case "expired_token":
		return &devicePollResult{Status: devicePollExpired}, nil
	default:
		return nil, fmt.Errorf("%s", firstNonEmpty(tokenErr.ErrorDescription, tokenErr.Error, "device token poll failed"))
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		cmd = exec.Command("open", url)
	}
	cmd.Start()
}

func printDeviceAuthorization(authorization *deviceAuthorization) {
	fmt.Println("Sign in to Context7")
	fmt.Printf("One-time code: %s\n", authorization.UserCode)
	if authorization.VerificationURIComplete != "" {
		fmt.Printf("Approve: %s\n", authorization.VerificationURIComplete)
		fmt.Printf("Or visit %s and enter the code above.\n", authorization.VerificationURI)
		return
	}
	fmt.Printf("Visit: %s\n", authorization.VerificationURI)
}

func tokenEndpointError(resp *http.Response, operation string) error {
	var tokenErr tokenErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenErr); err == nil {
		return &oauthTokenError{
			StatusCode:       resp.StatusCode,
			Code:             tokenErr.Error,
			ErrorDescription: tokenErr.ErrorDescription,
			Operation:        operation,
		}
	}
	return &oauthTokenError{StatusCode: resp.StatusCode, Operation: operation}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
