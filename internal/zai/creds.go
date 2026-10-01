// Package zai reports GLM Coding Plan usage windows plus Z.ai usage bundles
// and cash balance.
package zai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/McKean/aiquokka/internal/usage"
)

// userHomeDir is a test seam: it lets tests point the pi config lookup at a
// temp HOME instead of the real one.
var userHomeDir = os.UserHomeDir

// loadKey returns the Z.ai API key, trying $ZAI_API_KEY first, then the key
// the pi coding agent stores in ~/.pi/agent/auth.json ("zai" -> {type:
// "api_key", key: "..."}), and finally the zai provider entry in
// ~/.pi/agent/models.json. Z.ai keys have the form "{id}.{secret}". The key
// is only ever used for requests — it is never printed or logged.
func loadKey() (string, error) {
	if v := strings.TrimSpace(os.Getenv("ZAI_API_KEY")); v != "" {
		return v, nil
	}
	if home, err := userHomeDir(); err == nil {
		piAgent := filepath.Join(home, ".pi", "agent")
		if key := authKey(filepath.Join(piAgent, "auth.json")); key != "" {
			return key, nil
		}
		if key := piKey(filepath.Join(piAgent, "models.json")); key != "" {
			return key, nil
		}
	}
	return "", usage.NotConfigured("no Z.ai API key found — set ZAI_API_KEY ({id}.{secret}) or configure the zai provider in pi")
}

// authKey extracts the Z.ai credential from a pi auth.json file, which maps
// provider names to credential entries like {"zai": {"type": "api_key",
// "key": "..."}}. It returns "" when the file is missing, unreadable, or
// has no api_key zai entry. The returned key is never logged.
func authKey(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg map[string]struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	entry, ok := cfg["zai"]
	if !ok {
		return ""
	}
	if entry.Type != "" && entry.Type != "api_key" {
		return "" // e.g. an OAuth entry — not a raw API key
	}
	return strings.TrimSpace(entry.Key)
}

// piKey extracts providers.zai.apiKey from a pi models.json file, returning ""
// when the file or the entry is missing.
func piKey(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		Providers map[string]struct {
			APIKey string `json:"apiKey"`
		} `json:"providers"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Providers["zai"].APIKey)
}

// mintJWT signs the Zhipu-style authentication JWT: a HS256 token whose header
// carries sign_type "SIGN", whose payload carries the key id, an expiry and a
// timestamp, signed with the secret half of the API key. This is the only
// credential form the console endpoints accept — the raw API key is rejected.
func mintJWT(apiKey string) (string, error) {
	id, secret, ok := strings.Cut(apiKey, ".")
	if !ok || id == "" || secret == "" {
		return "", fmt.Errorf("Z.ai API key must have the form {id}.{secret}")
	}
	now := time.Now().UnixMilli()
	header := b64url([]byte(`{"alg":"HS256","sign_type":"SIGN"}`))
	payload, err := json.Marshal(map[string]any{
		"api_key":   id,
		"exp":       now + 3600_000,
		"timestamp": now,
	})
	if err != nil {
		return "", err
	}
	mac := hmacSign([]byte(secret), header+"."+b64url(payload))
	return header + "." + b64url(payload) + "." + mac, nil
}
