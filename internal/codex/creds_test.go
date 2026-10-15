package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadAuthFileWinsOverKeychain checks that an auth.json login wins even
// when a Keychain item exists.
func TestLoadAuthFileWinsOverKeychain(t *testing.T) {
	home := t.TempDir()
	auth := `{"tokens":{"access_token":"file-tok"}}`
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", home)
	stubKeychainSeams(t, []byte(`{"tokens":{"access_token":"keychain-tok"}}`))

	af, err := loadAuth(context.Background())
	if err != nil {
		t.Fatalf("loadAuth: %v", err)
	}
	if af.Tokens.AccessToken != "file-tok" {
		t.Errorf("access token = %q, want file-tok", af.Tokens.AccessToken)
	}
	if len(af.source.keychainItem) != 0 {
		t.Errorf("source = keychain, want the file")
	}
}

// TestLoadAuthFromKeychainWhenFileAbsent checks that the Keychain item fills in
// for a missing auth.json and carries the item so a refresh writes back there.
func TestLoadAuthFromKeychainWhenFileAbsent(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	stubKeychainSeams(t, []byte(`{"tokens":{"access_token":"kc-tok","refresh_token":"kc-r"},"auth_mode":"chatgpt"}`))

	af, err := loadAuth(context.Background())
	if err != nil {
		t.Fatalf("loadAuth: %v", err)
	}
	if af.Tokens.AccessToken != "kc-tok" || af.Tokens.RefreshToken != "kc-r" {
		t.Errorf("tokens = %+v, want the Keychain login", af.Tokens)
	}
	if string(af.source.keychainItem) != "cli|test" {
		t.Errorf("source item = %q, want cli|test", af.source.keychainItem)
	}
}

// TestLoadAuthNeitherStoreGivesErrNoAuthFile checks that with no auth.json and
// no Keychain item loadAuth reports the missing file, so fetchUsage falls back
// to codex.
func TestLoadAuthNeitherStoreGivesErrNoAuthFile(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	stubKeychainSeams(t, nil)

	if _, err := loadAuth(context.Background()); !errors.Is(err, errNoAuthFile) {
		t.Fatalf("loadAuth error = %v, want errNoAuthFile", err)
	}
}

// TestLoadAuthKeychainTimeoutHasHint checks that a Keychain read that blocks on
// a dialog surfaces the timeout with the prompt hint instead of falling through.
func TestLoadAuthKeychainTimeoutHasHint(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	readAuthSaved := readKeychainAuth
	readKeychainAuth = func(context.Context) ([]byte, []byte, error) {
		// What the darwin reader produces when security blocks too long.
		return nil, nil, classifySecurityError(errKeychainTimeout, "")
	}
	t.Cleanup(func() { readKeychainAuth = readAuthSaved })

	_, err := loadAuth(context.Background())
	if !errors.Is(err, errKeychainTimeout) {
		t.Fatalf("loadAuth error = %v, want errKeychainTimeout", err)
	}
	if errors.Is(err, errNoAuthFile) {
		t.Fatal("a Keychain timeout must not fall through to the codex fallback")
	}
	if !strings.Contains(err.Error(), keychainPromptHint) {
		t.Errorf("error = %v, want the prompt hint", err)
	}
}

// TestPersistKeychainSourceWritesBackToKeychain checks that a login read from
// the Keychain is refreshed in place: the same item is updated with the new
// tokens and unknown keys are kept, and no auth.json is created.
func TestPersistKeychainSourceWritesBackToKeychain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	original := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"old","refresh_token":"r1"},"OPENAI_API_KEY":null}`)
	var gotItem, gotData []byte
	readItemSaved, updateSaved := readKeychainItem, updateKeychainItem
	readKeychainItem = func(context.Context, []byte) ([]byte, error) {
		return original, nil
	}
	updateKeychainItem = func(_ context.Context, item, data []byte) error {
		gotItem, gotData = item, data
		return nil
	}
	t.Cleanup(func() { readKeychainItem, updateKeychainItem = readItemSaved, updateSaved })

	auth := &authFile{
		Tokens:      tokens{IDToken: "id-2", AccessToken: "new", RefreshToken: "r2", AccountID: "acc"},
		LastRefresh: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	auth.source = credentialSource{keychainItem: []byte("cli|abcd")}
	if err := persist(context.Background(), auth); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if string(gotItem) != "cli|abcd" {
		t.Errorf("updated item = %q, want cli|abcd (the item it came from)", gotItem)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(gotData, &raw); err != nil {
		t.Fatalf("written payload is not JSON: %v\n%s", err, gotData)
	}
	var tok tokens
	if err := json.Unmarshal(raw["tokens"], &tok); err != nil {
		t.Fatalf("tokens field: %v", err)
	}
	if tok.AccessToken != "new" || tok.RefreshToken != "r2" || tok.AccountID != "acc" {
		t.Errorf("written tokens = %+v, want the refreshed set", tok)
	}
	if string(raw["auth_mode"]) != `"chatgpt"` || string(raw["OPENAI_API_KEY"]) != "null" {
		t.Errorf("unknown keys not preserved: %s", gotData)
	}
	if _, err := os.Stat(filepath.Join(home, "auth.json")); !os.IsNotExist(err) {
		t.Errorf("auth.json exists (err %v), want no file written", err)
	}
}

// TestStoreKeyMatchesCodex pins the account format codex-cli derives from
// CODEX_HOME: "cli|" plus the first 16 hex characters of the SHA-256 of the
// canonicalized home, and the same key for a path through a symlink.
func TestStoreKeyMatchesCodex(t *testing.T) {
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(resolved))
	want := "cli|" + hex.EncodeToString(sum[:])[:16]

	if got := storeKey(dir); got != want {
		t.Errorf("storeKey(%q) = %q, want %q", dir, got, want)
	}

	link := filepath.Join(t.TempDir(), "codex-home")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if got := storeKey(link); got != want {
		t.Errorf("storeKey(symlink) = %q, want %q", got, want)
	}
}
