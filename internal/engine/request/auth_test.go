package request

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/engine"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
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

func TestEnsureCommandAuthSetsAuthorizationHeader(t *testing.T) {
	var calls int32
	var seen authcmd.Config

	eng := newTestEngine()
	eng.rt.AuthCmd().SetExecFunc(func(_ context.Context, cfg authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		seen = cfg
		return []byte("token-basic"), nil
	})

	auth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv":      `["gh","auth","token"]`,
		"cache_key": "github",
	}}
	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}

	res, err := eng.EnsureCommandAuth(
		context.Background(),
		&restfile.Document{Path: "/tmp/example.http"},
		req,
		vars.NewResolver(),
		"",
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("ensureCommandAuth: %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer token-basic" {
		t.Fatalf("expected bearer header, got %q", got)
	}
	if res.Token != "token-basic" {
		t.Fatalf("expected token result, got %q", res.Token)
	}
	if seen.Dir != "/tmp" {
		t.Fatalf("expected command dir /tmp, got %q", seen.Dir)
	}
	if seen.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %s", seen.Timeout)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected one command auth execution, got %d", calls)
	}
}

func TestEnsureCommandAuthSkipsWhenHeaderPresent(t *testing.T) {
	called := int32(0)

	eng := newTestEngine()
	eng.rt.AuthCmd().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&called, 1)
		return []byte("token-basic"), nil
	})

	req := &restfile.Request{
		Headers: http.Header{"Authorization": {"Bearer manual"}},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{Type: "command", Params: map[string]string{
				"argv": `["gh","auth","token"]`,
			}},
		},
	}

	if _, err := eng.EnsureCommandAuth(
		context.Background(),
		&restfile.Document{Path: "/tmp/example.http"},
		req,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth with existing header: %v", err)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatalf("expected no command auth execution, got %d", called)
	}
	if req.Headers.Get("Authorization") != "Bearer manual" {
		t.Fatalf("expected header to remain unchanged")
	}
}

func TestEnsureCommandAuthCacheOnlyReuseInheritsSeededConfig(t *testing.T) {
	var calls int32

	eng := newTestEngine()
	eng.rt.AuthCmd().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("token-basic"), nil
	})

	seedAuth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv":      `["gh","auth","token"]`,
		"cache_key": "github",
		"header":    "X-Registry-Token",
		"scheme":    "Token",
	}}
	cacheOnlyAuth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"cache_key": "github",
	}}

	seedReq := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: seedAuth}}
	if _, err := eng.EnsureCommandAuth(
		context.Background(),
		&restfile.Document{Path: "/tmp/example.http"},
		seedReq,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth seed: %v", err)
	}

	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: cacheOnlyAuth}}
	res, err := eng.EnsureCommandAuth(
		context.Background(),
		&restfile.Document{Path: "/tmp/example.http"},
		req,
		vars.NewResolver(),
		"",
		time.Second,
	)
	if err != nil {
		t.Fatalf("ensureCommandAuth cache-only: %v", err)
	}
	if got := req.Headers.Get("X-Registry-Token"); got != "Token token-basic" {
		t.Fatalf("expected inherited seeded header, got %q", got)
	}
	if res.Header != "X-Registry-Token" || res.Value != "Token token-basic" {
		t.Fatalf("unexpected command auth result %#v", res)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected cache-only reuse to skip execution, got %d calls", calls)
	}
}

func TestEnsureCommandAuthGlobalCrossFile(t *testing.T) {
	var calls int32
	var seen authcmd.Config

	workspace := t.TempDir()
	defsPath := filepath.Join(workspace, "auth_command.http")
	usePath := filepath.Join(workspace, "rts", "rts_all_features.http")

	eng := New(
		engine.Config{
			FilePath:        defsPath,
			EnvironmentName: "dev",
			WorkspaceRoot:   workspace,
		},
		rtrun.New(rtrun.Config{}),
	)
	eng.rt.AuthCmd().SetExecFunc(func(_ context.Context, cfg authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		seen = cfg
		return []byte("token-basic"), nil
	})

	defsDoc := parser.Parse(
		defsPath,
		[]byte(`# @auth global command argv=["gh","auth","token"] cache_key=github-cli-global`),
	)
	eng.registryIndex().Sync(defsDoc)

	seedReq := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Auth: restfile.CloneAuthSpec(&defsDoc.Auth[0].Spec),
		},
	}
	if _, err := eng.EnsureCommandAuth(
		context.Background(),
		defsDoc,
		seedReq,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth seed: %v", err)
	}

	useDoc := parser.Parse(usePath, []byte("GET https://example.com\n"))
	req := &restfile.Request{}
	eng.ResolveInheritedAuth(useDoc, req)
	if req.Metadata.Auth == nil {
		t.Fatal("expected inherited global auth")
	}
	if got := req.Metadata.Auth.SourcePath; got != defsPath {
		t.Fatalf("expected inherited auth source %q, got %q", defsPath, got)
	}

	if _, err := eng.EnsureCommandAuth(
		context.Background(),
		useDoc,
		req,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth inherited reuse: %v", err)
	}

	if got := seen.Dir; got != filepath.Dir(defsPath) {
		t.Fatalf("expected command dir %q, got %q", filepath.Dir(defsPath), got)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected inherited request to reuse seeded cache, got %d calls", calls)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer token-basic" {
		t.Fatalf("expected inherited auth header, got %q", got)
	}
}

