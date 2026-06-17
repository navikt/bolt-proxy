package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const tokenFetchTimeout = 3 * time.Second

type tokenRequest struct {
	IdentityProvider string `json:"identity_provider"`
	Target           string `json:"target"`
	SkipCache        bool   `json:"skip_cache,omitempty"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
}

// FetchToken retrieves an Entra ID M2M token from the Nais token endpoint
// (NAIS_TOKEN_ENDPOINT). The endpoint caches tokens automatically; set
// skipCache=true only when a previous token was rejected by the target API.
//
// Timeout is enforced internally — callers should pass a context with a
// deadline for the outer operation, not for this call specifically.
func FetchToken(ctx context.Context, endpoint, target string, skipCache bool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, tokenFetchTimeout)
	defer cancel()

	req := tokenRequest{
		IdentityProvider: "entra_id",
		Target:           target,
		SkipCache:        skipCache,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal token request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("token endpoint request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.Error != "" {
		return "", fmt.Errorf("token endpoint error: %s", tr.Error)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("token endpoint returned empty access_token")
	}

	return tr.AccessToken, nil
}
