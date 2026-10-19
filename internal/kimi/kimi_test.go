package kimi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McKean/aiquokka/internal/httpx"
)

func TestToWindowSetsPercentageAndKeepsCounts(t *testing.T) {
	w := toWindow("Weekly", row{
		Used:  num{set: true, v: 78},
		Limit: num{set: true, v: 100},
	})
	if w == nil {
		t.Fatal("toWindow() returned nil")
	}
	if w.Used == nil || *w.Used != 78 || w.Limit == nil || *w.Limit != 100 {
		t.Fatalf("absolute counts not preserved: %#v", w)
	}
	if w.UsedPercent == nil || math.Abs(*w.UsedPercent-78) > 0.000001 {
		t.Fatalf("UsedPercent = %v, want 78", w.UsedPercent)
	}
}

func TestToWindowDerivesPercentageFromRemaining(t *testing.T) {
	w := toWindow("5h", row{
		Limit:     num{set: true, v: 100},
		Remaining: num{set: true, v: 85},
	})
	if w == nil || w.Used == nil || *w.Used != 15 {
		t.Fatalf("derived usage = %#v, want 15", w)
	}
	if w.UsedPercent == nil || math.Abs(*w.UsedPercent-15) > 0.000001 {
		t.Fatalf("UsedPercent = %v, want 15", w.UsedPercent)
	}
}

// writeOAuth puts a kimi CLI OAuth credentials file in a fresh share dir and
// returns its path and bytes. KIMI_API_KEY is cleared so the file is used.
func writeOAuth(t *testing.T, raw string) (string, []byte) {
	t.Helper()
	t.Setenv("KIMI_API_KEY", "")
	t.Setenv("KIMI_CODING_API_KEY", "")
	share := t.TempDir()
	t.Setenv("KIMI_SHARE_DIR", share)
	if err := os.MkdirAll(filepath.Join(share, "credentials"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(share, "credentials", "kimi-code.json")
	data := []byte(raw)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

// statusTransport answers every request with the given status and records each
// URL, so a test can see whether aiquokka asked for anything else.
type statusTransport struct {
	status int
	urls   *[]string
}

func (s statusTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*s.urls = append(*s.urls, req.URL.String())
	return &http.Response{
		StatusCode: s.status,
		Status:     http.StatusText(s.status),
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Request:    req,
	}, nil
}

func useTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	old := httpx.Client
	httpx.Client = &http.Client{Transport: rt}
	t.Cleanup(func() { httpx.Client = old })
}

// TestExpiredOAuthTokenIsStaleAndFileUntouched checks that an expired OAuth
// token is reported as stale with no request, and the kimi CLI's credentials
// file is left byte-for-byte as it was.
func TestExpiredOAuthTokenIsStaleAndFileUntouched(t *testing.T) {
	path, raw := writeOAuth(t, `{"access_token":"old","refresh_token":"single-use","expires_at":1}`)
	var urls []string
	useTransport(t, statusTransport{status: http.StatusOK, urls: &urls})

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
		t.Errorf("credentials file changed:\n got %s\nwant %s", got, raw)
	}
}

// TestRejectedOAuthTokenIsStaleWithoutTokenRequest checks that a 401 on a
// token that has not expired is stale, and aiquokka never calls the OAuth
// token endpoint.
func TestRejectedOAuthTokenIsStaleWithoutTokenRequest(t *testing.T) {
	path, raw := writeOAuth(t, `{"access_token":"old","refresh_token":"single-use","expires_at":4102444800}`)
	var urls []string
	useTransport(t, statusTransport{status: http.StatusUnauthorized, urls: &urls})

	_, err := Fetch(context.Background())
	if !errors.Is(err, errStale) {
		t.Fatalf("Fetch error = %v, want stale", err)
	}
	for _, u := range urls {
		if strings.Contains(u, "oauth/token") {
			t.Errorf("aiquokka requested the token endpoint: %s", u)
		}
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("credentials file changed:\n got %s\nwant %s", got, raw)
	}
}

// TestExpiredOAuthTokenWithoutRefreshTokenIsStale checks that an OAuth file
// with no refresh token is still checked for expiry before any request.
func TestExpiredOAuthTokenWithoutRefreshTokenIsStale(t *testing.T) {
	writeOAuth(t, `{"access_token":"old","expires_at":1}`)
	var urls []string
	useTransport(t, statusTransport{status: http.StatusOK, urls: &urls})

	_, err := Fetch(context.Background())
	if !errors.Is(err, errStale) {
		t.Fatalf("Fetch error = %v, want stale", err)
	}
	if len(urls) != 0 {
		t.Errorf("requests = %v, want none", urls)
	}
}
