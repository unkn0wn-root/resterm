package request

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/oauth"
)

func TestOAuthNeedsHeadlessSeed(t *testing.T) {
	cfg := oauth.Config{
		GrantType: "authorization_code",
		CacheKey:  "oauth-public",
	}
	oa := oauth.NewManager(nil)

	headless := &Engine{}
	if !headless.oauthNeedsHeadlessSeed(oa, "dev", cfg) {
		t.Fatalf("expected unseeded auth code flow to require headless seed")
	}

	interactive := &Engine{cfg: engine.Config{AllowInteractiveOAuth: true}}
	if interactive.oauthNeedsHeadlessSeed(oa, "dev", cfg) {
		t.Fatalf("expected interactive engine to allow auth code flow")
	}

	oa.Restore([]oauth.SnapshotEntry{{
		Key:    "oauth-public",
		Config: cfg,
		Token: oauth.Token{
			AccessToken:  "expired-token",
			RefreshToken: "refresh-1",
			Expiry:       time.Now().Add(-time.Hour),
		},
	}})
	if headless.oauthNeedsHeadlessSeed(oa, "dev", cfg) {
		t.Fatalf("expected cached refresh token to satisfy headless auth code flow")
	}
}
