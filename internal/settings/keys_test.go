package settings

import "testing"

func TestIsHTTPKey(t *testing.T) {
	ok := []string{
		"timeout",
		"proxy",
		"followredirects",
		"insecure",
		"http-root-cas",
		"HTTP-CLIENT-CERT",
	}
	for _, k := range ok {
		if !IsHTTPKey(k) {
			t.Fatalf("expected http key %q to be supported", k)
		}
	}

	bad := []string{"grpc-timeout", "", "foo", "httpx-root"}
	for _, k := range bad {
		if IsHTTPKey(k) {
			t.Fatalf("expected http key %q to be unsupported", k)
		}
	}
}
