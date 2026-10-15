// Package zai reports GLM Coding Plan usage windows plus Z.ai usage bundles
// and cash balance.
package zai

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/McKean/aiquokka/internal/usage"
)

// loadKey returns the Z.ai API key from $ZAI_API_KEY. Since Benchi's D1
// (2026-10-15) the key lives in Automic Vault and is injected by the blessed
// av wrapper (docs/zai-automic-vault.md); pi's auth store is deliberately not
// consulted — pi never held the key in a shape aiquokka could read. Z.ai keys
// have the form "{id}.{secret}". The key is only ever used for requests — it
// is never printed or logged.
func loadKey() (string, error) {
	if v := strings.TrimSpace(os.Getenv("ZAI_API_KEY")); v != "" {
		return v, nil
	}
	return "", usage.NotConfigured("no Z.ai API key found — set ZAI_API_KEY ({id}.{secret}); on the Mac mini the blessed av wrapper injects it (docs/zai-automic-vault.md)")
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
