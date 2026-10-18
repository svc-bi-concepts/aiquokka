package codex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadAuthReadsAuthFile checks that loadAuth reads the login from
// auth.json.
func TestLoadAuthReadsAuthFile(t *testing.T) {
	home := t.TempDir()
	auth := `{"tokens":{"access_token":"file-tok","account_id":"acc-1"},"auth_mode":"chatgpt"}`
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", home)

	af, err := loadAuth()
	if err != nil {
		t.Fatalf("loadAuth: %v", err)
	}
	if af.Tokens.AccessToken != "file-tok" || af.Tokens.AccountID != "acc-1" {
		t.Errorf("tokens = %+v, want the auth.json login", af.Tokens)
	}
}

// TestLoadAuthMissingFileGivesErrNoAuthFile checks that with no auth.json
// loadAuth reports the missing file, so fetchUsage falls back to codex.
func TestLoadAuthMissingFileGivesErrNoAuthFile(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())

	if _, err := loadAuth(); !errors.Is(err, errNoAuthFile) {
		t.Fatalf("loadAuth error = %v, want errNoAuthFile", err)
	}
}

// TestPersistWritesBackToAuthFile checks that a refreshed login is written
// back to auth.json with unknown keys kept and 0600 permissions.
func TestPersistWritesBackToAuthFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	original := []byte(`{
  "auth_mode": "chatgpt",
  "OPENAI_API_KEY": null,
  "tokens": {"access_token": "old", "refresh_token": "r1"},
  "last_refresh": "2026-10-01T00:00:00Z"
}`)
	path := filepath.Join(home, "auth.json")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	auth := &authFile{
		Tokens:      tokens{IDToken: "id-2", AccessToken: "new", RefreshToken: "r2", AccountID: "acc"},
		LastRefresh: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	if err := persist(auth); err != nil {
		t.Fatalf("persist: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("written file is not JSON: %v\n%s", err, data)
	}
	var tok tokens
	if err := json.Unmarshal(raw["tokens"], &tok); err != nil {
		t.Fatalf("tokens field: %v", err)
	}
	if tok.AccessToken != "new" || tok.RefreshToken != "r2" || tok.AccountID != "acc" {
		t.Errorf("written tokens = %+v, want the refreshed set", tok)
	}
	if string(raw["auth_mode"]) != `"chatgpt"` || string(raw["OPENAI_API_KEY"]) != "null" {
		t.Errorf("unknown keys not preserved: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}
