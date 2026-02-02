package main

import "testing"

func TestInstallSrcForFlag(t *testing.T) {
	got := installSrcFor("darwin", "homebrew", "/tmp/resterm", envMap(nil))
	if got != srcBrew {
		t.Fatalf("expected %q, got %q", srcBrew, got)
	}
}

func TestInstallSrcForCellar(t *testing.T) {
	env := envMap(map[string]string{
		"HOMEBREW_CELLAR": "/opt/homebrew/Cellar",
	})
	exe := "/opt/homebrew/Cellar/resterm/1.2.3/bin/resterm"
	got := installSrcFor("darwin", "", exe, env)
	if got != srcBrew {
		t.Fatalf("expected %q, got %q", srcBrew, got)
	}
}

func TestInstallSrcForNoFalsePositive(t *testing.T) {
	exe := "/usr/local/bin/resterm"
	got := installSrcFor("darwin", "", exe, envMap(nil))
	if got != srcDirect {
		t.Fatalf("expected %q, got %q", srcDirect, got)
	}
}

func TestUpdCmd(t *testing.T) {
	if got := updCmd("homebrew"); got != cmdBrew {
		t.Fatalf("expected %q, got %q", cmdBrew, got)
	}
	if got := updCmd("direct"); got != cmdDirect {
		t.Fatalf("expected %q, got %q", cmdDirect, got)
	}
}

func envMap(m map[string]string) envFn {
	return func(k string) string {
		return m[k]
	}
}
