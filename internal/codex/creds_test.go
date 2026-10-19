package codex

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McKean/aiquokka/internal/httpx"
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

// unauthorizedTransport answers every request 401 and records each URL, so a
// test can tell whether aiquokka asked for anything beyond the usage read.
type unauthorizedTransport struct{ urls *[]string }

func (u unauthorizedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*u.urls = append(*u.urls, req.URL.String())
	return &http.Response{
		StatusCode: http.StatusUnauthorized,
		Status:     "401 Unauthorized",
		Body:       io.NopCloser(strings.NewReader(`{"error":"token_expired"}`)),
		Request:    req,
	}, nil
}

// TestRejectedTokenIsStaleAndAuthFileUntouched checks that a 401 on the usage
// read gives the stale error after one request, with no refresh and no write
// to auth.json.
func TestRejectedTokenIsStaleAndAuthFileUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"old","refresh_token":"single-use","account_id":"acc"}}`)
	path := filepath.Join(home, "auth.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var urls []string
	old := httpx.Client
	httpx.Client = &http.Client{Transport: unauthorizedTransport{&urls}}
	t.Cleanup(func() { httpx.Client = old })

	_, err := Fetch(context.Background())
	if !errors.Is(err, errStale) {
		t.Fatalf("Fetch error = %v, want stale", err)
	}
	if len(urls) != 1 || urls[0] != usageEndpoint {
		t.Errorf("requests = %v, want only the usage endpoint", urls)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("auth.json changed:\n got %s\nwant %s", got, raw)
	}
}
