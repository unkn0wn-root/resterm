package ssh

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestNormalizeProfileDefaults(t *testing.T) {
	p := restfile.SSHProfile{
		Name: "edge",
		Host: "10.0.0.1",
	}
	cfg, err := NormalizeProfile(p)
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}
	if cfg.Port != 22 {
		t.Fatalf("expected default port 22, got %d", cfg.Port)
	}
	if !cfg.Strict {
		t.Fatalf("expected strict default true")
	}
	if !cfg.Agent {
		t.Fatalf("expected agent default true")
	}
	if cfg.KnownHosts == "" ||
		!strings.HasSuffix(cfg.KnownHosts, filepath.Join(".ssh", "known_hosts")) {
		t.Fatalf("unexpected known hosts %q", cfg.KnownHosts)
	}
	if cfg.Timeout != defaultTimeout {
		t.Fatalf("unexpected timeout %v", cfg.Timeout)
	}
}

func TestNormalizeProfileValues(t *testing.T) {
	p := restfile.SSHProfile{
		Name:         "named",
		Host:         "jump",
		PortStr:      "2022",
		User:         "ops",
		Pass:         "pw",
		Key:          "/tmp/key",
		KeyPass:      "kp",
		KnownHosts:   "/tmp/kh",
		TimeoutStr:   "5s",
		KeepAliveStr: "2s",
		RetriesStr:   "3",
		Agent:        restfile.SSHOpt[bool]{Val: false, Set: true},
		Strict:       restfile.SSHOpt[bool]{Val: false, Set: true},
		Persist:      restfile.SSHOpt[bool]{Val: true, Set: true},
	}

	cfg, err := NormalizeProfile(p)
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}

	if cfg.Port != 2022 || cfg.PortRaw != "2022" {
		t.Fatalf("port parse failed: %d %q", cfg.Port, cfg.PortRaw)
	}
	if cfg.User != "ops" || cfg.Pass != "pw" || cfg.KeyPath != "/tmp/key" || cfg.KeyPass != "kp" {
		t.Fatalf("unexpected auth fields: %+v", cfg)
	}
	if cfg.KnownHosts != "/tmp/kh" {
		t.Fatalf("unexpected known_hosts %q", cfg.KnownHosts)
	}
	if cfg.Agent {
		t.Fatalf("expected agent false")
	}
	if cfg.Strict {
		t.Fatalf("expected strict false")
	}
	if !cfg.Persist {
		t.Fatalf("expected persist true")
	}
	if cfg.Timeout != 5*time.Second || cfg.TimeoutRaw != "5s" {
		t.Fatalf("timeout parse failed: %v raw=%q", cfg.Timeout, cfg.TimeoutRaw)
	}
	if cfg.KeepAlive != 2*time.Second || cfg.KeepAliveRaw != "2s" {
		t.Fatalf("keepalive parse failed: %v raw=%q", cfg.KeepAlive, cfg.KeepAliveRaw)
	}
	if cfg.Retries != 3 || cfg.RetriesRaw != "3" {
		t.Fatalf("retries parse failed: %d raw=%q", cfg.Retries, cfg.RetriesRaw)
	}
}

func TestNormalizeProfileExpandsPaths(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	p := restfile.SSHProfile{
		Host:       "jump",
		Key:        "~/id_ed25519",
		KnownHosts: "~/known_hosts",
	}
	cfg, err := NormalizeProfile(p)
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}
	if cfg.KeyPath != filepath.Join(tmp, "id_ed25519") {
		t.Fatalf("unexpected key path %q", cfg.KeyPath)
	}
	if cfg.KnownHosts != filepath.Join(tmp, "known_hosts") {
		t.Fatalf("unexpected known_hosts %q", cfg.KnownHosts)
	}
}

func TestNormalizeProfileRejectsInvalidNumbers(t *testing.T) {
	t.Run("port", func(t *testing.T) {
		if _, err := NormalizeProfile(restfile.SSHProfile{Host: "h", PortStr: "-1"}); err == nil {
			t.Fatalf("expected port error")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		if _, err := NormalizeProfile(
			restfile.SSHProfile{Host: "h", TimeoutStr: "nope"},
		); err == nil {
			t.Fatalf("expected timeout error")
		}
	})
	t.Run("keepalive", func(t *testing.T) {
		if _, err := NormalizeProfile(
			restfile.SSHProfile{Host: "h", KeepAliveStr: "-5s"},
		); err == nil {
			t.Fatalf("expected keepalive error")
		}
	})
	t.Run("retries", func(t *testing.T) {
		if _, err := NormalizeProfile(
			restfile.SSHProfile{Host: "h", RetriesStr: "-2"},
		); err == nil {
			t.Fatalf("expected retries error")
		}
	})
}

func TestCacheKeyReflectsAuthAndOptions(t *testing.T) {
	base := Cfg{
		Label:      "env",
		Name:       "edge",
		Host:       "jump",
		Port:       22,
		User:       "ops",
		Pass:       "pw1",
		KeyPass:    "kp1",
		KeyPath:    "/tmp/key1",
		KnownHosts: "/tmp/kh",
		Strict:     true,
		Agent:      true,
		Persist:    true,
		Timeout:    5 * time.Second,
		KeepAlive:  2 * time.Second,
		Retries:    1,
	}
	baseKey := cacheKey(base)

	changedPass := base
	changedPass.Pass = "pw2"
	if cacheKey(changedPass) == baseKey {
		t.Fatalf("expected cache key to change when password changes")
	}

	changedKeyPass := base
	changedKeyPass.KeyPass = "kp2"
	if cacheKey(changedKeyPass) == baseKey {
		t.Fatalf("expected cache key to change when key passphrase changes")
	}

	changedKeepAlive := base
	changedKeepAlive.KeepAlive = base.KeepAlive + time.Second
	if cacheKey(changedKeepAlive) == baseKey {
		t.Fatalf("expected cache key to change when keepalive changes")
	}

	changedRetries := base
	changedRetries.Retries = base.Retries + 1
	if cacheKey(changedRetries) == baseKey {
		t.Fatalf("expected cache key to change when retries changes")
	}
}
