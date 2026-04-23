package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withMissingDefaultKeys(t *testing.T) {
	t.Helper()
	old := defaultKeyPaths
	defaultKeyPaths = []string{filepath.Join(t.TempDir(), "missing")}
	t.Cleanup(func() {
		defaultKeyPaths = old
	})
}

func TestAuthMethodsNoAuth(t *testing.T) {
	withMissingDefaultKeys(t)
	t.Setenv("SSH_AUTH_SOCK", "")

	_, closeFn, err := (authSpec{}).methods()
	if err == nil || !strings.Contains(err.Error(), "no ssh auth methods") {
		t.Fatalf("expected no auth methods error, got %v", err)
	}
	if closeFn != nil {
		t.Fatalf("expected nil close function")
	}
}

func TestAuthMethodsPasswordOnly(t *testing.T) {
	withMissingDefaultKeys(t)
	t.Setenv("SSH_AUTH_SOCK", "")

	methods, closeFn, err := (authSpec{pass: "pw"}).methods()
	if err != nil {
		t.Fatalf("auth methods err: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("expected one password auth method, got %d", len(methods))
	}
	if closeFn != nil {
		t.Fatalf("expected nil close function")
	}
}

func TestAuthMethodsExplicitKeyReadError(t *testing.T) {
	withMissingDefaultKeys(t)
	t.Setenv("SSH_AUTH_SOCK", "")

	missing := filepath.Join(t.TempDir(), "missing-key")
	_, closeFn, err := (authSpec{keyPath: missing, pass: "pw"}).methods()
	if err == nil || !strings.Contains(err.Error(), "read ssh key") {
		t.Fatalf("expected read ssh key error, got %v", err)
	}
	if closeFn != nil {
		t.Fatalf("expected nil close function")
	}
}

func TestAuthMethodsExplicitKeyParseErrorBeforePassword(t *testing.T) {
	withMissingDefaultKeys(t)
	t.Setenv("SSH_AUTH_SOCK", "")

	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte("not a key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_, _, err := (authSpec{keyPath: path, pass: "pw"}).methods()
	if err == nil {
		t.Fatalf("expected explicit key parse error")
	}
	if strings.Contains(err.Error(), "no ssh auth methods") {
		t.Fatalf("expected key parse error before password fallback, got %v", err)
	}
}

func TestAuthMethodsAgentUnavailableSkips(t *testing.T) {
	withMissingDefaultKeys(t)
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing-agent.sock"))

	_, _, err := (authSpec{agent: true}).methods()
	if err == nil || !strings.Contains(err.Error(), "no ssh auth methods") {
		t.Fatalf("expected no auth methods after skipped agent, got %v", err)
	}
}
