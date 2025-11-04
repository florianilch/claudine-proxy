package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"golang.org/x/oauth2"

	"github.com/florianilch/claudine-proxy/internal/tokenstore"
)

// TokenSourceFactory creates an oauth2.TokenSource from a stored token string.
type TokenSourceFactory func(token string) oauth2.TokenSource

// PersistentTokenSource wraps an oauth2.TokenSource with token persistence.
// Initialization is deferred to avoid I/O during application startup.
type PersistentTokenSource struct {
	factory    TokenSourceFactory
	tokenStore tokenstore.TokenStore

	tokenSource func() (oauth2.TokenSource, error)

	lastRefreshToken atomic.Pointer[string]
	writeMu          sync.Mutex
}

// Compile-time check to ensure PersistentTokenSource implements oauth2.TokenSource
var _ oauth2.TokenSource = (*PersistentTokenSource)(nil)

// NewPersistentTokenSource creates a PersistentTokenSource.
// No I/O is performed until the first Token call.
func NewPersistentTokenSource(factory TokenSourceFactory, tokenStore tokenstore.TokenStore) (*PersistentTokenSource, error) {
	if factory == nil {
		return nil, fmt.Errorf("missing token source factory")
	}
	if tokenStore == nil {
		return nil, fmt.Errorf("missing token store")
	}

	p := &PersistentTokenSource{
		factory:    factory,
		tokenStore: tokenStore,
	}

	p.tokenSource = sync.OnceValues(p.createTokenSource)

	return p, nil
}

// createTokenSource performs one-time initialization of the TokenSource.
func (p *PersistentTokenSource) createTokenSource() (oauth2.TokenSource, error) {
	// oauth2.TokenSource.Token() has no context parameter (legacy interface limitation)
	// Use background context for initial token read
	ctx := context.Background()

	initialToken, err := p.tokenStore.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read initial token: %w", err)
	}

	// Remember the initial token to avoid unnecessary write-back on first call to `Token()`
	p.lastRefreshToken.Store(&initialToken)

	return p.factory(initialToken), nil
}

// Token returns a valid token, refreshing if necessary and persisting refresh tokens.
func (p *PersistentTokenSource) Token() (*oauth2.Token, error) {
	ts, err := p.tokenSource()
	if err != nil {
		return nil, err
	}

	freshToken, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("getting token from token source: %w", err)
	}

	// Hot path: lock-free atomic read for minimal contention
	lastPtr := p.lastRefreshToken.Load()
	last := ""
	if lastPtr != nil {
		last = *lastPtr
	}

	// Persist refresh token if changed
	// oauth2.TokenSource.Token() is contractually thread-safe, so concurrent calls receive
	// identical tokens. Worst case: multiple goroutines write the same refresh token value.
	// Note: Static tokens have empty RefreshToken, so this check naturally skips them
	if freshToken.RefreshToken != "" && freshToken.RefreshToken != last {
		p.writeMu.Lock()
		// Note: oauth2.TokenSource interface has no context parameter (legacy interface)
		// Use background context for non-critical write-back operation
		ctx := context.Background()
		if err := p.tokenStore.Write(ctx, freshToken.RefreshToken); err != nil {
			// Write failure for refreshable tokens is an error - this is data loss
			// Access token is still valid, but future refreshes will fail without persisted token
			slog.ErrorContext(ctx, "failed to persist refresh token")
		} else {
			// Update cached token only on success - allows retry on next call
			newToken := freshToken.RefreshToken
			p.lastRefreshToken.Store(&newToken)
		}
		p.writeMu.Unlock()
	}

	return freshToken, nil
}
