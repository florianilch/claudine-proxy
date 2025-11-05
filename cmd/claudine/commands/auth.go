package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
	"golang.org/x/oauth2"
	"golang.org/x/term"

	"github.com/florianilch/claudine-proxy/internal/app"
	"github.com/florianilch/claudine-proxy/internal/tokensource"
)

// authCommand returns the 'auth' subcommand for managing provider authentication.
func authCommand() *cli.Command {
	return &cli.Command{
		Name:  "auth",
		Usage: "Manage provider authentication",
		Commands: []*cli.Command{
			authLoginCommand(),
			authLogoutCommand(),
		},
	}
}

// authLoginCommand returns the 'auth login' subcommand.
func authLoginCommand() *cli.Command {
	return &cli.Command{
		Name:   "login",
		Usage:  "Login to Anthropic Claude and save credentials",
		Action: authLoginAction,
	}
}

// authLogoutCommand returns the 'auth logout' subcommand.
func authLogoutCommand() *cli.Command {
	return &cli.Command{
		Name:   "logout",
		Usage:  "Logout from Anthropic Claude and clear credentials",
		Action: authLogoutAction,
	}
}

// authLoginAction implements the OAuth login flow for Anthropic Claude.
func authLoginAction(ctx context.Context, cmd *cli.Command) error {
	cfg, err := loadConfig(cmd.String("config"), cmd, os.Environ)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Auth.Storage == app.TokenStorageTypeEnv {
		return fmt.Errorf("cannot login with env storage (read-only). Configure file or keyring storage")
	}

	store, err := cfg.Auth.NewTokenStore()
	if err != nil {
		return fmt.Errorf("failed to create token store: %w", err)
	}

	token, err := runAnthropicOAuth(ctx)
	if err != nil {
		return fmt.Errorf("oauth login failed: %w", err)
	}

	if err := store.Write(ctx, token); err != nil {
		return fmt.Errorf("failed to write token: %w", err)
	}

	fmt.Println()
	fmt.Println("=== Login Successful ===")
	fmt.Println("Token saved to configured storage")
	fmt.Println("Anthropic Claude is now configured and ready to use")

	return nil
}

// authLogoutAction implements the logout flow for Anthropic Claude.
func authLogoutAction(ctx context.Context, cmd *cli.Command) error {
	cfg, err := loadConfig(cmd.String("config"), cmd, os.Environ)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Auth.Storage == app.TokenStorageTypeEnv {
		return fmt.Errorf("cannot logout with env storage (read-only). Configure file or keyring storage")
	}

	store, err := cfg.Auth.NewTokenStore()
	if err != nil {
		return fmt.Errorf("failed to create token store: %w", err)
	}

	// Clear token via empty string write to maintain storage abstraction
	if err := store.Write(ctx, ""); err != nil {
		return fmt.Errorf("failed to clear token: %w", err)
	}

	fmt.Println()
	fmt.Println("=== Logout Successful ===")
	fmt.Println("Credentials cleared from configured storage")

	return nil
}

// readSecureInput reads user input with hidden display and context cancellation support.
// Goroutine+select pattern required because term.ReadPassword has no native context support.
func readSecureInput(ctx context.Context, prompt string) (string, error) {
	fmt.Print(prompt)
	defer fmt.Println()

	type result struct {
		value string
		err   error
	}
	resultCh := make(chan result, 1)

	go func() {
		inputBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		resultCh <- result{value: string(inputBytes), err: err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-resultCh:
		if res.err != nil {
			return "", fmt.Errorf("failed to read input: %w", res.err)
		}
		return res.value, nil
	}
}

// runAnthropicOAuth performs OAuth login for Anthropic Claude.
func runAnthropicOAuth(ctx context.Context) (string, error) {
	authorizer := tokensource.NewAuthorizer(
		tokensource.Endpoint,
		tokensource.RedirectURL,
	)

	verifier := oauth2.GenerateVerifier()
	authURL := authorizer.AuthCodeURL(verifier)

	fmt.Println("=== Anthropic Claude OAuth Login ===")
	fmt.Println()
	fmt.Printf("1. Visit this URL in your browser:\n   %s\n\n", authURL)
	fmt.Println("2. Authorize the application")
	fmt.Println("3. Paste the authorization code")

	code, err := readSecureInput(ctx, "\nEnter authorization code: ")
	if err != nil {
		return "", err
	}

	if code == "" {
		return "", fmt.Errorf("authorization code cannot be empty")
	}

	token, err := authorizer.Exchange(ctx, code, verifier)
	if err != nil {
		return "", fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	return token.RefreshToken, nil
}
