package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const introspectTimeout = 3 * time.Second

type introspectRequest struct {
	IdentityProvider string `json:"identity_provider"`
	Token            string `json:"token"`
}

// IntrospectResult contains the validated identity from the Nais introspection endpoint.
type IntrospectResult struct {
	// Active is true when the sidecar confirms the token is valid
	// (signature, exp, iss, aud all pass).
	Active bool

	// AZP is the client ID of the calling application, for logging.
	AZP string
}

// Introspect validates a bearer token via the Nais token introspection endpoint
// (NAIS_TOKEN_INTROSPECTION_ENDPOINT).
//
// active=true is sufficient for authorisation: Entra ID will not issue a token
// targeting this application's audience to any caller not listed in
// accessPolicy.inbound.rules, so the sidecar's signature+claims check is the
// full gate.
//
// The 3s timeout is enforced internally. Returns an error only on transport or
// decode failures — an invalid token returns (IntrospectResult{Active:false}, nil).
func Introspect(ctx context.Context, endpoint, token string) (IntrospectResult, error) {
	ctx, cancel := context.WithTimeout(ctx, introspectTimeout)
	defer cancel()

	req := introspectRequest{
		IdentityProvider: "entra_id",
		Token:            token,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return IntrospectResult{}, fmt.Errorf("marshal introspect request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return IntrospectResult{}, fmt.Errorf("create introspect request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return IntrospectResult{}, fmt.Errorf("introspect endpoint request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return IntrospectResult{}, fmt.Errorf("introspect endpoint returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Active bool   `json:"active"`
		AZP    string `json:"azp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return IntrospectResult{}, fmt.Errorf("decode introspect response: %w", err)
	}

	return IntrospectResult{Active: result.Active, AZP: result.AZP}, nil
}
