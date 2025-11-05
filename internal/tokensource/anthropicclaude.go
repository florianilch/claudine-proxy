package tokensource

import (
	"golang.org/x/oauth2"
)

const (
	// ClientID is the public OAuth2 client identifier for Anthropic Claude.
	// This is a public client (no client secret) using PKCE for security.
	ClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	// RedirectURL is the standard redirect URL for Anthropic Claude OAuth flows.
	RedirectURL = "https://console.anthropic.com/oauth/code/callback"
)

// Endpoint defines the OAuth2 endpoints for Anthropic Claude authentication.
var Endpoint = oauth2.Endpoint{
	AuthURL:   "https://claude.ai/oauth/authorize", // for Claude Pro/Max
	TokenURL:  "https://console.anthropic.com/v1/oauth/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// scopes defines the required OAuth scopes for Anthropic Claude
var scopes = []string{"org:create_api_key", "user:profile", "user:inference"}
