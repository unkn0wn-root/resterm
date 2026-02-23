package connprofile

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestExpandPathSupportsHomeAndNamedUser(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	got, err := ExpandPath("~/kube/config", "home err")
	if err != nil {
		t.Fatalf("expand path: %v", err)
	}
	if got != filepath.Clean(filepath.Join(home, "kube", "config")) {
		t.Fatalf("unexpected expanded path %q", got)
	}

	current, err := user.Current()
	if err != nil || strings.TrimSpace(current.Username) == "" ||
		strings.TrimSpace(current.HomeDir) == "" {
		t.Skip("current user lookup unavailable")
	}
	got, err = ExpandPath("~"+current.Username+"/.ssh/config", "home err")
	if err != nil {
		t.Fatalf("expand named-user path: %v", err)
	}
	want := filepath.Clean(filepath.Join(current.HomeDir, ".ssh", "config"))
	if got != want {
		t.Fatalf("unexpected named-user path: got %q want %q", got, want)
	}
}

func TestExpandPathUnknownNamedUserReturnsError(t *testing.T) {
	_, err := ExpandPath("~definitely_not_a_real_user_1234/.ssh/config", "custom home err")
	if err == nil || !strings.Contains(err.Error(), "custom home err") {
		t.Fatalf("expected custom home error, got %v", err)
	}
}

func TestExpandValueEnvLookup(t *testing.T) {
	t.Setenv("EXPAND_VALUE_KEY", "abc123")
	got, err := ExpandValue("env:EXPAND_VALUE_KEY", nil)
	if err != nil {
		t.Fatalf("expand env value: %v", err)
	}
	if got != "abc123" {
		t.Fatalf("unexpected env expansion %q", got)
	}
}

func TestExpandValueEnvLookupUppercaseFallback(t *testing.T) {
	t.Setenv("EXPAND_VALUE_UPPER", "upper")
	got, err := ExpandValue("env:expand_value_upper", nil)
	if err != nil {
		t.Fatalf("expand env value with uppercase fallback: %v", err)
	}
	if got != "upper" {
		t.Fatalf("unexpected env expansion %q", got)
	}
}

func TestExpandValueEnvLookupResolverFallback(t *testing.T) {
	resolver := vars.NewResolver(
		vars.NewMapProvider("env", map[string]string{"custom_token": "from-resolver"}),
	)
	got, err := ExpandValue("env:custom_token", resolver)
	if err != nil {
		t.Fatalf("expand env via resolver fallback: %v", err)
	}
	if got != "from-resolver" {
		t.Fatalf("unexpected resolver expansion %q", got)
	}
}

func TestExpandValueMissingEnvReturnsError(t *testing.T) {
	_, err := ExpandValue("env:DOES_NOT_EXIST_12345", nil)
	if err == nil || !strings.Contains(err.Error(), "undefined env variable") {
		t.Fatalf("expected missing env error, got %v", err)
	}
}
