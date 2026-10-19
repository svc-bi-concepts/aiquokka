// Package claude reads Claude Code credentials and fetches subscription usage.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/McKean/aiquokka/internal/usage"
)

// credentialsPath is ~/.claude/.credentials.json.
func credentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
}

// oauth mirrors the claudeAiOauth object in .credentials.json.
type oauth struct {
	AccessToken           string   `json:"accessToken"`
	RefreshToken          string   `json:"refreshToken"`
	ExpiresAt             int64    `json:"expiresAt"`
	RefreshTokenExpiresAt int64    `json:"refreshTokenExpiresAt"`
	Scopes                []string `json:"scopes"`
	SubscriptionType      string   `json:"subscriptionType"`
	RateLimitTier         string   `json:"rateLimitTier"`
}

type credentialsFile struct {
	ClaudeAiOauth oauth `json:"claudeAiOauth"`
}

var errKeychainCredentialsNotFound = errors.New("Claude Code Keychain credentials not found")

// errStale means the access token has expired. aiquokka does not refresh it
// and never writes Claude Code's credentials: running claude renews the token.
var errStale = errors.New("Claude credentials are stale: the access token expired — run `claude`")

// readKeychainCredentials is a private seam so tests never touch the real
// Keychain. Production code always uses the platform implementation.
var readKeychainCredentials = loadKeychainCredentials

// expired reports whether the access token is past its expiry.
func (o oauth) expired(now time.Time) bool {
	if o.ExpiresAt == 0 {
		return false
	}
	return now.UnixMilli() >= o.ExpiresAt
}

// loadCredentials prefers ~/.claude/.credentials.json when it contains an
// OAuth access token and falls back to the macOS Keychain otherwise.
func loadCredentials(ctx context.Context) (*oauth, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}

	data, fileErr := os.ReadFile(path)
	if fileErr == nil {
		o, _, err := parseCredentials(data)
		if err == nil {
			return o, nil
		}
		fileErr = fmt.Errorf("parsing %s: %w", path, err)
	} else if !os.IsNotExist(fileErr) {
		return nil, fileErr
	}

	if data, err := readKeychainCredentials(ctx); err == nil {
		o, _, err := parseCredentials(data)
		if err != nil {
			return nil, err
		}
		return o, nil
	} else if !errors.Is(err, errKeychainCredentialsNotFound) {
		return nil, err
	}

	if fileErr != nil && !os.IsNotExist(fileErr) {
		return nil, fileErr
	}
	return nil, usage.NotConfigured("no Claude credentials at %s — run `claude` to log in", path)
}

// parseCredentials accepts both the historical file envelope and a bare OAuth
// object, which lets the Keychain backend read whichever representation the
// installed Claude Code version uses.
func parseCredentials(data []byte) (*oauth, bool, error) {
	var cf credentialsFile
	if err := json.Unmarshal(data, &cf); err == nil && cf.ClaudeAiOauth.AccessToken != "" {
		return &cf.ClaudeAiOauth, false, nil
	}

	var o oauth
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, false, err
	}
	if o.AccessToken == "" {
		return nil, false, fmt.Errorf("no OAuth access token")
	}
	return &o, true, nil
}
