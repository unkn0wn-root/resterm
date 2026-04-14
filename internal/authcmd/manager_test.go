package authcmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagerResolveCachesByEnvironment(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	mgr := NewManager()
	mgr.now = func() time.Time { return time.Unix(100, 0) }
	mgr.SetExecFunc(func(context.Context, Config) ([]byte, error) {
		n := calls.Add(1)
		return []byte(fmt.Sprintf("token-%d", n)), nil
	})

	cfg := Config{Argv: []string{"gh"}, CacheKey: "github"}

	a, err := mgr.Resolve(context.Background(), "dev", cfg)
	if err != nil {
		t.Fatalf("Resolve(dev) error = %v", err)
	}
	b, err := mgr.Resolve(context.Background(), "dev", cfg)
	if err != nil {
		t.Fatalf("Resolve(dev) second error = %v", err)
	}
	c, err := mgr.Resolve(context.Background(), "prod", cfg)
	if err != nil {
		t.Fatalf("Resolve(prod) error = %v", err)
	}

	if a.Token != b.Token {
		t.Fatalf("expected dev cache hit, got %q and %q", a.Token, b.Token)
	}
	if c.Token == a.Token {
		t.Fatalf("expected prod cache separation, got shared token %q", c.Token)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 runs, got %d", got)
	}
}

func TestManagerResolveTTLRefreshesExpiredEntry(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	now := time.Unix(100, 0)

	mgr := NewManager()
	mgr.now = func() time.Time { return now }
	mgr.SetExecFunc(func(context.Context, Config) ([]byte, error) {
		n := calls.Add(1)
		return []byte(fmt.Sprintf("token-%d", n)), nil
	})

	cfg := Config{
		Argv:     []string{"gh"},
		CacheKey: "github",
		TTL:      time.Minute,
	}

	first, err := mgr.Resolve(context.Background(), "dev", cfg)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	now = now.Add(30 * time.Second)
	second, err := mgr.Resolve(context.Background(), "dev", Config{CacheKey: "github"})
	if err != nil {
		t.Fatalf("Resolve() cached error = %v", err)
	}
	now = now.Add(40 * time.Second)
	third, err := mgr.Resolve(context.Background(), "dev", Config{CacheKey: "github"})
	if err != nil {
		t.Fatalf("Resolve() refreshed error = %v", err)
	}

	if first.Token != second.Token {
		t.Fatalf("expected cached token, got %q then %q", first.Token, second.Token)
	}
	if third.Token == second.Token {
		t.Fatalf("expected refreshed token after ttl, got %q", third.Token)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 runs, got %d", got)
	}
}

