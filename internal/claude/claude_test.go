package claude

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McKean/aiquokka/internal/httpx"
)

// refuseNetwork fails the test on any request, so a stale token cannot reach
// Anthropic for a refresh or a usage read.
func refuseNetwork(t *testing.T) {
	t.Helper()
	old := httpx.Client
	httpx.Client = &http.Client{Transport: refusedTransport{t}}
	t.Cleanup(func() { httpx.Client = old })
}

type refusedTransport struct{ t *testing.T }

func (r refusedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.t.Errorf("unexpected request to %s", req.URL)
	return nil, errors.New("network refused in test")
}

// TestFetchExpiredFileTokenIsStaleAndFileUntouched checks that an expired
// token in .credentials.json gives the stale error, makes no request, and
// leaves the file byte-for-byte as Claude Code wrote it.
func TestFetchExpiredFileTokenIsStaleAndFileUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", ".credentials.json")
	raw := []byte(`{"claudeAiOauth":{"accessToken":"old","refreshToken":"single-use","expiresAt":1}}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	refuseNetwork(t)

	_, err := Fetch(context.Background())
	if !errors.Is(err, errStale) {
		t.Fatalf("Fetch error = %v, want stale", err)
	}
	if !strings.Contains(err.Error(), "run `claude`") {
		t.Errorf("message = %q, want the claude hint", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("credentials file changed:\n got %s\nwant %s", got, raw)
	}
}

// TestFetchExpiredKeychainTokenIsStale checks the same for a token read from
// the Keychain: stale, no request, and no Keychain write (the write path no
// longer exists, so only the read seam is stubbed here).
func TestFetchExpiredKeychainTokenIsStale(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	original := readKeychainCredentials
	t.Cleanup(func() { readKeychainCredentials = original })
	readKeychainCredentials = func(context.Context) ([]byte, error) {
		return []byte(`{"accessToken":"old","refreshToken":"single-use","expiresAt":1}`), nil
	}
	refuseNetwork(t)

	_, err := Fetch(context.Background())
	if !errors.Is(err, errStale) {
		t.Fatalf("Fetch error = %v, want stale", err)
	}
}
