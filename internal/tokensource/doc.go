// Package tokensource provides OAuth2 token acquisition and automatic refresh
// for Anthropic Claude API.
//
// Anthropic's OAuth2 implementation require custom handling in a few ways:
//   - Token exchange and refresh use JSON-encoded requests (OAuth2 typically uses form-encoding)
//   - Token exchange requires a "state" field in the request body
//   - Authorization codes are returned in "code#state" format requiring custom parsing
//
// # OAuth2 Authorization Flow
//
// Use Authorizer for the initial OAuth2 flow to obtain refresh tokens:
//
//	auth := tokensource.NewAuthorizer(tokensource.Endpoint, redirectURL)
//	verifier := oauth2.GenerateVerifier() // Save for Exchange call
//	authURL := auth.AuthCodeURL(verifier)
//	// After user authorizes, Anthropic redirects with "code#state" format
//	codeWithState := "auth_code_xyz#state_value" // Extract from redirect
//	token, err := auth.Exchange(ctx, codeWithState, verifier)
//	// Save token.RefreshToken for future use
//
// # Token Sources
//
// Use NewTokenSource for OAuth2 refresh tokens:
//
//	ts := tokensource.NewTokenSource(refreshToken, tokensource.Endpoint)
//	// TokenSource implements oauth2.TokenSource and can be used with oauth2.Transport
//
// # Custom Base Transport
//
// Configure a custom base transport for token refresh requests (e.g., for
// proxies or custom timeouts):
//
//	ts := tokensource.NewTokenSource(
//	  refreshToken,
//	  tokensource.Endpoint,
//	  tokensource.WithTransport(customTransport),
//	)
package tokensource
