package tokenstore

import (
	"context"
	"fmt"

	"github.com/zalando/go-keyring"
)

// KeyringStore provides OS-native secure credential storage for tokens.
// Uses macOS Keychain, Windows Credential Manager, or Linux Secret Service.
type KeyringStore struct {
	service string
	user    string
}

// Compile-time check to ensure KeyringStore implements TokenStore
var _ TokenStore = (*KeyringStore)(nil)

// NewKeyringStore creates a KeyringStore for the OS-native credential storage
// (macOS Keychain, Windows Credential Manager, etc.) using the given service and user identifiers.
func NewKeyringStore(service, user string) (*KeyringStore, error) {
	if service == "" {
		return nil, fmt.Errorf("service cannot be empty")
	}
	if user == "" {
		return nil, fmt.Errorf("user cannot be empty")
	}

	return &KeyringStore{
		service: service,
		user:    user,
	}, nil
}

// Read returns the token from the system keyring. Returns error if not found or empty.
func (k *KeyringStore) Read(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	token, err := keyring.Get(k.service, k.user)
	if err != nil {
		return "", err
	}

	if token == "" {
		return "", fmt.Errorf("empty token in keyring for service %s, user %s", k.service, k.user)
	}

	return token, nil
}

// Write persists the token to the system keyring, overwriting any existing value.
func (k *KeyringStore) Write(ctx context.Context, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return keyring.Set(k.service, k.user, token)
}
