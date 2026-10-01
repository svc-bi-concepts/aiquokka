package zai

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/McKean/aiquokka/internal/usage"
)

func TestPiKeyFromPiConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	os.WriteFile(path, []byte(`{"providers":{"zai":{"apiKey":"abc.def"}}}`), 0o600)

	if got := piKey(path); got != "abc.def" {
		t.Fatalf("piKey = %q, want abc.def", got)
	}
}

func TestPiKeyMissing(t *testing.T) {
	if got := piKey(filepath.Join(t.TempDir(), "models.json")); got != "" {
		t.Fatalf("piKey = %q, want empty", got)
	}
}

func TestMintJWTShape(t *testing.T) {
	token, err := mintJWT("myid.mysecret")
	if err != nil {
		t.Fatalf("mintJWT: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}
	decode := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatalf("decoding segment: %v", err)
		}
		return b
	}

	var header struct {
		Alg      string `json:"alg"`
		SignType string `json:"sign_type"`
	}
	if err := json.Unmarshal(decode(parts[0]), &header); err != nil {
		t.Fatalf("header: %v", err)
	}
	if header.Alg != "HS256" || header.SignType != "SIGN" {
		t.Fatalf("header = %+v, want HS256/SIGN", header)
	}

	var payload struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(decode(parts[1]), &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.APIKey != "myid" {
		t.Fatalf("api_key = %q, want myid", payload.APIKey)
	}

	mac := hmac.New(sha256.New, []byte("mysecret"))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != want {
		t.Fatalf("signature mismatch: got %q, want %q", parts[2], want)
	}
}

func TestMintJWTRejectsMalformedKey(t *testing.T) {
	for _, key := range []string{"", "no-dot", ".secret", "id."} {
		if _, err := mintJWT(key); err == nil {
			t.Errorf("mintJWT(%q) succeeded, want error", key)
		}
	}
}

func TestReportFromBundles(t *testing.T) {
	bundles := []bundle{{
		ResourcePackageName: "20 million GLM-5.3 trial packs",
		SuitableModel:       "glm-5.3",
		Status:              "EFFECTIVE",
		TokensMagnitude:     20_000_000,
		AvailableBalance:    18_000_000,
		ExpirationTime:      "2026-11-28T15:34:31",
	}}

	report := reportFromBundles(bundles)

	if report.Provider != "Z.ai" {
		t.Fatalf("Provider = %q, want Z.ai", report.Provider)
	}
	if len(report.Windows) != 1 {
		t.Fatalf("Windows = %d, want 1", len(report.Windows))
	}
	w := report.Windows[0]
	if w.Label != "Bundle" {
		t.Fatalf("Label = %q, want Bundle", w.Label)
	}
	if w.Used == nil || *w.Used != 2_000_000 {
		t.Fatalf("Used = %v, want 2000000", w.Used)
	}
	if w.Limit == nil || *w.Limit != 20_000_000 {
		t.Fatalf("Limit = %v, want 20000000", w.Limit)
	}
	if len(report.Extra) != 3 {
		t.Fatalf("Extra = %d, want 3", len(report.Extra))
	}
	if report.Extra[1].Label != "Applies to" || report.Extra[1].Value != "glm-5.3" {
		t.Fatalf("Extra[1] = %+v, want Applies to glm-5.3", report.Extra[1])
	}
	if report.Extra[2].Label != "Expires" || report.Extra[2].Value != "2026-11-28" {
		t.Fatalf("Extra[2] = %+v, want Expires 2026-11-28", report.Extra[2])
	}
}

func TestReportFromBundlesSkipsIneffective(t *testing.T) {
	bundles := []bundle{
		{Status: "EXPIRED", TokensMagnitude: 100, AvailableBalance: 0},
		{Status: "effective", TokensMagnitude: 50, AvailableBalance: 25},
	}
	report := reportFromBundles(bundles)
	if len(report.Windows) != 1 {
		t.Fatalf("Windows = %d, want 1", len(report.Windows))
	}
	if report.Windows[0].Limit == nil || *report.Windows[0].Limit != 50 {
		t.Fatalf("Limit = %v, want 50", report.Windows[0].Limit)
	}
}

func TestReportFromBundlesMultipleGetsModelLabel(t *testing.T) {
	bundles := []bundle{
		{Status: "EFFECTIVE", SuitableModel: "glm-5.3", TokensMagnitude: 100, AvailableBalance: 90},
		{Status: "EFFECTIVE", SuitableModel: "glm-5.3-flash", TokensMagnitude: 200, AvailableBalance: 100},
	}
	report := reportFromBundles(bundles)
	if len(report.Windows) != 2 {
		t.Fatalf("Windows = %d, want 2", len(report.Windows))
	}
	if report.Windows[0].Label != "Bundle (glm-5.3)" {
		t.Fatalf("Label = %q, want Bundle (glm-5.3)", report.Windows[0].Label)
	}
	if report.Windows[1].Label != "Bundle (glm-5.3-flash)" {
		t.Fatalf("Label = %q, want Bundle (glm-5.3-flash)", report.Windows[1].Label)
	}
}

func TestAppendCash(t *testing.T) {
	report := reportFromBundles(nil)
	appendCash(report, &accountData{
		Balance:          2.95,
		RechargeAmount:   3.0,
		GiveAmount:       0.0,
		TotalSpendAmount: 0.05,
	})

	if len(report.Windows) != 1 {
		t.Fatalf("Windows = %d, want 1", len(report.Windows))
	}
	w := report.Windows[0]
	if w.Label != "Cash" {
		t.Fatalf("Label = %q, want Cash", w.Label)
	}
	if w.Remaining == nil || *w.Remaining != 2.95 {
		t.Fatalf("Remaining = %v, want 2.95", w.Remaining)
	}
	if w.Currency != "USD" {
		t.Fatalf("Currency = %q, want USD (api.z.ai default)", w.Currency)
	}
	// Granted is zero-amount, so only Topped up and Spent appear.
	if len(report.Extra) != 2 {
		t.Fatalf("Extra = %d, want 2", len(report.Extra))
	}
	if report.Extra[0].Value != "$3.00" || report.Extra[1].Value != "$0.05" {
		t.Fatalf("Extra = %+v, want Topped up $3.00 and Spent $0.05", report.Extra)
	}
}

func TestAppendCashNil(t *testing.T) {
	report := reportFromBundles(nil)
	appendCash(report, nil)
	if len(report.Windows) != 0 {
		t.Fatalf("Windows = %d, want 0", len(report.Windows))
	}
}

func TestAuthKeyFromPiAuthJson(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	os.WriteFile(path, []byte(`{"zai":{"type":"api_key","key":"abc.def"},"openrouter":{"type":"api_key","key":"other"}}`), 0o600)

	if got := authKey(path); got != "abc.def" {
		t.Fatalf("authKey = %q, want abc.def", got)
	}
}

func TestAuthKeyMissingOrNonApiKey(t *testing.T) {
	dir := t.TempDir()
	if got := authKey(filepath.Join(dir, "auth.json")); got != "" {
		t.Fatalf("authKey on missing file = %q, want empty", got)
	}
	path := filepath.Join(dir, "auth.json")
	os.WriteFile(path, []byte(`{"zai":{"type":"oauth","key":"abc.def"}}`), 0o600)
	if got := authKey(path); got != "" {
		t.Fatalf("authKey on oauth entry = %q, want empty", got)
	}
}

// TestLoadKeyOrder pins the credential precedence: env var, then
// ~/.pi/agent/auth.json, then ~/.pi/agent/models.json — all against a temp
// HOME so the real configuration is never touched.
func TestLoadKeyOrder(t *testing.T) {
	home := t.TempDir()
	piAgent := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(piAgent, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(piAgent, "auth.json"), []byte(`{"zai":{"type":"api_key","key":"auth.id"}}`), 0o600)
	os.WriteFile(filepath.Join(piAgent, "models.json"), []byte(`{"providers":{"zai":{"apiKey":"models.id"}}}`), 0o600)

	restore := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = restore })

	t.Run("env wins", func(t *testing.T) {
		t.Setenv("ZAI_API_KEY", "env.id")
		key, err := loadKey()
		if err != nil || key != "env.id" {
			t.Fatalf("loadKey = %q, %v; want env.id", key, err)
		}
	})
	t.Run("auth.json before models.json", func(t *testing.T) {
		t.Setenv("ZAI_API_KEY", "")
		key, err := loadKey()
		if err != nil || key != "auth.id" {
			t.Fatalf("loadKey = %q, %v; want auth.id", key, err)
		}
	})
	t.Run("models.json fallback", func(t *testing.T) {
		t.Setenv("ZAI_API_KEY", "")
		if err := os.Remove(filepath.Join(piAgent, "auth.json")); err != nil {
			t.Fatal(err)
		}
		key, err := loadKey()
		if err != nil || key != "models.id" {
			t.Fatalf("loadKey = %q, %v; want models.id", key, err)
		}
	})
	t.Run("nothing configured", func(t *testing.T) {
		t.Setenv("ZAI_API_KEY", "")
		if err := os.Remove(filepath.Join(piAgent, "models.json")); err != nil {
			t.Fatal(err)
		}
		_, err := loadKey()
		if !usage.IsNotConfigured(err) {
			t.Fatalf("loadKey error = %v, want NotConfigured", err)
		}
	})
}

