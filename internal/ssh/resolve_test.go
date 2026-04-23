package ssh

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
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

func TestResolvePrefersFileProfileOverGlobal(t *testing.T) {
	spec := &restfile.SSHSpec{Use: "edge"}
	fileProfiles := []restfile.SSHProfile{
		{Scope: restfile.SSHScopeFile, Name: "edge", Host: "file-host"},
	}
	globalProfiles := []restfile.SSHProfile{
		{Scope: restfile.SSHScopeGlobal, Name: "edge", Host: "global-host"},
	}

	cfg, err := Resolve(spec, fileProfiles, globalProfiles, nil, "")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Host != "file-host" {
		t.Fatalf("expected file profile host, got %q", cfg.Host)
	}
}

func TestResolveExpandsAfterMerge(t *testing.T) {
	tmp := t.TempDir()
	key := filepath.Join(tmp, "id_ed25519")
	kh := filepath.Join(tmp, "known_hosts")
	spec := &restfile.SSHSpec{
		Use: "edge",
		Inline: &restfile.SSHProfile{
			PortStr: "{{ssh_port}}",
			User:    "{{ssh_user}}",
			Pass:    "env:ssh_pw",
		},
	}
	fileProfiles := []restfile.SSHProfile{{
		Scope:      restfile.SSHScopeFile,
		Name:       "edge",
		Host:       "{{ssh_host}}",
		Key:        "{{ssh_key}}",
		KnownHosts: "{{ssh_known_hosts}}",
	}}
	res := vars.NewResolver(vars.NewMapProvider("test", map[string]string{
		"ssh_host":        "jump.example.com",
		"ssh_port":        "2022",
		"ssh_user":        "ops",
		"ssh_pw":          "secret",
		"ssh_key":         key,
		"ssh_known_hosts": kh,
	}))

	cfg, err := Resolve(spec, fileProfiles, nil, res, "dev")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Host != "jump.example.com" || cfg.Port != 2022 || cfg.User != "ops" {
		t.Fatalf("unexpected expanded endpoint: %+v", cfg)
	}
	if cfg.Pass != "secret" || cfg.KeyPath != key || cfg.KnownHosts != kh {
		t.Fatalf("unexpected expanded auth fields: %+v", cfg)
	}
	if cfg.Label != "dev" {
		t.Fatalf("expected env label dev, got %q", cfg.Label)
	}
}

func TestResolveInlineOverridesOptionalFields(t *testing.T) {
	spec := &restfile.SSHSpec{
		Use: "edge",
		Inline: &restfile.SSHProfile{
			Host:         "inline-host",
			PortStr:      "2222",
			User:         "inline-user",
			Pass:         "inline-pass",
			Key:          "/tmp/inline-key",
			KeyPass:      "inline-key-pass",
			KnownHosts:   "/tmp/inline-known-hosts",
			Agent:        restfile.Opt[bool]{Val: false, Set: true},
			Strict:       restfile.Opt[bool]{Val: false, Set: true},
			Persist:      restfile.Opt[bool]{Val: true, Set: true},
			TimeoutStr:   "5s",
			KeepAliveStr: "2s",
			RetriesStr:   "4",
		},
	}
	fileProfiles := []restfile.SSHProfile{{
		Scope:        restfile.SSHScopeFile,
		Name:         "edge",
		Host:         "profile-host",
		PortStr:      "22",
		User:         "profile-user",
		Pass:         "profile-pass",
		Key:          "/tmp/profile-key",
		KeyPass:      "profile-key-pass",
		KnownHosts:   "/tmp/profile-known-hosts",
		Agent:        restfile.Opt[bool]{Val: true, Set: true},
		Strict:       restfile.Opt[bool]{Val: true, Set: true},
		Persist:      restfile.Opt[bool]{Val: false, Set: true},
		TimeoutStr:   "30s",
		KeepAliveStr: "10s",
		RetriesStr:   "1",
	}}

	cfg, err := Resolve(spec, fileProfiles, nil, nil, "")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Host != "inline-host" ||
		cfg.Port != 2222 ||
		cfg.User != "inline-user" ||
		cfg.Pass != "inline-pass" ||
		cfg.KeyPath != "/tmp/inline-key" ||
		cfg.KeyPass != "inline-key-pass" ||
		cfg.KnownHosts != "/tmp/inline-known-hosts" {
		t.Fatalf("inline string overrides failed: %+v", cfg)
	}
	if cfg.Agent || cfg.Strict || !cfg.Persist {
		t.Fatalf("inline bool overrides failed: %+v", cfg)
	}
	if cfg.TimeoutRaw != "5s" || cfg.KeepAliveRaw != "2s" || cfg.Retries != 4 {
		t.Fatalf("inline option overrides failed: %+v", cfg)
	}
}