func TestManagerResolveExpiresInUsesFetchCompletionTime(t *testing.T) {
	t.Parallel()

	times := []time.Time{
		time.Unix(100, 0),
		time.Unix(140, 0),
	}
	var nowCalls int

	mgr := NewManager()
	mgr.now = func() time.Time {
		if nowCalls >= len(times) {
			return times[len(times)-1]
		}
		ts := times[nowCalls]
		nowCalls++
		return ts
	}
	mgr.SetExecFunc(func(context.Context, Config) ([]byte, error) {
		return []byte(`{"access_token":"abc","expires_in":"60"}`), nil
	})

	res, err := mgr.Resolve(context.Background(), "dev", Config{
		Argv:          []string{"tool"},
		Format:        FormatJSON,
		TokenPath:     "access_token",
		ExpiresInPath: "expires_in",
		CacheKey:      "shared",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := times[1].Add(time.Minute)
	if !res.Expiry.Equal(want) {
		t.Fatalf("expected expiry %s, got %s", want, res.Expiry)
	}
}

func TestManagerCachedUsesPreviewCacheOnly(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.now = func() time.Time { return time.Unix(100, 0) }
	cfg := Config{Argv: []string{"gh"}, CacheKey: "github"}
	mgr.store(cacheEntryKey("dev", cfg.normalize()), cfg.normalize(), credential{
		Token:     "abc",
		FetchedAt: time.Unix(90, 0),
	})

	res, ok, err := mgr.Cached(" dev ", Config{CacheKey: " github "})
	if err != nil {
		t.Fatalf("Cached() error = %v", err)
	}
	if !ok {
		t.Fatal("expected cached result")
	}
	if res.Token != "abc" {
		t.Fatalf("expected cached token, got %q", res.Token)
	}
}

func TestManagerResolveNormalizesConfig(t *testing.T) {
	t.Parallel()

	var seen Config

	mgr := NewManager()
	mgr.now = func() time.Time { return time.Unix(100, 0) }
	mgr.SetExecFunc(func(_ context.Context, cfg Config) ([]byte, error) {
		seen = cfg
		return []byte(`{"access_token":"abc"}`), nil
	})

	cfg := Config{
		Argv:      []string{" gh ", "auth", "token"},
		Dir:       " /tmp/project ",
		Format:    Format(" JSON "),
		Header:    " Authorization ",
		TokenPath: " access_token ",
		CacheKey:  " github ",
	}

	res, err := mgr.Resolve(context.Background(), " dev ", cfg)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.Value != "Bearer abc" {
		t.Fatalf("expected bearer header value, got %q", res.Value)
	}
	if seen.Argv[0] != "gh" {
		t.Fatalf("expected normalized argv[0], got %q", seen.Argv[0])
	}
	if seen.Dir != "/tmp/project" {
		t.Fatalf("expected normalized dir, got %q", seen.Dir)
	}
	if seen.Format != FormatJSON {
		t.Fatalf("expected normalized format, got %q", seen.Format)
	}
	if seen.Header != "Authorization" {
		t.Fatalf("expected normalized header, got %q", seen.Header)
	}
	if seen.TokenPath != "access_token" {
		t.Fatalf("expected normalized token path, got %q", seen.TokenPath)
	}
	if seen.CacheKey != "github" {
		t.Fatalf("expected normalized cache key, got %q", seen.CacheKey)
	}
}

func TestManagerResolveRendersCachedCredentialPerRequestConfig(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	mgr := NewManager()
	mgr.now = func() time.Time { return time.Unix(100, 0) }
	mgr.SetExecFunc(func(context.Context, Config) ([]byte, error) {
		calls.Add(1)
		return []byte("token-1"), nil
	})

	base := Config{
		Argv:     []string{"gh", "auth", "token"},
		CacheKey: "github",
	}
	authCfg := base
	authCfg.Scheme = "Token"

	customCfg := Config{
		CacheKey: "github",
		Header:   "X-Registry-Token",
	}

	first, err := mgr.Resolve(context.Background(), "dev", authCfg)
	if err != nil {
		t.Fatalf("Resolve(authCfg) error = %v", err)
	}
	second, err := mgr.Resolve(context.Background(), "dev", customCfg)
	if err != nil {
		t.Fatalf("Resolve(customCfg) error = %v", err)
	}

	if first.Header != "Authorization" || first.Value != "Token token-1" {
		t.Fatalf("unexpected auth result %#v", first)
	}
	if second.Header != "X-Registry-Token" || second.Value != "Token token-1" {
		t.Fatalf("unexpected custom result %#v", second)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 command execution, got %d", got)
	}
}

func TestManagerResolveRejectsConflictingArgv(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	mgr := NewManager()
	mgr.now = func() time.Time { return time.Unix(100, 0) }
	mgr.SetExecFunc(func(_ context.Context, cfg Config) ([]byte, error) {
		calls.Add(1)
		return []byte(cfg.Argv[len(cfg.Argv)-1]), nil
	})

	alphaCfg := Config{
		Argv:     []string{"tool", "alpha"},
		CacheKey: "shared",
	}
	betaCfg := Config{
		Argv:     []string{"tool", "beta"},
		CacheKey: "shared",
	}

	alpha, err := mgr.Resolve(context.Background(), "dev", alphaCfg)
	if err != nil {
		t.Fatalf("Resolve(alphaCfg) error = %v", err)
	}

	_, err = mgr.Resolve(context.Background(), "dev", betaCfg)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if got := err.Error(); got == "" || !containsAll(got, `cache_key "shared"`, "argv") {
		t.Fatalf("unexpected conflict error %q", got)
	}
	if alpha.Token != "alpha" {
		t.Fatalf("unexpected alpha token %q", alpha.Token)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 command execution, got %d", got)
	}
}

func TestManagerResolveRejectsConflictingExtractionConfig(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	mgr := NewManager()
	mgr.now = func() time.Time { return time.Unix(100, 0) }
	mgr.SetExecFunc(func(_ context.Context, cfg Config) ([]byte, error) {
		calls.Add(1)
		return []byte(`{"alpha":"token-a","beta":"token-b"}`), nil
	})

	alphaCfg := Config{
		Argv:      []string{"tool"},
		CacheKey:  "shared",
		Format:    FormatJSON,
		TokenPath: "alpha",
	}
	betaCfg := Config{
		Argv:      []string{"tool"},
		CacheKey:  "shared",
		Format:    FormatJSON,
		TokenPath: "beta",
	}

	alpha, err := mgr.Resolve(context.Background(), "dev", alphaCfg)
	if err != nil {
		t.Fatalf("Resolve(alphaCfg) error = %v", err)
	}

	_, err = mgr.Resolve(context.Background(), "dev", betaCfg)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if got := err.Error(); got == "" || !containsAll(got, `cache_key "shared"`, "token_path") {
		t.Fatalf("unexpected conflict error %q", got)
	}
	if alpha.Token != "token-a" {
		t.Fatalf("unexpected alpha token %q", alpha.Token)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 command execution, got %d", got)
	}
}

func TestManagerResolveUnseededCacheOnlyFails(t *testing.T) {
	t.Parallel()

	mgr := NewManager()

	_, err := mgr.Resolve(context.Background(), "dev", Config{CacheKey: "github"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !containsAll(got, "requires argv", "seed the cache") {
		t.Fatalf("unexpected error %q", got)
	}
}

func TestManagerPrepareInheritsCachedConfig(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	base := Config{
		Argv:      []string{"gh", "auth", "token"},
		Dir:       "/tmp/project",
		Format:    FormatJSON,
		Header:    "X-Seeded",
		Scheme:    "Token",
		TokenPath: "access_token",
		CacheKey:  "github",
		TTL:       time.Minute,
		Timeout:   3 * time.Second,
	}
	mgr.store(cacheEntryKey("dev", base), base, credential{
		Token:     "abc",
		FetchedAt: time.Unix(100, 0),
	})

	prep, err := mgr.Prepare("dev", Config{
		CacheKey: "github",
		Header:   "Authorization",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	merged := prep.cfg

	if len(merged.Argv) != 3 || merged.Argv[0] != "gh" {
		t.Fatalf("expected argv to be inherited, got %#v", merged.Argv)
	}
	if merged.Format != FormatJSON || merged.TokenPath != "access_token" {
		t.Fatalf("expected extraction config to be inherited, got %#v", merged)
	}
	if merged.Header != "Authorization" {
		t.Fatalf("expected request header override to win, got %q", merged.Header)
	}
	if merged.Scheme != "Token" {
		t.Fatalf("expected scheme to be inherited, got %q", merged.Scheme)
	}
	if merged.Timeout != 3*time.Second {
		t.Fatalf("expected timeout to be inherited, got %s", merged.Timeout)
	}
}

func TestManagerResolveDeduplicatesInflightCalls(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	start := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once

	mgr := NewManager()
	mgr.now = func() time.Time { return time.Unix(100, 0) }
	mgr.SetExecFunc(func(context.Context, Config) ([]byte, error) {
		calls.Add(1)
		startOnce.Do(func() {
			close(start)
		})
		<-release
		return []byte("token-1"), nil
	})

	cfg := Config{Argv: []string{"gh"}, CacheKey: "github"}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	ress := make(chan Result, 2)

	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := mgr.Resolve(context.Background(), "dev", cfg)
			errs <- err
			ress <- res
		}()
	}

	<-start
	close(release)
	wg.Wait()
	close(errs)
	close(ress)

	for err := range errs {
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
	}

	var first Result
	i := 0
	for res := range ress {
		if i == 0 {
			first = res
		} else if res.Token != first.Token {
			t.Fatalf("expected shared token, got %q and %q", first.Token, res.Token)
		}
		i++
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 inflight run, got %d", got)
	}
}

func TestManagerSnapshotRestore(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	cfg := Config{
		Argv:     []string{"gh", "auth", "token"},
		CacheKey: "github",
		Header:   "X-Token",
		Scheme:   "Token",
		TTL:      time.Hour,
	}
	key := cacheEntryKey("dev", cfg.normalize())
	mgr.store(key, cfg.normalize(), credential{
		Token:     "abc",
		Type:      "Token",
		FetchedAt: time.Unix(100, 0),
		Expiry:    time.Unix(200, 0),
	})

	snap := mgr.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one snapshot entry, got %+v", snap)
	}

	restored := NewManager()
	restored.now = func() time.Time { return time.Unix(150, 0) }
	restored.Restore(snap)

	res, ok, err := restored.Cached("dev", Config{CacheKey: "github"})
	if err != nil {
		t.Fatalf("Cached() error = %v", err)
	}
	if !ok {
		t.Fatal("expected restored cached auth result")
	}
	if res.Token != "abc" {
		t.Fatalf("expected restored token, got %q", res.Token)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
