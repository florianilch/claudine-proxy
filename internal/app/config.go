package app

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/florianilch/claudine-proxy/internal/tokenstore"
	"github.com/go-playground/validator/v10"
)

// LogFormat represents the logging output format.
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// TokenStorageType represents the different storage types supported for stored tokens.
type TokenStorageType string

const (
	TokenStorageTypeFile TokenStorageType = "file"
	TokenStorageTypeEnv  TokenStorageType = "env"
)

// AuthenticationMethod represents the different authentication methods supported.
type AuthenticationMethod string

const (
	AuthenticationMethodStatic AuthenticationMethod = "static"
	AuthenticationMethodOAuth  AuthenticationMethod = "oauth"
)

// Default configuration values
const (
	DefaultConfigLogFormat       = LogFormatText
	DefaultConfigServerHost      = "127.0.0.1"
	DefaultConfigServerPort      = 4000
	DefaultConfigShutdownTimeout = 5 * time.Second
	DefaultConfigAuthStorage     = TokenStorageTypeFile
	DefaultConfigAuthMethod      = AuthenticationMethodOAuth
	DefaultConfigUpstreamBaseURL = "https://api.anthropic.com/v1"
)

// ServerConfig holds server-specific configuration.
type ServerConfig struct {
	Host string `json:"host" validate:"hostname_rfc1123|ip"`
	Port uint16 `json:"port"` // Port range 0-65535 handled by uint16 type
}

// ShutdownConfig holds shutdown behavior configuration.
type ShutdownConfig struct {
	// Timeout for graceful shutdown.
	Timeout time.Duration `json:"timeout"`
}

// UpstreamConfig holds upstream API configuration.
type UpstreamConfig struct {
	BaseURL string `json:"base_url" validate:"required,url"`
}

// AuthConfig represents the configuration for provider authentication.
// Describes how to construct TokenStore and TokenSource components.
type AuthConfig struct {
	// Storage configuration - where the stored token comes from
	Storage TokenStorageType `json:"storage" validate:"required,oneof=file env keyring"`

	// Storage-specific settings (mutually exclusive based on Storage type)
	File   string `json:"file,omitempty"`    // For file storage: path to token file
	EnvKey string `json:"env_key,omitempty"` // For env storage: environment variable name

	// Authentication method - how to convert stored_token to access_token
	Method AuthenticationMethod `json:"method" validate:"required,oneof=oauth static"`
}

// NewTokenStore creates a TokenStore from the authentication configuration.
func (a *AuthConfig) NewTokenStore() (tokenstore.TokenStore, error) {
	switch a.Storage {
	case TokenStorageTypeFile:
		return tokenstore.NewFileStore(a.File)
	case TokenStorageTypeEnv:
		return tokenstore.NewEnvStore(a.EnvKey)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", a.Storage)
	}
}

// Config holds the application's configuration.
type Config struct {
	// LogLevel for logging output (defaults to Info if unset).
	LogLevel  slog.Level     `json:"log_level"`
	LogFormat LogFormat      `json:"log_format" validate:"oneof=text json"`
	Server    ServerConfig   `json:"server"`
	Shutdown  ShutdownConfig `json:"shutdown"`
	Upstream  UpstreamConfig `json:"upstream"`
	Auth      AuthConfig     `json:"auth"`
}

// Default creates a new Config with default values applied.
func Default() (*Config, error) {
	cfg := &Config{}
	if err := cfg.ApplyDefaults(); err != nil {
		return nil, fmt.Errorf("failed to apply defaults: %w", err)
	}
	return cfg, nil
}

// ApplyDefaults fills unset config fields with sensible defaults.
func (c *Config) ApplyDefaults() error {
	if c.LogFormat == "" {
		c.LogFormat = DefaultConfigLogFormat
	}
	if c.Server.Host == "" {
		c.Server.Host = DefaultConfigServerHost
	}
	if c.Server.Port == 0 {
		c.Server.Port = DefaultConfigServerPort
	}
	if c.Shutdown.Timeout == 0 {
		c.Shutdown.Timeout = DefaultConfigShutdownTimeout
	}
	if c.Upstream.BaseURL == "" {
		c.Upstream.BaseURL = DefaultConfigUpstreamBaseURL
	}
	if c.Auth.Storage == "" {
		c.Auth.Storage = DefaultConfigAuthStorage
	}
	if c.Auth.Method == "" {
		c.Auth.Method = DefaultConfigAuthMethod
	}

	// Dynamic defaults based on storage type
	switch c.Auth.Storage {
	case TokenStorageTypeFile:
		if c.Auth.File == "" {
			configDir, err := os.UserConfigDir()
			if err != nil {
				return fmt.Errorf("auth.file required (auto-detect failed: %w)", err)
			}
			c.Auth.File = filepath.Join(configDir, "claudine-proxy", "auth")
		}
	case TokenStorageTypeEnv:
		// env_key must be explicitly configured (no sensible default)
	}

	return nil
}

// Validate validates the configuration using struct tags and enum values.
func (c *Config) Validate() error {
	if err := validator.New().Struct(c); err != nil {
		return err
	}

	// OAuth requires writable storage (env is read-only)
	if c.Auth.Method == AuthenticationMethodOAuth && c.Auth.Storage == TokenStorageTypeEnv {
		return errors.New("oauth authentication requires writable storage, env is read-only")
	}

	switch c.Auth.Storage {
	case TokenStorageTypeFile:
		if c.Auth.File == "" {
			return errors.New("file path required for file storage")
		}
	case TokenStorageTypeEnv:
		if c.Auth.EnvKey == "" {
			return errors.New("env_key required for env storage")
		}
	}

	return nil
}
