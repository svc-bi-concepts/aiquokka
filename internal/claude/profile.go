package claude

import (
	"context"
	"net/http"
	"strings"

	"github.com/McKean/aiquokka/internal/httpx"
)

// profileEndpoint is the live OAuth profile. The subscription and tier in the
// stored credentials are written at login and never refreshed, so an upgrade
// after login only shows here. A var so tests can point it at a fake server.
var profileEndpoint = "https://api.anthropic.com/api/oauth/profile"

// profileResponse holds the only profile fields we read; the account's name
// and email are deliberately left out.
type profileResponse struct {
	Organization struct {
		OrganizationType string `json:"organization_type"`
		RateLimitTier    string `json:"rate_limit_tier"`
	} `json:"organization"`
}

// livePlan returns the plan from the profile endpoint. Each field the profile
// names replaces the cached one; a field it leaves empty keeps the cached
// value, and a failed call keeps both. The error is dropped on purpose: the
// cached plan is a good enough label and usage still renders.
func livePlan(ctx context.Context, o *oauth) string {
	var resp profileResponse
	err := httpx.GetJSON(ctx, profileEndpoint, &resp, func(r *http.Request) {
		setAuthHeaders(r, o)
	})
	if err != nil {
		return plan(o)
	}
	merged := oauth{SubscriptionType: o.SubscriptionType, RateLimitTier: o.RateLimitTier}
	// organization_type is "claude_max"; the credentials say "max".
	if t := strings.TrimPrefix(resp.Organization.OrganizationType, "claude_"); t != "" {
		merged.SubscriptionType = t
	}
	if tier := resp.Organization.RateLimitTier; tier != "" {
		merged.RateLimitTier = tier
	}
	return plan(&merged)
}
