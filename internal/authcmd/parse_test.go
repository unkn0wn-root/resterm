package authcmd

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params map[string]string
		check  func(t *testing.T, cfg Config)
	}{
		{
			name: "text defaults",
			params: map[string]string{
				"argv": `["gh","auth","token"]`,
			},
			check: func(t *testing.T, cfg Config) {
				t.Helper()
				if cfg.Format != FormatText {
					t.Fatalf("expected text format, got %q", cfg.Format)
				}
				if cfg.Header != "Authorization" {
					t.Fatalf("expected Authorization header, got %q", cfg.Header)
				}
				if got := len(cfg.Argv); got != 3 {
					t.Fatalf("expected 3 argv entries, got %d", got)
				}
			},
		},
		{
			name: "json with optional fields",
			params: map[string]string{
				"argv":            `["mycli","auth","print","--json"]`,
				"format":          "json",
				"header":          "X-Access-Token",
				"scheme":          "Token",
				"token_path":      "data.token",
				"type_path":       "type",
				"expiry_path":     "exp",
				"expires_in_path": "expires_in",
				"cache_key":       "myapi",
				"ttl":             "10m",
				"timeout":         "3s",
			},
			check: func(t *testing.T, cfg Config) {
				t.Helper()
				if cfg.Format != FormatJSON {
					t.Fatalf("expected json format, got %q", cfg.Format)
				}
				if cfg.Header != "X-Access-Token" {
					t.Fatalf("expected custom header, got %q", cfg.Header)
				}
				if cfg.CacheKey != "myapi" {
					t.Fatalf("expected cache key, got %q", cfg.CacheKey)
				}
				if cfg.TTL != 10*time.Minute {
					t.Fatalf("expected ttl 10m, got %s", cfg.TTL)
				}
				if cfg.Timeout != 3*time.Second {
					t.Fatalf("expected timeout 3s, got %s", cfg.Timeout)
				}
			},
		},
		{
			name: "normalizes parsed fields",
			params: map[string]string{
				"argv":            `[" gh ","auth","token"]`,
				"format":          " JSON ",
				"header":          " Authorization ",
				"scheme":          " Token ",
				"token_path":      " access_token ",
				"type_path":       " token_type ",
				"expiry_path":     " expiry ",
				"expires_in_path": " expires_in ",
				"cache_key":       " github ",
			},
			check: func(t *testing.T, cfg Config) {
				t.Helper()
				if cfg.Argv[0] != "gh" {
					t.Fatalf("expected trimmed argv[0], got %q", cfg.Argv[0])
				}
				if cfg.Format != FormatJSON {
					t.Fatalf("expected normalized json format, got %q", cfg.Format)
				}
				if cfg.Header != "Authorization" {
					t.Fatalf("expected normalized header, got %q", cfg.Header)
				}
				if cfg.Scheme != "Token" {
					t.Fatalf("expected normalized scheme, got %q", cfg.Scheme)
				}
				if cfg.TokenPath != "access_token" {
					t.Fatalf("expected normalized token path, got %q", cfg.TokenPath)
				}
				if cfg.CacheKey != "github" {
					t.Fatalf("expected normalized cache key, got %q", cfg.CacheKey)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := Parse(tt.params, "/tmp/project")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if cfg.Dir != "/tmp/project" {
				t.Fatalf("expected dir to be set, got %q", cfg.Dir)
			}
			tt.check(t, cfg)
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{
			name: "missing argv",
			want: "@auth command requires argv",
		},
		{
			name: "invalid argv json",
			params: map[string]string{
				"argv": `["gh",]`,
			},
			want: "decode command argv",
		},
		{
			name: "empty argv",
			params: map[string]string{
				"argv": `[]`,
			},
			want: "must not be empty",
		},
		{
			name: "empty argv zero",
			params: map[string]string{
				"argv": `["","auth"]`,
			},
			want: "argv[0] must not be empty",
		},
		{
			name: "invalid format",
			params: map[string]string{
				"argv":   `["gh","auth","token"]`,
				"format": "yaml",
			},
			want: "unsupported command auth format",
		},
		{
			name: "json missing token path",
			params: map[string]string{
				"argv":   `["gh","auth","token"]`,
				"format": "json",
			},
			want: "token_path is required",
		},
		{
			name: "ttl without cache key",
			params: map[string]string{
				"argv": `["gh","auth","token"]`,
				"ttl":  "5m",
			},
			want: "ttl requires cache_key",
		},
		{
			name: "invalid ttl",
			params: map[string]string{
				"argv":      `["gh","auth","token"]`,
				"cache_key": "gh",
				"ttl":       "tomorrow",
			},
			want: "invalid ttl duration",
		},
		{
			name: "invalid timeout",
			params: map[string]string{
				"argv":    `["gh","auth","token"]`,
				"timeout": "-1s",
			},
			want: "timeout must not be negative",
		},
		{
			name: "rejects shell",
			params: map[string]string{
				"argv": `["bash","-lc","gh auth token"]`,
			},
			want: "does not allow shell front-end",
		},
		{
			name: "rejects shell exe path",
			params: map[string]string{
				"argv": `["C:/Program Files/Git/bin/bash.exe","-lc","gh auth token"]`,
			},
			want: "does not allow shell front-end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(tt.params, "")
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); !strings.Contains(got, tt.want) {
				t.Fatalf("Parse() error = %q, want substring %q", got, tt.want)
			}
		})
	}
}
