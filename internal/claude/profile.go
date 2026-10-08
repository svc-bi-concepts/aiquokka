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

// livePlan returns the plan from the profile endpoint, or the plan cached in
// the credentials when the call fails or names no plan. The error is dropped
// on purpose: the cached plan is a good enough label and usage still renders.
func livePlan(ctx context.Context, o *oauth) string {
	var resp profileResponse
	err := httpx.GetJSON(ctx, profileEndpoint, &resp, func(r *http.Request) {
		setAuthHeaders(r, o)
	})
	if err == nil {
		// organization_type is "claude_max"; the credentials say "max".
		live := oauth{
			SubscriptionType: strings.TrimPrefix(resp.Organization.OrganizationType, "claude_"),
			RateLimitTier:    resp.Organization.RateLimitTier,
		}
		if p := plan(&live); p != "" {
			return p
		}
	}
	return plan(o)
}
