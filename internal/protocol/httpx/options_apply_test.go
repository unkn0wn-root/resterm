package httpx

import (
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/http/version"
)

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

// The send path re-reads settings that were already validated. An unreadable
// value must not stop the readable ones behind it from being applied.
func TestApplyRequestSettingsSkipsUnreadableValues(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	opts := Options{Timeout: time.Second, HTTPVersion: version.V11, CookieJar: jar}

	got := applyRequestSettings(opts, map[string]string{
		"http-version":    "unsupported",
		"timeout":         "5sec",
		"followredirects": "maybe",
		"insecure":        "true",
		"no-cookies":      "true",
	})

	if got.HTTPVersion != version.V11 {
		t.Fatalf("HTTPVersion = %v, want it unchanged", got.HTTPVersion)
	}
	if got.Timeout != time.Second {
		t.Fatalf("Timeout = %v, want it unchanged", got.Timeout)
	}
	if got.FollowRedirects {
		t.Fatal("FollowRedirects should stay at its current value")
	}
	if !got.InsecureSkipVerify {
		t.Fatal("insecure=true follows an unreadable value and still has to apply")
	}
	if got.CookieJar != nil {
		t.Fatal("no-cookies=true follows an unreadable value and still has to apply")
	}
}
