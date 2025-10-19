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

func New(ts oauth2.TokenSource, baseURL string) (*Proxy, error) {
	return nil, nil
}

func (p *Proxy) Start(context.Context, string) (<-chan error, error) {
	return nil, nil
}

func (p *Proxy) Shutdown(context.Context) error {
	return nil
}
