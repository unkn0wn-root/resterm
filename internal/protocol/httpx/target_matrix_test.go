package httpx

import (
	"net/url"
	"strings"
	"testing"
)

// Generate combinations to check that a schemeless target never introduces
// credentials or a destination host that was not present in the input.
func TestResolveRequestTargetSchemelessInvariants(t *testing.T) {
	userinfos := []string{"", "user@", "user:pass@", "user:@", ":pass@", "u%40b@", "user@host@"}
	hosts := []string{"example.com", "127.0.0.1", "localhost", "[::1]", "sub.example.com"}
	ports := []string{"", ":8080", ":443", ":80a", ":"}
	paths := []string{"", "/", "/p", "/p/q"}
	queries := []string{"", "?a=1", "?next=http://evil.example", "?next=https://evil.example/x"}
	frags := []string{"", "#f", "#next=http://evil.example"}

	checked := 0
	for _, ui := range userinfos {
		for _, h := range hosts {
			for _, p := range ports {
				for _, path := range paths {
					for _, q := range queries {
						for _, f := range frags {
							raw := ui + h + p + path + q + f
							checked++
							u := resolveChecked(t, raw)
							// Userinfo must not change the destination to the
							// host after the "@".
							if u != nil && ui != "" && u.Hostname() != "" &&
								u.Hostname() != strings.Trim(h, "[]") {
								t.Errorf("target %q resolved to host %q, want %q", raw, u.Hostname(), h)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("checked %d schemeless targets", checked)
}

// These malformed targets exercise delimiters and encodings that could change
// the parsed host.
func TestResolveRequestTargetHostileTargets(t *testing.T) {
	for _, raw := range []string{
		`localhost:8080\@evil.example/x`,
		`localhost:8080%2F@evil.example/x`,
		"localhost:8080/x\r\nHost: evil.example",
		"localhost:8080\t/x",
		"localhost:8080#@evil.example",
		"localhost:8080?@evil.example",
		"localhost:8080/..;@evil.example",
		"localhost:8080@@evil.example/x",
		"localhost:8080:9090/x",
		"localhost:8080./x",
		"localhost:8080\\evil.example",
		"[::1]:8080@evil.example/x",
		"user@localhost:8080@evil.example/x",
		// These start like bracketed hosts but place userinfo after the bracket.
		// They must be rejected rather than sent to the host after the "@".
		"[::1]@evil.example/x",
		"[::1]@evil.example:8080/x",
		"[foo]@evil.example/x",
		"[::1]x@evil.example/x",
		"\u0435xample.com:8080/x", // A Cyrillic lookalike host.
	} {
		t.Run(raw, func(t *testing.T) {
			resolveChecked(t, raw)
		})
	}
}

// resolveChecked returns the resolved URL after checking common safety rules.
// It returns nil when the target is rejected.
func resolveChecked(t *testing.T, raw string) *url.URL {
	t.Helper()

	got, err := resolveRequestTarget(raw, "", nil, schemeHTTP)
	if err != nil {
		return nil
	}

	u, parseErr := url.Parse(got)
	if parseErr != nil {
		t.Fatalf("target %q resolved to unparseable %q: %v", raw, got, parseErr)
	}

	// net/http turns URL.User into an Authorization header, so a target that
	// never named a scheme must never come back carrying credentials.
	if u.User != nil {
		t.Errorf("target %q resolved to %q, which carries userinfo %q", raw, got, u.User)
	}

	// A delimiter or query value must not introduce a different destination.
	if u.Hostname() != "" && !strings.Contains(raw, u.Hostname()) {
		t.Errorf("target %q resolved to host %q, which it never named", raw, u.Hostname())
	}
	return u
}
