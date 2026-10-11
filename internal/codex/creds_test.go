package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testLogin = `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"id_token":"id-1","access_token":"access-1","refresh_token":"refresh-1","account_id":"acct-1"},"last_refresh":"2026-10-01T00:00:00Z"}`

// fakeKeychain swaps the Keychain seams for the test and records what was
// written. A nil read result means the item is absent.
type fakeKeychain struct {
	item    []byte
	reads   []string
	writes  map[string][]byte
	readErr error
}

func useFakeKeychain(t *testing.T, item []byte) *fakeKeychain {
	t.Helper()
	fk := &fakeKeychain{item: item, writes: map[string][]byte{}}
	oldRead, oldWrite := readKeychain, writeKeychain
	t.Cleanup(func() { readKeychain, writeKeychain = oldRead, oldWrite })
	readKeychain = func(_ context.Context, account string) ([]byte, error) {
		fk.reads = append(fk.reads, account)
		if fk.readErr != nil {
			return nil, fk.readErr
		}
		if fk.item == nil {
			return nil, errKeychainNotFound
		}
		return fk.item, nil
	}
	writeKeychain = func(_ context.Context, account string, data []byte) error {
		fk.writes[account] = data
		return nil
	}
	return fk
}

// useCodexHome points CODEX_HOME at a fresh directory and returns it.
func useCodexHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	return home
}

func TestLoadAuthPrefersAuthFile(t *testing.T) {
	home := useCodexHome(t)
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(testLogin), 0o600); err != nil {
		t.Fatal(err)
	}
	fk := useFakeKeychain(t, []byte(testLogin))

	auth, err := loadAuth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if auth.Tokens.AccessToken != "access-1" {
		t.Fatalf("access token = %q, want access-1", auth.Tokens.AccessToken)
	}
	if len(fk.reads) != 0 {
		t.Fatalf("Keychain read although auth.json exists: %v", fk.reads)
	}
	if auth.source.keychainAccount != "" {
		t.Fatal("file login marked as a Keychain login")
	}
}

func TestLoadAuthFromKeychain(t *testing.T) {
	home := useCodexHome(t)
	fk := useFakeKeychain(t, []byte(testLogin))

	auth, err := loadAuth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := keychainAccount(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(fk.reads) != 1 || fk.reads[0] != want {
		t.Fatalf("Keychain reads = %v, want [%s]", fk.reads, want)
	}
	if auth.Tokens.AccessToken != "access-1" || auth.Tokens.AccountID != "acct-1" {
		t.Fatalf("tokens not read from the Keychain item: %+v", auth.Tokens)
	}
	if auth.source.keychainAccount != want {
		t.Fatalf("source account = %q, want %q", auth.source.keychainAccount, want)
	}
	if _, err := os.Stat(filepath.Join(home, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("loading created auth.json: %v", err)
	}
}

func TestLoadAuthMissingEverything(t *testing.T) {
	home := useCodexHome(t)
	useFakeKeychain(t, nil)

	_, err := loadAuth(context.Background())
	if err == nil {
		t.Fatal("expected an error with no auth.json and no Keychain item")
	}
	msg := err.Error()
	if !strings.Contains(msg, filepath.Join(home, "auth.json")) || !strings.Contains(msg, "Keychain") || !strings.Contains(msg, "codex login") {
		t.Fatalf("error does not name both stores and the fix: %v", err)
	}
}

func TestLoadAuthKeychainReadErrorIsReturned(t *testing.T) {
	useCodexHome(t)
	fk := useFakeKeychain(t, nil)
	fk.readErr = errKeychainTimeout

	_, err := loadAuth(context.Background())
	if !errors.Is(err, errKeychainTimeout) {
		t.Fatalf("got %v, want errKeychainTimeout", err)
	}
}

func TestLoadAuthKeychainWithoutAccessToken(t *testing.T) {
	useCodexHome(t)
	useFakeKeychain(t, []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-test"}`))

	_, err := loadAuth(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no ChatGPT access token") {
		t.Fatalf("got %v, want the no-access-token error", err)
	}
	if strings.Contains(err.Error(), "sk-test") {
		t.Fatal("error leaked the API key")
	}
}

func TestKeychainAccount(t *testing.T) {
	home := t.TempDir()
	got, err := keychainAccount(home)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(canonical))
	want := "cli|" + hex.EncodeToString(sum[:])[:16]
	if got != want {
		t.Fatalf("account = %q, want %q", got, want)
	}
	if _, err := keychainAccount(filepath.Join(home, "missing")); err == nil {
		t.Fatal("expected an error for a CODEX_HOME that does not exist")
	}
}

func TestPersistWritesRefreshToKeychain(t *testing.T) {
	home := useCodexHome(t)
	fk := useFakeKeychain(t, nil)
	account, err := keychainAccount(home)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := parseAuth([]byte(testLogin), "the Keychain")
	if err != nil {
		t.Fatal(err)
	}
	auth.source = authSource{keychainAccount: account, raw: []byte(testLogin)}
	auth.Tokens.AccessToken = "access-2"
	auth.Tokens.RefreshToken = "refresh-2"
	auth.LastRefresh = "2026-10-11T00:00:00Z"

	if err := persist(auth); err != nil {
		t.Fatal(err)
	}
	written, ok := fk.writes[account]
	if !ok {
		t.Fatalf("refresh not written to the Keychain item %s", account)
	}
	if strings.Contains(string(written), "\n") {
		t.Fatalf("Keychain write is multi-line, which `security -w` reads back hex-encoded: %q", written)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(written, &got); err != nil {
		t.Fatal(err)
	}
	var tok tokens
	if err := json.Unmarshal(got["tokens"], &tok); err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "access-2" || tok.RefreshToken != "refresh-2" || tok.AccountID != "acct-1" {
		t.Fatalf("written tokens = %+v", tok)
	}
	if _, ok := got["auth_mode"]; !ok {
		t.Fatal("unknown auth_mode key was dropped")
	}
	if _, err := os.Stat(filepath.Join(home, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("refresh created auth.json, which undoes keyring storage: %v", err)
	}
}

func TestClassifySecurityError(t *testing.T) {
	notFound := exec.Command("sh", "-c", "exit 44").Run()
	if got := classifySecurityError(notFound, ""); !errors.Is(got, errKeychainNotFound) {
		t.Fatalf("exit 44: got %v, want errKeychainNotFound", got)
	}
	if got := classifySecurityError(errors.New("exit status 1"), "The specified item could not be found in the keychain."); !errors.Is(got, errKeychainNotFound) {
		t.Fatalf("not-found message: got %v, want errKeychainNotFound", got)
	}
	if got := classifySecurityError(errKeychainTimeout, ""); !errors.Is(got, errKeychainTimeout) {
		t.Fatalf("timeout: got %v, want errKeychainTimeout", got)
	}
	other := classifySecurityError(errors.New("exit status 1"), "User interaction is not allowed.")
	if errors.Is(other, errKeychainNotFound) || other == nil {
		t.Fatalf("permission error: got %v", other)
	}
	if !strings.Contains(other.Error(), "User interaction is not allowed.") {
		t.Fatalf("permission error lost its message: %v", other)
	}
}
