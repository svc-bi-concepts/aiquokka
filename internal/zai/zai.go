package zai

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/McKean/aiquokka/internal/httpx"
	"github.com/McKean/aiquokka/internal/usage"
)

// baseURL returns the Z.ai API base URL (override via ZAI_BASE_URL). The
// China platform (open.bigmodel.cn) exposes the same endpoints.
func baseURL() string {
	if v := os.Getenv("ZAI_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.z.ai/api"
}

// tokenAccountsResponse mirrors GET /biz/tokenAccounts/list/my — the
// prepaid usage bundles ("resource packages") attached to the account.
type tokenAccountsResponse struct {
	Rows []bundle `json:"rows"`
}

// bundle is one prepaid token bundle. Amounts are token counts.
type bundle struct {
	ResourcePackageName string `json:"resourcePackageName"`
	SuitableModel       string `json:"suitableModel"`
	SuitableScene       string `json:"suitableScene"`
	Status              string `json:"status"`
	ConsumeType         string `json:"consumeType"`
	TokensMagnitude     int64  `json:"tokensMagnitude"`  // original size
	AvailableBalance    int64  `json:"availableBalance"` // tokens left
	ExpirationTime      string `json:"expirationTime"`
}

// accountReportResponse mirrors GET /biz/account/query-customer-account-report
// — the cash (pay-as-you-go) balance. Amounts are plain decimals.
type accountReportResponse struct {
	Code    int         `json:"code"`
	Success bool        `json:"success"`
	Msg     string      `json:"msg"`
	Data    accountData `json:"data"`
}

type accountData struct {
	Balance          float64 `json:"balance"`
	RechargeAmount   float64 `json:"rechargeAmount"`
	GiveAmount       float64 `json:"giveAmount"`
	TotalSpendAmount float64 `json:"totalSpendAmount"`
	FrozenBalance    float64 `json:"frozenBalance"`
}

// quotaLimitResponse mirrors GET /monitor/usage/quota/limit — the GLM Coding
// Plan usage windows (the 5-hour prompt window, the weekly window when the
// plan has one, and MCP/tool quotas when present).
type quotaLimitResponse struct {
	Code    int       `json:"code"`
	Success bool      `json:"success"`
	Msg     string    `json:"msg"`
	Data    quotaData `json:"data"`
}

type quotaData struct {
	Limits []quotaLimit `json:"limits"`
	Level  string       `json:"level"` // Coding Plan tier, e.g. "pro"
}

// quotaLimit is one Coding Plan window. usage is the window's total credit
// allowance, currentValue the credits already spent, and nextResetTime the
// reset deadline in epoch milliseconds.
type quotaLimit struct {
	Type          string `json:"type"`
	Unit          int    `json:"unit"`
	Number        int    `json:"number"`
	Usage         int64  `json:"usage"`
	CurrentValue  int64  `json:"currentValue"`
	Remaining     int64  `json:"remaining"`
	Percentage    int    `json:"percentage"` // truncated integer — recomputed
	NextResetTime int64  `json:"nextResetTime"`
}

// Fetch reports the GLM Coding Plan usage windows plus the Z.ai usage
// bundles and cash balance for the configured API key. The Coding Plan
// windows (5h prompt, weekly, MCP/tool) come first: they are the capacity
// signal for subscription-style use. Bundles are the prepaid token packages
// (e.g. a "20 million GLM-5.3 trial pack"); each is model-specific, so a
// bundle can sit untouched while pay-as-you-go cash is spent on another
// model. An account without a Coding Plan reports bundles/cash plus a clear
// "Coding plan: none" fact instead of failing.
func Fetch(ctx context.Context) (*usage.Report, error) {
	key, err := loadKey()
	if err != nil {
		return nil, err
	}
	jwt, err := mintJWT(key)
	if err != nil {
		return nil, err
	}

	quota, quotaErr := getQuota(ctx, key) // non-fatal: bundles alone are useful
	bundles, err := getBundles(ctx, jwt)
	if err != nil {
		return nil, err
	}
	cash, cashErr := getCash(ctx, jwt) // non-fatal
	if cashErr != nil {
		cash = nil
	}

	report := &usage.Report{Provider: "Z.ai"}
	appendQuota(report, quota)
	bundleReport := reportFromBundles(bundles)
	report.Windows = append(report.Windows, bundleReport.Windows...)
	report.Extra = append(report.Extra, bundleReport.Extra...)
	appendCash(report, cash)
	if cashErr != nil {
		report.Extra = append(report.Extra, usage.Fact{Label: "Cash", Value: "unavailable"})
	}
	if quotaErr != nil {
		report.Extra = append(report.Extra, usage.Fact{Label: "Coding plan", Value: "unavailable — " + firstLine(quotaErr.Error())})
	}
	return report, nil
}

// get performs an authenticated GET and decodes the JSON body into out.
func get(ctx context.Context, jwt, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", jwt)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "aiquokka")

	resp, err := httpx.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Z.ai credentials were rejected (%s) — check ZAI_API_KEY", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s%s: %s: %s", baseURL(), path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}

// getBearer is get with a "Bearer <raw API key>" Authorization header — the
// scheme the monitor endpoints require (the console /biz endpoints take the
// signed JWT instead).
func getBearer(ctx context.Context, apiKey, path string, out any) error {
	return get(ctx, "Bearer "+apiKey, path, out)
}

func getBundles(ctx context.Context, jwt string) ([]bundle, error) {
	var out tokenAccountsResponse
	if err := get(ctx, jwt, "/biz/tokenAccounts/list/my", &out); err != nil {
		return nil, err
	}
	return out.Rows, nil
}

// getQuota fetches the GLM Coding Plan usage windows with the raw API key.
// An account without a Coding Plan succeeds with an empty limits list.
func getQuota(ctx context.Context, apiKey string) (*quotaData, error) {
	var out quotaLimitResponse
	if err := getBearer(ctx, apiKey, "/monitor/usage/quota/limit", &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, fmt.Errorf("Z.ai quota: %s", out.Msg)
	}
	return &out.Data, nil
}

func getCash(ctx context.Context, jwt string) (*accountData, error) {
	var out accountReportResponse
	if err := get(ctx, jwt, "/biz/account/query-customer-account-report", &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, fmt.Errorf("Z.ai account report: %s", out.Msg)
	}
	return &out.Data, nil
}

// appendQuota adds the GLM Coding Plan windows to the report and records the
// plan tier. Accounts without a Coding Plan (empty limits) get a clear fact
// instead of silently showing nothing.
func appendQuota(report *usage.Report, quota *quotaData) {
	if quota == nil {
		return
	}
	for _, lim := range quota.Limits {
		if w := quotaWindow(lim); w != nil {
			report.Windows = append(report.Windows, *w)
		}
	}
	switch {
	case len(quota.Limits) > 0 && quota.Level != "":
		report.Plan = quota.Level
	case len(quota.Limits) == 0:
		report.Extra = append(report.Extra, usage.Fact{
			Label: "Coding plan",
			Value: "none — this account has no GLM Coding Plan",
		})
	}
}

// quotaWindow converts one Coding Plan limit into a shared usage window,
// matching the shape of the Claude/Codex windows (used_percent plus
// resets_at). Credits are the unit: usage is the allowance, currentValue the
// spend. It returns nil for entries with no measurable allowance.
func quotaWindow(lim quotaLimit) *usage.Window {
	var pct *float64
	if lim.Usage > 0 && lim.CurrentValue >= 0 {
		// Recompute from counts: the API's percentage field is truncated.
		p := float64(lim.CurrentValue) / float64(lim.Usage) * 100
		pct = &p
	} else if lim.Percentage > 0 {
		p := float64(lim.Percentage)
		pct = &p
	} else {
		return nil
	}
	label, duration := quotaWindowName(lim)
	w := &usage.Window{Label: label, UsedPercent: pct, Duration: duration}
	if lim.Usage > 0 {
		used, total := lim.CurrentValue, lim.Usage
		w.Used, w.Limit = &used, &total
	}
	if lim.NextResetTime > 0 {
		w.ResetsAt = time.UnixMilli(lim.NextResetTime)
	}
	return w
}

// quotaWindowName names a quota window from its unit and count: unit 3 with
// number 5 is the 5-hour prompt window ("5h", like Claude), unit 6 the weekly
// window ("Weekly"). Non-credit limits (e.g. an MCP/tool quota) are prefixed
// with their type so they stay distinguishable.
func quotaWindowName(lim quotaLimit) (string, time.Duration) {
	var label string
	var duration time.Duration
	switch lim.Unit {
	case 3: // hours
		label = fmt.Sprintf("%dh", lim.Number)
		duration = time.Duration(lim.Number) * time.Hour
	case 6: // weeks
		if lim.Number == 1 {
			label = "Weekly"
		} else {
			label = fmt.Sprintf("%dw", lim.Number)
		}
		duration = time.Duration(lim.Number) * 7 * 24 * time.Hour
	default:
		label = "Quota"
	}
	if lim.Type != "" && !strings.EqualFold(lim.Type, "CREDIT_LIMIT") {
		label = shortLimitType(lim.Type) + " " + label
	}
	return label, duration
}

// shortLimitType trims a "_LIMIT" suffix: "MCP_LIMIT" -> "MCP".
func shortLimitType(t string) string {
	if v := strings.TrimSuffix(t, "_LIMIT"); v != t {
		return v
	}
	return t
}

// firstLine trims an error message to its first line so facts stay compact.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// reportFromBundles converts effective prepaid bundles into usage windows.
// Each window shows the consumed fraction of the bundle (used/total tokens);
// expiry and applicability are surfaced as extra facts since bundles do not
// "reset" — they simply expire.
func reportFromBundles(bundles []bundle) *usage.Report {
	report := &usage.Report{Provider: "Z.ai"}
	for _, b := range bundles {
		if !strings.EqualFold(b.Status, "EFFECTIVE") {
			continue
		}
		if b.TokensMagnitude <= 0 {
			continue
		}
		used := b.TokensMagnitude - b.AvailableBalance
		report.Windows = append(report.Windows, usage.Window{
			Label: bundleLabel(b, bundles),
			Used:  &used,
			Limit: &b.TokensMagnitude,
		})
		report.Extra = append(report.Extra,
			usage.Fact{Label: bundleLabel(b, bundles), Value: b.ResourcePackageName},
		)
		if b.SuitableModel != "" {
			report.Extra = append(report.Extra,
				usage.Fact{Label: "Applies to", Value: b.SuitableModel},
			)
		}
		if exp := shortDate(b.ExpirationTime); exp != "" {
			report.Extra = append(report.Extra, usage.Fact{Label: "Expires", Value: exp})
		}
	}
	return report
}

// bundleLabel names a bundle window, adding the model when several bundles
// (or the bundle itself) make the target model relevant.
func bundleLabel(b bundle, bundles []bundle) string {
	if len(bundles) > 1 && b.SuitableModel != "" {
		return "Bundle (" + b.SuitableModel + ")"
	}
	return "Bundle"
}

// appendCash adds the pay-as-you-go balance as a remaining-balance window
// (full bar that runs down to zero) with top-up facts, mirroring DeepSeek.
func appendCash(report *usage.Report, cash *accountData) {
	if cash == nil {
		return
	}
	balance := cash.Balance
	report.Windows = append(report.Windows, usage.Window{
		Label:     "Cash",
		Remaining: &balance,
		Currency:  currency(),
	})
	if cash.RechargeAmount > 0 {
		report.Extra = append(report.Extra, usage.Fact{
			Label: "Topped up",
			Value: usage.FormatMoney(cash.RechargeAmount, currency()),
		})
	}
	if cash.GiveAmount > 0 {
		report.Extra = append(report.Extra, usage.Fact{
			Label: "Granted",
			Value: usage.FormatMoney(cash.GiveAmount, currency()),
		})
	}
	if cash.TotalSpendAmount > 0 {
		report.Extra = append(report.Extra, usage.Fact{
			Label: "Spent",
			Value: usage.FormatMoney(cash.TotalSpendAmount, currency()),
		})
	}
}

// currency guesses the cash-balance currency from the API host: the
// international platform bills in USD, the China platform in CNY.
func currency() string {
	if strings.Contains(baseURL(), "bigmodel.cn") {
		return "CNY"
	}
	return "USD"
}

// shortDate trims a wire timestamp like "2026-11-28T15:34:31" to its date.
func shortDate(s string) string {
	if len(s) < 10 {
		return s
	}
	return s[:10]
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func hmacSign(secret []byte, data string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	return b64url(mac.Sum(nil))
}
