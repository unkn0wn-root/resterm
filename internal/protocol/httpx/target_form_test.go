package httpx

import (
	"net/url"
	"testing"
)

func TestClassifyTarget(t *testing.T) {
	tests := []struct {
		raw  string
		want targetForm
	}{
		// Relative references continue to use base-url.
		{"", formReference},
		{"users", formReference},
		{"/users", formReference},
		{"../health", formReference},
		{"?page=2", formReference},
		{"#section", formReference},
		{"//up.example.com/x", formReference},
		{"example.com/users", formReference},
		{"example.com", formReference},
		{"files/a:b", formReference},
		{"a/b:c/d", formReference},
		{":8080/users", formReference},
		{":", formReference},

		// Hosts without a scheme.
		{"localhost:8080/users", formAuthority},
		{"localhost:8080", formAuthority},
		{"localhost:8080?q=1", formAuthority},
		{"127.0.0.1:8080/users", formAuthority},
		{"proxy.example.com:443", formAuthority},
		{"[::1]:8080/users", formAuthority},
		{"[::1]/path", formAuthority},
		{"[::1]", formAuthority},
		{"{{host}}:8080/x", formAuthority},
		// A URL in a query or fragment must not affect how the request URL is
		// classified.
		{"localhost:8080/path?next=http://example.com", formAuthority},
		{"localhost:8080/path#next=http://example.com", formAuthority},

		// Explicit and opaque URI schemes.
		{"http://x/y", formAbsolute},
		{"https://x/y", formAbsolute},
		{"ws://x/y", formAbsolute},
		{"wss://x/y", formAbsolute},
		{"HTTP://x/y", formAbsolute},
		{"ftp://x/y", formAbsolute},
		// A known scheme without "//" is an invalid absolute URL, not a host.
		// Reading "https:443/path" as a host would reach a server called
		// "https" over plain HTTP.
		{"https:443/path", formAbsolute},
		{"http:8080/path", formAbsolute},
		{"ws:8080/path", formAbsolute},
		{"localhost:/path", formAbsolute},
		{"localhost:80a/x", formAbsolute},
		{"mailto:someone@example.com", formAbsolute},
		{"data:text/html,x", formAbsolute},
		{"javascript:alert(1)", formAbsolute},
		{"a:b/c", formAbsolute},
		{"example.com:8080@evil.com/x", formAbsolute},
	}

	// Reject userinfo before a schemeless host because net/http would turn it
	// into an Authorization header.
	credentials := []struct {
		raw  string
		want targetForm
	}{
		{"user:pass@example.com:8080/x", formCredentials},
		{"user@example.com:8080/path", formCredentials},
		{"user@127.0.0.1:8080/path", formCredentials},
		{"user@localhost:8080", formCredentials},
		{"user@[::1]:8080/x", formCredentials},
		{"[::1]@evil.com:8080/x", formCredentials},
		{"u%40b@example.com:8080/x", formCredentials},
		{"a@b@example.com:8080/x", formCredentials},
		// An "@" with no host and port after it is an ordinary path character.
		{"user@example.com/path", formReference},
		{"user@example.com", formReference},
		{"/users/user@example.com", formReference},
		{"mailto:someone@example.com", formAbsolute},
	}

	tests = append(tests, credentials...)

	names := map[targetForm]string{
		formReference:   "reference",
		formAbsolute:    "absolute",
		formAuthority:   "authority",
		formCredentials: "credentials",
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := classifyTarget(tt.raw); got != tt.want {
				t.Fatalf("classifyTarget(%q) = %s, want %s", tt.raw, names[got], names[tt.want])
			}
		})
	}
}

// Keep isPort's syntax check in sync with net/url. An empty port is the one
// deliberate exception: net/url allows localhost:/x, but Resterm rejects it.
func TestIsPortAgreesWithNetURL(t *testing.T) {
	for _, port := range []string{"8080", "0", "80", "08080", "99999", "65536", "80a", "+80", "-80", "8_080"} {
		t.Run(port, func(t *testing.T) {
			_, err := url.Parse("http://localhost:" + port + "/x")
			if got, want := isPort(port), err == nil; got != want {
				t.Fatalf("isPort(%q) = %v, but url.Parse accepts it = %v (%v)", port, got, want, err)
			}
		})
	}
}