func TestEnsureCommandAuthWorkspaceScope(t *testing.T) {
	var calls int32

	rt := rtrun.New(rtrun.Config{})
	rt.AuthCmd().SetExecFunc(func(_ context.Context, cfg authcmd.Config) ([]byte, error) {
		n := atomic.AddInt32(&calls, 1)
		return []byte(fmt.Sprintf("%s/token-%d", cfg.Dir, n)), nil
	})

	authA := &restfile.AuthSpec{
		Type: "command",
		Params: map[string]string{
			"argv":      `["gh","auth","token"]`,
			"cache_key": "shared",
		},
		SourcePath: "/tmp/workspace-a/auth.http",
	}
	authB := &restfile.AuthSpec{
		Type: "command",
		Params: map[string]string{
			"argv":      `["gh","auth","token"]`,
			"cache_key": "shared",
		},
		SourcePath: "/tmp/workspace-b/auth.http",
	}

	engA := New(engine.Config{
		FilePath:        "/tmp/workspace-a/request.http",
		EnvironmentName: "dev",
		WorkspaceRoot:   "/tmp/workspace-a",
	}, rt)
	engB := New(engine.Config{
		FilePath:        "/tmp/workspace-b/request.http",
		EnvironmentName: "dev",
		WorkspaceRoot:   "/tmp/workspace-b",
	}, rt)

	reqA := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: authA}}
	reqB := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: authB}}

	if _, err := engA.EnsureCommandAuth(
		context.Background(),
		&restfile.Document{Path: "/tmp/workspace-a/request.http"},
		reqA,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth workspace A: %v", err)
	}
	if _, err := engB.EnsureCommandAuth(
		context.Background(),
		&restfile.Document{Path: "/tmp/workspace-b/request.http"},
		reqB,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth workspace B: %v", err)
	}

	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected separate command executions per workspace, got %d", calls)
	}
}

func TestBuildCommandAuthConfigExpandsArgvAfterJSONDecode(t *testing.T) {
	eng := newTestEngine()

	auth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv": `["aws","--profile","{{aws.profile}}","ecr","get-login-password"]`,
	}}
	resolver := vars.NewResolver(vars.NewMapProvider("aws", map[string]string{
		"profile": `qa"blue\team`,
	}))

	cfg, err := eng.BuildCommandAuthConfig(
		&restfile.Document{Path: "/tmp/example.http"},
		auth,
		resolver,
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("buildCommandAuthConfig: %v", err)
	}

	if got := cfg.Argv[2]; got != `qa"blue\team` {
		t.Fatalf("expected expanded argv value with quotes and slashes preserved, got %q", got)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %s", cfg.Timeout)
	}
}

func TestBuildOAuthConfigExpandsKnownParamsAndExtra(t *testing.T) {
	eng := newTestEngine()

	auth := &restfile.AuthSpec{Type: "oauth2", Params: map[string]string{
		"token_url": " https://auth.local/{{oauth.host}}/token ",
		"client_id": "{{oauth.client_id}}",
		"grant":     " client_credentials ",
		"header":    " X-Registry-Token ",
		"prompt":    " {{oauth.prompt}} ",
		"max-age":   " {{oauth.max_age}} ",
	}}
	resolver := vars.NewResolver(vars.NewMapProvider("oauth", map[string]string{
		"host":      "tenant-a",
		"client_id": "client-1",
		"prompt":    "consent",
		"max_age":   "300",
	}))

	cfg, err := eng.BuildOAuthConfig(auth, resolver)
	if err != nil {
		t.Fatalf("buildOAuthConfig: %v", err)
	}

	if cfg.TokenURL != "https://auth.local/tenant-a/token" {
		t.Fatalf("expected expanded token_url, got %q", cfg.TokenURL)
	}
	if cfg.ClientID != "client-1" {
		t.Fatalf("expected expanded client_id, got %q", cfg.ClientID)
	}
	if cfg.GrantType != oauth.GrantClientCredentials {
		t.Fatalf("expected normalized grant type, got %q", cfg.GrantType)
	}
	if cfg.Header != "X-Registry-Token" {
		t.Fatalf("expected trimmed header, got %q", cfg.Header)
	}
	if got := cfg.Extra["prompt"]; got != "consent" {
		t.Fatalf("expected extra prompt, got %q", got)
	}
	if got := cfg.Extra["max_age"]; got != "300" {
		t.Fatalf("expected normalized extra key max_age, got %q", got)
	}
}

func TestEnsureCommandAuthWithoutRuntimeReturnsInitError(t *testing.T) {
	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
		Type: "command",
		Params: map[string]string{
			"argv": `["gh","auth","token"]`,
		},
	}}}

	_, err := (&Engine{}).EnsureCommandAuth(
		context.Background(),
		&restfile.Document{Path: "/tmp/example.http"},
		req,
		vars.NewResolver(),
		"",
		time.Second,
	)
	if err == nil {
		t.Fatal("expected init error")
	}
	if !strings.Contains(err.Error(), errCommandAuthNotInitialized) {
		t.Fatalf("expected init error, got %v", err)
	}
}

func newTestEngine() *Engine {
	return New(
		engine.Config{
			FilePath:        "/tmp/example.http",
			EnvironmentName: "dev",
		},
		rtrun.New(rtrun.Config{}),
	)
}
