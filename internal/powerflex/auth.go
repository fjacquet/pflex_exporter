package powerflex

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	accessTokenTTL  = 5 * time.Minute  // PowerFlex access tokens are valid 5 minutes
	refreshTokenTTL = 30 * time.Minute // refresh tokens are valid 30 minutes
	expiryMargin    = 30 * time.Second // refresh/relogin this long before actual expiry
)

// tokenResponse is the PowerFlex auth response for login and update-token.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// tokenStore manages the OAuth-style token lifecycle for one cluster. All access is
// mutex-guarded so concurrent requests on a cluster never trigger a double login.
type tokenStore struct {
	mu         sync.Mutex
	httpClient *resty.Client
	baseURL    string
	username   string
	password   string

	accessToken   string
	refreshToken  string
	accessExpiry  time.Time
	refreshExpiry time.Time
}

func newTokenStore(httpClient *resty.Client, baseURL, username, password string) *tokenStore {
	return &tokenStore{
		httpClient: httpClient,
		baseURL:    baseURL,
		username:   username,
		password:   password,
	}
}

// ensureValidToken returns a valid access token, logging in or refreshing as needed.
func (t *tokenStore) ensureValidToken(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	switch {
	case t.accessToken == "":
		if err := t.login(ctx); err != nil {
			return "", err
		}
	case now.After(t.accessExpiry.Add(-expiryMargin)):
		if t.refreshToken != "" && now.Before(t.refreshExpiry) {
			if err := t.refresh(ctx); err != nil {
				return "", err
			}
		} else if err := t.login(ctx); err != nil {
			return "", err
		}
	}
	return t.accessToken, nil
}

// login performs a fresh authentication. Caller must hold t.mu.
func (t *tokenStore) login(ctx context.Context) error {
	resp, err := t.httpClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"username": t.username, "password": t.password}).
		Post(t.baseURL + authLoginPath)
	if err != nil {
		return fmt.Errorf("powerflex login request failed: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("powerflex login failed: status=%d body=%s", resp.StatusCode(), truncate(resp.Body(), 200))
	}

	var tr tokenResponse
	if err := json.Unmarshal(resp.Body(), &tr); err != nil {
		return fmt.Errorf("failed to parse login response: %w", err)
	}
	if tr.AccessToken == "" {
		return fmt.Errorf("powerflex login returned empty access token")
	}

	now := time.Now()
	t.accessToken = tr.AccessToken
	t.refreshToken = tr.RefreshToken
	t.accessExpiry = now.Add(accessTokenTTL)
	t.refreshExpiry = now.Add(refreshTokenTTL)
	return nil
}

// refresh exchanges the refresh token for a new access token, falling back to a full
// login if the refresh token is rejected. Caller must hold t.mu.
func (t *tokenStore) refresh(ctx context.Context) error {
	resp, err := t.httpClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"refresh_token": t.refreshToken}).
		Post(t.baseURL + authRefreshPath)
	if err != nil {
		return fmt.Errorf("powerflex token refresh request failed: %w", err)
	}
	if resp.IsError() {
		// Refresh token expired or invalid: re-authenticate from scratch.
		return t.login(ctx)
	}

	var tr tokenResponse
	if err := json.Unmarshal(resp.Body(), &tr); err != nil {
		return fmt.Errorf("failed to parse refresh response: %w", err)
	}
	if tr.AccessToken == "" {
		return t.login(ctx)
	}

	now := time.Now()
	t.accessToken = tr.AccessToken
	t.accessExpiry = now.Add(accessTokenTTL)
	if tr.RefreshToken != "" {
		t.refreshToken = tr.RefreshToken
		t.refreshExpiry = now.Add(refreshTokenTTL)
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
