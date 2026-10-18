// Package codex reads OpenAI Codex CLI credentials and fetches usage limits.
package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/McKean/aiquokka/internal/usage"
)

// errNoAuthFile means auth.json does not exist: the login is in codex itself
// (see appserver.go) or there is none.
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
}

// loadAuth reads and parses the Codex credentials from auth.json. It reports
// errNoAuthFile when the file does not exist, so fetchUsage can ask codex.
func loadAuth() (*authFile, error) {
	path, err := authPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, errNoAuthFile
	}
	if err != nil {
		return nil, err
	}
	return parseAuth(data, path)
}

// parseAuth parses the auth.json payload.
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
