package grok

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

// recordingTransport answers every request with the given status and records
// each URL, so a test can see whether aiquokka asked for anything else.
type recordingTransport struct {
	status int
	urls   *[]string
}

func (r recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*r.urls = append(*r.urls, req.URL.String())
	return &http.Response{
		StatusCode: r.status,
		Status:     http.StatusText(r.status),
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Request:    req,
	}, nil
}

// writeAuth puts auth.json in a fresh GROK_HOME and returns its path and bytes.
func writeAuth(t *testing.T, raw string) (string, []byte) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GROK_HOME", home)
	path := filepath.Join(home, "auth.json")
	data := []byte(raw)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

// useTransport routes httpx.Client through rt for the rest of the test.
func useTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	old := httpx.Client
	httpx.Client = &http.Client{Transport: rt}
	t.Cleanup(func() { httpx.Client = old })
}

// TestExpiredTokenIsStaleAndAuthFileUntouched checks that an expired access
// token is reported as stale with no request, and auth.json is left as grok
// wrote it.
func TestExpiredTokenIsStaleAndAuthFileUntouched(t *testing.T) {
	path, raw := writeAuth(t, `{"https://auth.x.ai::1":{"key":"old","refresh_token":"single-use","expires_at":"2001-01-01T00:00:00Z","oidc_client_id":"c"}}`)
	var urls []string
	useTransport(t, recordingTransport{status: http.StatusOK, urls: &urls})

	_, err := Fetch(context.Background())
	if !errors.Is(err, errStale) {
		t.Fatalf("Fetch error = %v, want stale", err)
	}
	if len(urls) != 0 {
		t.Errorf("requests = %v, want none", urls)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("auth.json changed:\n got %s\nwant %s", got, raw)
	}
}

// TestRejectedTokenIsStaleWithoutTokenRequest checks that a 401 on a token
// that looks valid is stale, and that aiquokka never calls the OAuth token
// endpoint.
func TestRejectedTokenIsStaleWithoutTokenRequest(t *testing.T) {
	path, raw := writeAuth(t, `{"https://auth.x.ai::1":{"key":"old","refresh_token":"single-use","expires_at":"2099-01-01T00:00:00Z","oidc_client_id":"c"}}`)
	var urls []string
	useTransport(t, recordingTransport{status: http.StatusUnauthorized, urls: &urls})

	_, err := Fetch(context.Background())
	if !errors.Is(err, errStale) {
		t.Fatalf("Fetch error = %v, want stale", err)
	}
	for _, u := range urls {
		if strings.Contains(u, "oauth2/token") {
			t.Errorf("aiquokka requested the token endpoint: %s", u)
		}
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("auth.json changed:\n got %s\nwant %s", got, raw)
	}
}
