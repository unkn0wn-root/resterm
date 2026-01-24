package ssh

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestResolveUseMissingWithInline(t *testing.T) {
	spec := &restfile.SSHSpec{
		Use: "missing",
		Inline: &restfile.SSHProfile{
			Host: "jump",
		},
	}

	if _, err := Resolve(spec, nil, nil, nil, ""); err == nil {
		t.Fatalf("expected error for missing profile")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveUseWithInlineOverrides(t *testing.T) {
	spec := &restfile.SSHSpec{
		Use: "edge",
		Inline: &restfile.SSHProfile{
			Host: "inline-host",
		},
	}
	fileProfiles := []restfile.SSHProfile{
		{Scope: restfile.SSHScopeFile, Name: "edge", Host: "profile-host"},
	}

	cfg, err := Resolve(spec, fileProfiles, nil, nil, "")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Host != "inline-host" {
		t.Fatalf("expected inline host override, got %q", cfg.Host)
	}
}