func TestQuotaWindowFiveHour(t *testing.T) {
	w := quotaWindow(quotaLimit{
		Type: "CREDIT_LIMIT", Unit: 3, Number: 5,
		Usage: 12000, CurrentValue: 176, Remaining: 11823,
		Percentage: 1, NextResetTime: 1790863518271,
	})
	if w == nil {
		t.Fatal("quotaWindow = nil, want a window")
	}
	if w.Label != "5h" {
		t.Fatalf("Label = %q, want 5h", w.Label)
	}
	wantPct := float64(176) / 12000 * 100
	if w.UsedPercent == nil || *w.UsedPercent != wantPct {
		t.Fatalf("UsedPercent = %v, want %v (recomputed, not the truncated API value)", w.UsedPercent, wantPct)
	}
	if w.Used == nil || *w.Used != 176 || w.Limit == nil || *w.Limit != 12000 {
		t.Fatalf("Used/Limit = %v/%v, want 176/12000 credits", w.Used, w.Limit)
	}
	if w.Duration != 5*time.Hour {
		t.Fatalf("Duration = %v, want 5h", w.Duration)
	}
	if !w.ResetsAt.Equal(time.UnixMilli(1790863518271)) {
		t.Fatalf("ResetsAt = %v, want epoch-ms 1790863518271", w.ResetsAt)
	}
}

