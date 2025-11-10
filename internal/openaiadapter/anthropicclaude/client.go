package anthropicclaude

import (
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// newClient creates a new Anthropic client with the provided transport.
// The transport chain needs to handle authentication.
func newClient(transport http.RoundTripper) (*anthropic.Client, error) {
	if transport == nil {
		return nil, fmt.Errorf("transport cannot be nil")
	}

	httpClient := &http.Client{
		Transport: transport,
		// Client.Timeout = 0 allows long-running SSE streams (bounded by server WriteTimeout)
	}

	client := anthropic.NewClient(
		option.WithHTTPClient(httpClient),
		// Generous RequestTimeout bypasses SDK maxTokens checks - actual limit enforced by server WriteTimeout
		option.WithRequestTimeout(1*time.Hour),
	)

	return &client, nil
}
