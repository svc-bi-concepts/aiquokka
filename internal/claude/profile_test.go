package claude

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeProfile points profileEndpoint at a server answering with status and
// body, and checks the request carries the usage call's auth headers.
func fakeProfile(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != "oauth-2025-04-20" {
			t.Errorf("anthropic-beta = %q", got)
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	original := profileEndpoint
	profileEndpoint = srv.URL
	t.Cleanup(func() { profileEndpoint = original })
}

var cached = &oauth{AccessToken: "tok", SubscriptionType: "max", RateLimitTier: "default_claude_max_5x"}

func TestLivePlanPrefersProfile(t *testing.T) {
	fakeProfile(t, http.StatusOK, `{"account":{"email":"x@example.com"},"organization":{"organization_type":"claude_max","rate_limit_tier":"default_claude_max_20x"}}`)
	if got, want := livePlan(context.Background(), cached), "max/default_claude_max_20x"; got != want {
		t.Fatalf("plan = %q, want %q", got, want)
	}
}

func TestLivePlanFallsBackOnError(t *testing.T) {
	fakeProfile(t, http.StatusUnauthorized, `{"error":"nope"}`)
	if got, want := livePlan(context.Background(), cached), "max/default_claude_max_5x"; got != want {
		t.Fatalf("plan = %q, want %q", got, want)
	}
}

func TestLivePlanFallsBackOnEmptyProfile(t *testing.T) {
	fakeProfile(t, http.StatusOK, `{"organization":{}}`)
	if got, want := livePlan(context.Background(), cached), "max/default_claude_max_5x"; got != want {
		t.Fatalf("plan = %q, want %q", got, want)
	}
}