func TestQuotaWindowWeeklyAndMCPPrefix(t *testing.T) {
	week := quotaWindow(quotaLimit{Type: "CREDIT_LIMIT", Unit: 6, Number: 1, Usage: 60000, CurrentValue: 5288, NextResetTime: 1791382419984})
	if week == nil || week.Label != "Weekly" {
		t.Fatalf("window = %+v, want Weekly", week)
	}
	if week.Duration != 7*24*time.Hour {
		t.Fatalf("Duration = %v, want 7d", week.Duration)
	}

	mcp := quotaWindow(quotaLimit{Type: "MCP_LIMIT", Unit: 3, Number: 5, Usage: 100, CurrentValue: 0})
	if mcp == nil || mcp.Label != "MCP 5h" {
		t.Fatalf("window = %+v, want label 'MCP 5h'", mcp)
	}
	if mcp.UsedPercent == nil || *mcp.UsedPercent != 0 {
		t.Fatalf("UsedPercent = %v, want 0 (empty window still reports)", mcp.UsedPercent)
	}

	if got := quotaWindow(quotaLimit{Type: "CREDIT_LIMIT", Unit: 9, Usage: 0, Percentage: 0}); got != nil {
		t.Fatalf("quotaWindow on zero allowance = %+v, want nil", got)
	}
}

func TestAppendQuota(t *testing.T) {
	report := &usage.Report{Provider: "Z.ai"}
	appendQuota(report, &quotaData{
		Level: "pro",
		Limits: []quotaLimit{
			{Type: "CREDIT_LIMIT", Unit: 3, Number: 5, Usage: 12000, CurrentValue: 176, NextResetTime: 1790863518271},
			{Type: "CREDIT_LIMIT", Unit: 6, Number: 1, Usage: 60000, CurrentValue: 5288},
		},
	})
	if report.Plan != "pro" {
		t.Fatalf("Plan = %q, want pro", report.Plan)
	}
	if len(report.Windows) != 2 || report.Windows[0].Label != "5h" || report.Windows[1].Label != "Weekly" {
		t.Fatalf("Windows = %+v, want 5h then Weekly", report.Windows)
	}

	// No Coding Plan: clear fact instead of failure.
	noPlan := &usage.Report{Provider: "Z.ai"}
	appendQuota(noPlan, &quotaData{})
	if len(noPlan.Windows) != 0 || len(noPlan.Extra) != 1 {
		t.Fatalf("report = %+v, want no windows and one fact", noPlan)
	}
	if noPlan.Extra[0].Label != "Coding plan" || !strings.Contains(noPlan.Extra[0].Value, "no GLM Coding Plan") {
		t.Fatalf("fact = %+v, want a clear no-Coding-Plan statement", noPlan.Extra[0])
	}

	appendQuota(noPlan, nil) // nil quota (fetch failed) adds nothing here
	if len(noPlan.Extra) != 1 {
		t.Fatalf("Extra = %d after nil quota, want 1", len(noPlan.Extra))
	}
}

