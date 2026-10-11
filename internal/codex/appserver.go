package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// appServerTimeout bounds one round trip to `codex app-server`, the whole
// session including the process exit. A variable so a test can shorten it.
var appServerTimeout = 30 * time.Second

// rateLimitsID is the request id of account/rateLimits/read; initialize is 1.
const rateLimitsID = 2

// A hardened codex (Automic Vault's `av harden codex`, or any codex set to
// cli_auth_credentials_store = "keyring") keeps its login out of auth.json.
// codex app-server answers the usage read with that login, so aiquokka asks
// codex instead of reading the Keychain itself: the token never leaves codex.
// Checked against codex-cli 0.162.1.

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// rateLimitsResult mirrors the fields of account/rateLimits/read that are used.
type rateLimitsResult struct {
	RateLimits struct {
		Primary   *appWindow  `json:"primary"`
		Secondary *appWindow  `json:"secondary"`
		Credits   *appCredits `json:"credits"`
		PlanType  string      `json:"planType"`
	} `json:"rateLimits"`
	RateLimitResetCredits *struct {
		AvailableCount *int64 `json:"availableCount"`
	} `json:"rateLimitResetCredits"`
}

type appWindow struct {
	UsedPercent       *int32 `json:"usedPercent"`
	WindowDurationMin *int64 `json:"windowDurationMins"`
	ResetsAt          *int64 `json:"resetsAt"`
}

type appCredits struct {
	HasCredits bool    `json:"hasCredits"`
	Unlimited  bool    `json:"unlimited"`
	Balance    *string `json:"balance"`
}

// appServerUsage asks `codex app-server` for the account's usage windows and
// returns them in the same shape /wham/usage gives, so the report is built once.
func appServerUsage(ctx context.Context) (*usageResponse, error) {
	bin, err := exec.LookPath("codex")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, appServerTimeout)
	defer cancel()

	cmd := exec.Command(bin, "app-server", "--listen", "stdio://")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Kill codex if the timeout passes while it is still silent; otherwise
	// closing stdin ends it, and Wait reaps it before the function returns.
	stop := context.AfterFunc(ctx, func() { _ = cmd.Process.Kill() })
	defer func() {
		stdin.Close()
		_ = cmd.Wait()
		stop()
	}()

	enc := json.NewEncoder(stdin)
	initID, rlID := 1, rateLimitsID
	requests := []rpcRequest{
		{JSONRPC: "2.0", ID: &initID, Method: "initialize", Params: map[string]any{
			"clientInfo": map[string]string{"name": "aiquokka", "version": "dev"},
		}},
		{JSONRPC: "2.0", Method: "initialized"},
		{JSONRPC: "2.0", ID: &rlID, Method: "account/rateLimits/read", Params: map[string]any{
			"excludeResetCreditDetails": true,
		}},
	}
	for _, r := range requests {
		if err := enc.Encode(r); err != nil {
			return nil, fmt.Errorf("codex app-server: %w", err)
		}
	}

	raw, err := readResult(stdout, rateLimitsID)
	if err != nil {
		return nil, fmt.Errorf("codex app-server: %w", err)
	}
	var rl rateLimitsResult
	if err := json.Unmarshal(raw, &rl); err != nil {
		return nil, fmt.Errorf("decoding codex rate limits: %w", err)
	}
	return rl.usage(), nil
}

// readResult scans codex's stdout for the response to id and returns its result.
// Notifications and other responses are skipped.
func readResult(r io.Reader, id int) (json.RawMessage, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		var msg rpcResponse
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil || msg.ID == nil || *msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("%s", msg.Error.Message)
		}
		return msg.Result, nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF
}

// usage converts the app-server result to the /wham/usage shape.
func (rl *rateLimitsResult) usage() *usageResponse {
	out := &usageResponse{PlanType: rl.RateLimits.PlanType}
	out.RateLimit.Primary = windowFrom(rl.RateLimits.Primary)
	out.RateLimit.Secondary = windowFrom(rl.RateLimits.Secondary)
	if c := rl.RateLimits.Credits; c != nil {
		out.Credits = &credits{HasCredits: c.HasCredits, Unlimited: c.Unlimited}
		if c.Balance != nil {
			out.Credits.Balance = *c.Balance
		}
	}
	if rc := rl.RateLimitResetCredits; rc != nil {
		out.RateLimitResetCredits = &struct {
			AvailableCount           *int64 `json:"available_count"`
			ApplicableAvailableCount *int64 `json:"applicable_available_count"`
		}{AvailableCount: rc.AvailableCount}
	}
	return out
}

// windowFrom converts an app-server window; its duration is in minutes and
// its reset is a Unix time, where /wham/usage uses seconds for both.
func windowFrom(w *appWindow) *window {
	if w == nil || w.UsedPercent == nil {
		return nil
	}
	used := float64(*w.UsedPercent)
	out := &window{UsedPercent: &used, ResetAt: w.ResetsAt}
	if w.WindowDurationMin != nil {
		out.LimitWindowSeconds = *w.WindowDurationMin * 60
	}
	return out
}
