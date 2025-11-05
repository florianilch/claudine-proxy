package tokensource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Authorizer handles the OAuth2 authorization flow for Anthropic Claude.
// It uses manual HTTP requests for token exchange because Anthropic requires
// a non-standard 'state' field in the token endpoint request body.
type Authorizer struct {
	config *oauth2.Config
	client *http.Client
}

// NewAuthorizer creates a new Anthropic Claude OAuth authorizer.
func NewAuthorizer(endpoint oauth2.Endpoint, redirectURL string) *Authorizer {
	config := &oauth2.Config{
		ClientID:     ClientID,
		ClientSecret: "",
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint:     endpoint,
	}

	return &Authorizer{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// AuthCodeURL generates the authorization URL for the OAuth2 flow with PKCE.
// The state parameter serves dual purpose: OAuth2 CSRF protection and PKCE code verifier.
// Caller must persist state and provide the same value to Exchange.
func (a *Authorizer) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	allOpts := append(opts,
		oauth2.S256ChallengeOption(state),
		oauth2.SetAuthURLParam("code", "true"),
	)

	return a.config.AuthCodeURL(state, allOpts...)
}

// Exchange completes the OAuth2 flow by exchanging an authorization code for tokens.
// Handles Anthropic's non-standard "code#state" response format and includes the state
// field in the token request body (required by Anthropic but not standard OAuth2).
// Verifier must be the same value passed as state to AuthCodeURL.
func (a *Authorizer) Exchange(ctx context.Context, codeWithState string, verifier string) (*oauth2.Token, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if verifier == "" {
		return nil, errors.New("verifier cannot be empty")
	}

	code, state, found := strings.Cut(codeWithState, "#")
	if !found {
		return nil, errors.New("invalid code format: missing '#' separator")
	}

	if state != verifier {
		return nil, errors.New("state mismatch")
	}

	exchangeReq := exchangeRequest{
		Code:         code,
		State:        state,
		GrantType:    "authorization_code",
		ClientID:     ClientID,
		RedirectURI:  a.config.RedirectURL,
		CodeVerifier: verifier,
	}

	requestBody, err := json.Marshal(exchangeReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling exchange request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.config.Endpoint.TokenURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("creating exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	now := time.Now()
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange failed with status %d", resp.StatusCode)
	}

	var token oauth2.Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("decoding exchange response: %w", err)
	}

	// Convert ExpiresIn to Expiry (see oauth2.Token.ExpiresIn field documentation)
	if token.ExpiresIn > 0 {
		token.Expiry = now.Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}

// exchangeRequest represents the token exchange request body.
// Includes the non-standard State field required by Anthropic's token endpoint.
type exchangeRequest struct {
	Code         string `json:"code"`
	State        string `json:"state"`
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	CodeVerifier string `json:"code_verifier"`
}
