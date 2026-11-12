package httpx

import (
	"strings"
	"testing"
	"time"
)

func optionsFrom(t *testing.T, settings map[string]string) Options {
	t.Helper()
	var opts Options
	if err := ApplyOptionSettings(&opts, settings); err != nil {
		t.Fatalf("ApplyOptionSettings(%v) error = %v", settings, err)
	}
	return opts
}

func TestApplyOptionSettingsRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		val  string
		want string
	}{
		{name: "timeout", key: "timeout", val: "5sec", want: `invalid timeout "5sec" (use a duration such as 30s)`},
		{name: "followredirects", key: "followredirects", val: "maybe", want: `invalid followredirects "maybe" (use true or false)`},
		{name: "insecure", key: "insecure", val: "maybe", want: `invalid insecure "maybe" (use true or false)`},
		{name: "no-cookies", key: "no-cookies", val: "maybe", want: `invalid no-cookies "maybe" (use true or false)`},
		{
			name: "ws-compression",
			key:  "ws-compression",
			val:  "maybe",
			want: `invalid ws-compression "maybe" (use true or false)`,
		},
		{name: "http-version", key: "http-version", val: "unsupported", want: `invalid http-version "unsupported"`},
		{
			name: "http-version 1.0",
			key:  "http-version",
			val:  "1.0",
			want: `invalid http-version "1.0" (use 1.1, 2 or HTTP/1.1, HTTP/2)`,
		},
		// A bare "@setting insecure" writes no value at all, so say it is missing.
		{name: "bare bool", key: "insecure", val: "", want: "missing insecure value (use true or false)"},
		{
			name: "bare timeout",
			key:  "timeout",
			val:  "",
			want: "missing timeout value (use a duration such as 30s)",
		},
		{
			name: "empty base url",
			key:  "base-url",
			val:  "",
			want: "missing base-url value",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{Timeout: time.Second}
			err := ApplyOptionSettings(&opts, map[string]string{tc.key: tc.val})
			if err == nil {
				t.Fatalf("expected an error for %s=%s", tc.key, tc.val)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
			if opts.Timeout != time.Second {
				t.Fatalf("expected the rejected value to leave options untouched, got %v", opts.Timeout)
			}
		})
	}
}

func TestApplyOptionSettingsKeepsBaseURLRawForRequestResolution(t *testing.T) {
	opts := Options{}

	err := ApplyOptionSettings(&opts, map[string]string{
		"base-url": "  {{api.base}}  ",
	})
	if err != nil {
		t.Fatalf("ApplyOptionSettings returned error: %v", err)
	}
	if opts.BaseURL != "{{api.base}}" {
		t.Fatalf("BaseURL = %q, want raw template", opts.BaseURL)
	}
}

// A bare "@setting proxy" now reads as proxy=true, which url.Parse happily
// accepts as a relative path. Without a scheme and host it would only fail once
// the transport tried to dial it.
func TestApplyOptionSettingsValidatesProxy(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want string
	}{
		{name: "http", val: "http://host:8080", want: "http://host:8080"},
		{name: "socks5", val: "socks5://user:pass@host:1080", want: "socks5://user:pass@host:1080"},
		{name: "flag value", val: "true"},
		{name: "no scheme", val: "localhost:8080"},
		{name: "unexpanded template", val: "{{PROXY}}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{}
			err := ApplyOptionSettings(&opts, map[string]string{"proxy": tt.val})
			if tt.want == "" {
				if err == nil {
					t.Fatalf("expected an error for proxy=%s", tt.val)
				}
				if opts.ProxyURL != "" {
					t.Fatalf("ProxyURL = %q, want it left unset", opts.ProxyURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("ApplyOptionSettings returned error: %v", err)
			}
			if opts.ProxyURL != tt.want {
				t.Fatalf("ProxyURL = %q, want %q", opts.ProxyURL, tt.want)
			}
		})
	}
}

// An empty proxy means the setting is not in use, unlike an empty boolean.
func TestApplyOptionSettingsIgnoresEmptyProxy(t *testing.T) {
	opts := Options{ProxyURL: "http://host:8080"}

	if err := ApplyOptionSettings(&opts, map[string]string{"proxy": ""}); err != nil {
		t.Fatalf("ApplyOptionSettings returned error: %v", err)
	}
	if opts.ProxyURL != "http://host:8080" {
		t.Fatalf("ProxyURL = %q, want it unchanged", opts.ProxyURL)
	}
}

func TestApplyOptionSettingsAcceptsBooleanSpellings(t *testing.T) {
	opts := Options{}

	err := ApplyOptionSettings(&opts, map[string]string{
		"insecure":        "yes",
		"followredirects": "off",
	})
	if err != nil {
		t.Fatalf("ApplyOptionSettings returned error: %v", err)
	}
	if !opts.InsecureSkipVerify {
		t.Fatal("expected insecure=yes to enable skip verify")
	}
	if opts.FollowRedirects {
		t.Fatal("expected followredirects=off to disable redirects")
	}
}

func TestApplyOptionSettingsReadsStreamLimits(t *testing.T) {
	var opts Options
	err := ApplyOptionSettings(&opts, map[string]string{
		"sse-max-line-bytes":   "2mb",
		"sse-max-event-bytes":  "16mb",
		"ws-max-message-bytes": "64kb",
	})
	if err != nil {
		t.Fatalf("ApplyOptionSettings: %v", err)
	}
	if opts.SSEMaxLineBytes != 2<<20 {
		t.Errorf("SSEMaxLineBytes = %d, want %d", opts.SSEMaxLineBytes, 2<<20)
	}
	if opts.SSEMaxEventBytes != 16<<20 {
		t.Errorf("SSEMaxEventBytes = %d, want %d", opts.SSEMaxEventBytes, 16<<20)
	}
	if opts.WSMaxMessageBytes != 64<<10 {
		t.Errorf("WSMaxMessageBytes = %d, want %d", opts.WSMaxMessageBytes, 64<<10)
	}
}

func TestApplyOptionSettingsRejectsInvalidStreamLimits(t *testing.T) {
	for _, key := range []string{
		"sse-max-line-bytes",
		"sse-max-event-bytes",
		"ws-max-message-bytes",
	} {
		t.Run(key, func(t *testing.T) {
			var opts Options
			err := ApplyOptionSettings(&opts, map[string]string{key: "none"})
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error = %v, want it to name %s", err, key)
			}
		})
	}
}
