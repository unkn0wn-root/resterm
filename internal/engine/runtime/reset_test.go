package runtime

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/oauth"
)

// sourcedScope has the shape vars gives a scope read from an environment
// file: the source prefix, the name and a digest of the file path.
const sourcedScope = "f2:dev@0123456789abcdef"

// A scope naming its environment file cannot resolve in another workspace,
// so crossing a workspace boundary keeps it dormant and forgets the rest.
func TestResetSharedSecretsKeepsSourcedScopes(t *testing.T) {
	rt := New(Config{})

	rt.Globals().Set(sourcedScope, "auth.token", "KEEP", true)
	rt.Globals().Set("dev", "auth.token", "SHARED", true)
	rt.Globals().Set("", "auth.token", "SHARED", true)
	rt.Files().Set(sourcedScope, "/ws/a.http", "file.token", "KEEP", true)
	rt.Files().Set("dev", "/ws/a.http", "file.token", "SHARED", true)
	piped := "f2:qa|west@0123456789abcdef"
	rt.Files().Set(piped, "/ws/a.http", "file.token", "KEEP", true)

	u, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	rt.Cookies().Jar(sourcedScope).SetCookies(u, []*http.Cookie{{Name: "s", Value: "KEEP"}})
	rt.Cookies().Jar("dev").SetCookies(u, []*http.Cookie{{Name: "s", Value: "SHARED"}})

	rt.ResetSharedSecrets()

	if got := rt.Globals().Snapshot(sourcedScope); got["auth.token"].Value != "KEEP" {
		t.Fatalf("sourced globals did not stay dormant: %v", got)
	}
	for _, scope := range []string{"dev", ""} {
		if got := rt.Globals().Snapshot(scope); len(got) != 0 {
			t.Fatalf("globals for %q survived: %v", scope, got)
		}
	}
	if got := rt.Files().Snapshot(sourcedScope, "/ws/a.http"); got["file.token"].Value != "KEEP" {
		t.Fatalf("sourced file values did not stay dormant: %v", got)
	}
	if got := rt.Files().Snapshot("dev", "/ws/a.http"); len(got) != 0 {
		t.Fatalf("file values survived: %v", got)
	}
	if got := rt.Files().Snapshot(piped, "/ws/a.http"); got["file.token"].Value != "KEEP" {
		t.Fatalf("a separator in the environment name dropped sourced file values: %v", got)
	}
	if got := rt.Cookies().Jar(sourcedScope).Cookies(u); len(got) != 1 {
		t.Fatalf("sourced cookies did not stay dormant: %v", got)
	}
	if got := rt.Cookies().Jar("dev").Cookies(u); len(got) != 0 {
		t.Fatalf("cookies survived: %v", got)
	}
}

func TestResetSharedSecretsKeepsSourcedAuth(t *testing.T) {
	rt := New(Config{})
	rt.LoadAuthState(engine.AuthState{
		OAuth: []oauth.SnapshotEntry{
			{
				Key:    "16:" + sourcedScope + "6:github",
				Env:    sourcedScope,
				Config: oauth.Config{TokenURL: "https://auth.local/token", CacheKey: "github"},
				Token:  oauth.Token{AccessToken: "KEEP"},
			},
			{
				Key:    "3:dev6:github",
				Env:    "dev",
				Config: oauth.Config{TokenURL: "https://auth.local/token", CacheKey: "github"},
				Token:  oauth.Token{AccessToken: "SHARED"},
			},
		},
		Command: []authcmd.SnapshotEntry{{
			Key:    "3:dev|6:/ws/a|6:github",
			Config: authcmd.Config{CacheKey: "github"},
			Token:  "cmd-token",
		}},
	})

	rt.ResetSharedSecrets()

	st := rt.AuthState()
	if len(st.OAuth) != 1 || st.OAuth[0].Env != sourcedScope {
		t.Fatalf("oauth entries after reset: %+v", st.OAuth)
	}
	if len(st.Command) != 1 {
		t.Fatalf("workspace keyed command auth must survive: %+v", st.Command)
	}
}
