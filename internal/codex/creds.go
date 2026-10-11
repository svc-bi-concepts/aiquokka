// Package codex reads OpenAI Codex CLI credentials and fetches usage limits.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/McKean/aiquokka/internal/usage"
)

// codexHome is $CODEX_HOME, or ~/.codex.
func codexHome() (string, error) {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

// authPath is ~/.codex/auth.json.
func authPath() (string, error) {
	home, err := codexHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "auth.json"), nil
}

// tokens mirrors the tokens object in auth.json.
type tokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

type authFile struct {
	AuthMode     string `json:"auth_mode"`
	OpenAIAPIKey string `json:"OPENAI_API_KEY"`
	Tokens       tokens `json:"tokens"`
	LastRefresh  string `json:"last_refresh"`

	// source identifies where the credential was loaded so a refreshed token
	// is written back to that same store. It is deliberately not serialized.
	source authSource
}

// authSource is set when the login came from the Keychain (codex-cli's
// cli_auth_credentials_store = "keyring"): the item's account and the JSON it
// held, so a refresh can rewrite the item without losing unknown keys.
type authSource struct {
	keychainAccount string
	raw             []byte
}

// loadAuth reads the Codex login from auth.json, or from the Keychain when
// there is no auth.json (codex-cli with keyring storage).
func loadAuth(ctx context.Context) (*authFile, error) {
	path, err := authPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return loadKeychainAuth(ctx, path)
		}
		return nil, err
	}
	return parseAuth(data, path)
}

// loadKeychainAuth reads the login codex-cli stored in the Keychain. path is
// the auth.json it would have used, for the error message.
func loadKeychainAuth(ctx context.Context, path string) (*authFile, error) {
	notFound := usage.NotConfigured("no Codex credentials at %s or in the Keychain — run `codex login` first", path)
	home, err := codexHome()
	if err != nil {
		return nil, err
	}
	account, err := keychainAccount(home)
	if err != nil {
		return nil, notFound
	}
	data, err := readKeychain(ctx, account)
	if errors.Is(err, errKeychainNotFound) {
		return nil, notFound
	}
	if err != nil {
		return nil, err
	}
	af, err := parseAuth(data, "the Keychain")
	if err != nil {
		return nil, err
	}
	af.source = authSource{keychainAccount: account, raw: data}
	return af, nil
}

// parseAuth reads a login in auth.json form; origin names where it came from.
func parseAuth(data []byte, origin string) (*authFile, error) {
	var af authFile
	if err := json.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", origin, err)
	}
	if af.Tokens.AccessToken == "" {
		return nil, usage.NotConfigured("no ChatGPT access token in %s (API-key auth has no usage limits)", origin)
	}
	return &af, nil
}
