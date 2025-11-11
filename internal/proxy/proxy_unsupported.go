//go:build !goexperiment.jsonv2

package proxy

import (
	"context"

	"golang.org/x/oauth2"
)

func init() {
	panic("proxy requires GOEXPERIMENT=jsonv2")
}

type Proxy struct{}

type ReadinessChecker interface {
	IsReady() bool
}

type Option func(*config)
type config struct{}

func WithBaseURL(baseURL string) Option {
	return func(c *config) {}
}

func New(oauth2.TokenSource, ReadinessChecker, ...Option) (*Proxy, error) {
	return nil, nil
}

func (p *Proxy) Start(context.Context, string) (<-chan error, error) {
	return nil, nil
}

func (p *Proxy) Shutdown(context.Context) error {
	return nil
}
