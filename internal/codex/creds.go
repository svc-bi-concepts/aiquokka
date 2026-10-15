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

// errNoAuthFile means auth.json does not exist: the login is somewhere else
// (the Keychain on darwin, see keychain.go, or codex, see appserver.go), or
// there is none.
var errNoAuthFile = errors.New("no Codex auth file")

// codexHome is $CODEX_HOME or ~/.codex.
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
	source credentialSource
}

type credentialSource struct {
	keychainItem []byte
}

// loadAuth reads and parses the Codex credentials: auth.json first, unchanged;
// when it does not exist, the Codex Keychain item on darwin.
func loadAuth(ctx context.Context) (*authFile, error) {
	path, err := authPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		return parseAuth(data, path)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	// auth.json is absent: on darwin the login may be in the Keychain.
	data, item, err := readKeychainAuth(ctx)
	if err == nil {
		af, err := parseAuth(data, keychainWhere)
		if err != nil {
			return nil, err
		}
		af.source = credentialSource{keychainItem: item}
		return af, nil
	}
	if !errors.Is(err, errKeychainCredentialsNotFound) {
		return nil, err
	}
	return nil, errNoAuthFile
}

// parseAuth parses the auth.json payload — the Keychain item stores the same
// JSON — and reports which store it came from in error messages.
func parseAuth(data []byte, where string) (*authFile, error) {
	var af authFile
	if err := json.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", where, err)
	}
	if af.Tokens.AccessToken == "" {
		return nil, usage.NotConfigured("no ChatGPT access token in %s (API-key auth has no usage limits)", where)
	}
	return &af, nil
}