// zaiTestServer serves the three Z.ai endpoints for Fetch tests. quotaStatus
// lets a test force the quota call to fail.
func zaiTestServer(t *testing.T, quotaBody string, quotaStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/monitor/usage/quota/limit":
			// The monitor endpoint takes the raw key, not the console JWT.
			if r.Header.Get("Authorization") != "Bearer id.secret" {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"success":false,"msg":"bad auth"}`)
				return
			}
			w.WriteHeader(quotaStatus)
			fmt.Fprint(w, quotaBody)
		case "/biz/tokenAccounts/list/my":
			fmt.Fprint(w, `{"rows":[]}`)
		case "/biz/account/query-customer-account-report":
			fmt.Fprint(w, `{"code":200,"success":true,"msg":"ok","data":{"balance":2.95}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestFetchCodingPlanWindows(t *testing.T) {
	srv := zaiTestServer(t, `{"code":200,"msg":"Operation successful","data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"usage":12000,"currentValue":176,"remaining":11823,"percentage":1,"nextResetTime":1790863518271},{"type":"CREDIT_LIMIT","unit":6,"number":1,"usage":60000,"currentValue":5288,"remaining":54711,"percentage":8,"nextResetTime":1791382419984}],"level":"pro"},"success":true}`, http.StatusOK)
	defer srv.Close()
	t.Setenv("ZAI_BASE_URL", srv.URL)
	t.Setenv("ZAI_API_KEY", "id.secret")

	report, err := Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if report.Plan != "pro" {
		t.Fatalf("Plan = %q, want pro", report.Plan)
	}
	// 5h + Weekly quota windows first, then the cash balance window.
	if len(report.Windows) != 3 {
		t.Fatalf("Windows = %d (%+v), want 3", len(report.Windows), report.Windows)
	}
	if report.Windows[0].Label != "5h" || report.Windows[1].Label != "Weekly" || report.Windows[2].Label != "Cash" {
		t.Fatalf("window order = %q, %q, %q; want 5h, Weekly, Cash",
			report.Windows[0].Label, report.Windows[1].Label, report.Windows[2].Label)
	}
	if !report.Windows[0].ResetsAt.Equal(time.UnixMilli(1790863518271)) {
		t.Fatalf("ResetsAt = %v, want epoch-ms 1790863518271", report.Windows[0].ResetsAt)
	}
}

func TestFetchNoCodingPlanIsNotAnError(t *testing.T) {
	srv := zaiTestServer(t, `{"code":200,"msg":"Operation successful","data":{"limits":[],"level":""},"success":true}`, http.StatusOK)
	defer srv.Close()
	t.Setenv("ZAI_BASE_URL", srv.URL)
	t.Setenv("ZAI_API_KEY", "id.secret")

	report, err := Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(report.Windows) != 1 || report.Windows[0].Label != "Cash" {
		t.Fatalf("Windows = %+v, want only Cash", report.Windows)
	}
	found := false
	for _, f := range report.Extra {
		if f.Label == "Coding plan" && strings.Contains(f.Value, "no GLM Coding Plan") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Extra = %+v, want a Coding plan: none fact", report.Extra)
	}
}

func TestFetchQuotaFailureIsNonFatal(t *testing.T) {
	srv := zaiTestServer(t, `{"error":"boom"}`, http.StatusInternalServerError)
	defer srv.Close()
	t.Setenv("ZAI_BASE_URL", srv.URL)
	t.Setenv("ZAI_API_KEY", "id.secret")

	report, err := Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v (quota failures must not fail the report)", err)
	}
	found := false
	for _, f := range report.Extra {
		if f.Label == "Coding plan" && strings.HasPrefix(f.Value, "unavailable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Extra = %+v, want a Coding plan: unavailable fact", report.Extra)
	}
}
