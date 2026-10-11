package codex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/McKean/aiquokka/internal/usage"
)

// standIn writes a fake `codex` into a fresh directory and points PATH at only
// that directory, so the test never reaches the real codex or its login.
func standIn(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if script != "" {
		path := filepath.Join(dir, "codex")
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

// appServerScript answers like `codex app-server`: it checks that the three
// requests arrive in order, then writes a notification before the responses.
// It exits non-zero when a request is not the one aiquokka sends. It uses only
// shell builtins, because standIn leaves PATH with nothing else on it.
const appServerScript = `#!/bin/sh
read -r init
read -r initialized
read -r req
case "$init$initialized$req" in
  *'"method":"initialize"'*'"method":"initialized"'*'"method":"account/rateLimits/read"'*) ;;
  *) exit 1 ;;
esac
printf '%s\n' \
  '{"method":"remoteControl/status/changed","params":{"status":"disabled"}}' \
  '{"id":1,"result":{"userAgent":"aiquokka/0.162.1","codexHome":"/tmp/codex","platformFamily":"unix","platformOs":"macos"}}' \
  '{"id":2,"result":{"ordinaryUsageAllowed":true,"rateLimits":{"limitId":"codex","primary":{"usedPercent":12,"windowDurationMins":300,"resetsAt":1792295892},"secondary":{"usedPercent":40,"windowDurationMins":10080,"resetsAt":1792900000},"credits":{"hasCredits":true,"unlimited":false,"balance":"4.50"},"planType":"plus"},"rateLimitResetCredits":{"availableCount":2,"credits":null}}}'
`

func TestFetchFromAppServerWithoutAuthFile(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	standIn(t, appServerScript)

	report, err := Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if report.Plan != "plus" {
		t.Errorf("Plan = %q, want plus", report.Plan)
	}
	if len(report.Windows) != 2 {
		t.Fatalf("got %d windows, want 2: %+v", len(report.Windows), report.Windows)
	}
	five := report.Windows[0]
	if five.Label != "5h" || five.Duration != 5*time.Hour {
		t.Errorf("first window = %q %v, want 5h 5h0m0s", five.Label, five.Duration)
	}
	if five.UsedPercent == nil || *five.UsedPercent != 12 {
		t.Errorf("5h used = %v, want 12", five.UsedPercent)
	}
	if want := time.Unix(1792295892, 0); !five.ResetsAt.Equal(want) {
		t.Errorf("5h resets = %v, want %v", five.ResetsAt, want)
	}
	weekly := report.Windows[1]
	if weekly.Label != "Weekly" || weekly.Duration != 7*24*time.Hour {
		t.Errorf("second window = %q %v, want Weekly 168h0m0s", weekly.Label, weekly.Duration)
	}
	want := []usage.Fact{{Label: "Resets", Value: "2"}, {Label: "Credits", Value: "4.50"}}
	if len(report.Extra) != len(want) {
		t.Fatalf("Extra = %+v, want %+v", report.Extra, want)
	}
	for i := range want {
		if report.Extra[i] != want[i] {
			t.Errorf("Extra[%d] = %+v, want %+v", i, report.Extra[i], want[i])
		}
	}
}

func TestFetchNoLoginAndNoCodex(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	standIn(t, "")

	_, err := Fetch(context.Background())
	if !usage.IsNotConfigured(err) {
		t.Fatalf("Fetch error = %v, want not configured", err)
	}
	if !strings.Contains(err.Error(), "run `codex login` first") {
		t.Errorf("message = %q, want the login hint", err)
	}
}

func TestFetchAppServerFailureIsNotConfigured(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	standIn(t, "#!/bin/sh\nexit 1\n")

	_, err := Fetch(context.Background())
	if !usage.IsNotConfigured(err) {
		t.Fatalf("Fetch error = %v, want not configured", err)
	}
}

// TestFetchAuthFileFirst checks that an auth.json login is used and codex is
// not asked: the stand-in codex fails if it is called.
func TestFetchAuthFileFirst(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":7,"limit_window_seconds":18000,"reset_after_seconds":600}}}`))
	}))
	defer srv.Close()
	old := usageEndpoint
	usageEndpoint = srv.URL
	t.Cleanup(func() { usageEndpoint = old })

	home := t.TempDir()
	auth := `{"tokens":{"access_token":"tok-1","account_id":"acc-1"}}`
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", home)
	standIn(t, "#!/bin/sh\nexit 1\n")

	report, err := Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotAuth != "Bearer tok-1" {
		t.Errorf("Authorization = %q, want Bearer tok-1", gotAuth)
	}
	if report.Plan != "pro" || len(report.Windows) != 1 || report.Windows[0].Label != "5h" {
		t.Errorf("report = %+v, want pro with a 5h window", report)
	}
}

// TestFetchSilentAppServerIsKilled checks that a codex that reads its requests
// and never answers is killed at the timeout, so Fetch returns instead of
// hanging on launchd. The stand-in blocks on stdin, which only the kill ends.
func TestFetchSilentAppServerIsKilled(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	standIn(t, "#!/bin/sh\nwhile :; do read -r line || :; done\n")
	old := appServerTimeout
	appServerTimeout = 200 * time.Millisecond
	t.Cleanup(func() { appServerTimeout = old })

	start := time.Now()
	_, err := Fetch(context.Background())
	if !usage.IsNotConfigured(err) {
		t.Fatalf("Fetch error = %v, want not configured", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Fetch took %v, want it cut off near the timeout", d)
	}
}
