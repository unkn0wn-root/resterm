package authcmd

import (
	"testing"
	"time"
)

func TestConfigHeaderName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "default", want: "Authorization"},
		{name: "custom", cfg: Config{Header: "X-API-Key"}, want: "X-API-Key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.HeaderName(); got != tt.want {
				t.Fatalf("HeaderName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigUsesCache(t *testing.T) {
	t.Parallel()

	if (Config{}).usesCache() {
		t.Fatal("expected empty config to skip cache")
	}
	if !(Config{CacheKey: " key "}).usesCache() {
		t.Fatal("expected non-empty cache key to enable cache")
	}
}

func TestConfigNormalize(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Argv:          []string{" gh ", " auth ", " token "},
		Dir:           " /tmp/project ",
		Format:        Format(" JSON "),
		Header:        " Authorization ",
		Scheme:        " Token ",
		TokenPath:     " access_token ",
		TypePath:      " token_type ",
		ExpiryPath:    " expiry ",
		ExpiresInPath: " expires_in ",
		CacheKey:      " github ",
	}

	got := cfg.normalize()
	if got.Argv[0] != "gh" {
		t.Fatalf("expected argv[0] to be trimmed, got %q", got.Argv[0])
	}
	if got.Argv[1] != " auth " {
		t.Fatalf("expected non-command argv entries to remain unchanged, got %q", got.Argv[1])
	}
	if got.Dir != "/tmp/project" {
		t.Fatalf("expected trimmed dir, got %q", got.Dir)
	}
	if got.Format != FormatJSON {
		t.Fatalf("expected json format, got %q", got.Format)
	}
	if got.Header != "Authorization" {
		t.Fatalf("expected trimmed header, got %q", got.Header)
	}
	if got.Scheme != "Token" {
		t.Fatalf("expected trimmed scheme, got %q", got.Scheme)
	}
	if got.TokenPath != "access_token" {
		t.Fatalf("expected trimmed token path, got %q", got.TokenPath)
	}
	if got.TypePath != "token_type" {
		t.Fatalf("expected trimmed type path, got %q", got.TypePath)
	}
	if got.ExpiryPath != "expiry" {
		t.Fatalf("expected trimmed expiry path, got %q", got.ExpiryPath)
	}
	if got.ExpiresInPath != "expires_in" {
		t.Fatalf("expected trimmed expires_in path, got %q", got.ExpiresInPath)
	}
	if got.CacheKey != "github" {
		t.Fatalf("expected trimmed cache key, got %q", got.CacheKey)
	}
}

func TestConfigTimeoutFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		base time.Duration
		want time.Duration
	}{
		{name: "uses base", base: 5 * time.Second, want: 5 * time.Second},
		{
			name: "uses command timeout when smaller",
			cfg:  Config{Timeout: 3 * time.Second},
			base: 5 * time.Second,
			want: 3 * time.Second,
		},
		{
			name: "keeps base when command timeout larger",
			cfg:  Config{Timeout: 7 * time.Second},
			base: 5 * time.Second,
			want: 5 * time.Second,
		},
		{
			name: "uses command timeout when base missing",
			cfg:  Config{Timeout: 2 * time.Second},
			want: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.timeoutFor(tt.base); got != tt.want {
				t.Fatalf("timeoutFor(%s) = %s, want %s", tt.base, got, tt.want)
			}
		})
	}
}

func TestConfigWithBaseTimeout(t *testing.T) {
	t.Parallel()

	cfg := Config{Timeout: 3 * time.Second}.WithBaseTimeout(5 * time.Second)
	if cfg.Timeout != 3*time.Second {
		t.Fatalf("expected smaller command timeout, got %s", cfg.Timeout)
	}

	cfg = (Config{}).WithBaseTimeout(5 * time.Second)
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("expected base timeout, got %s", cfg.Timeout)
	}
}
