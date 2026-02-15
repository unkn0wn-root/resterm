package k8s

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestNormalizeProfileDefaults(t *testing.T) {
	p := restfile.K8sProfile{
		Name: "api",
		Pod:  "api-server",
		Port: 8080, PortStr: "8080",
	}
	cfg, err := NormalizeProfile(p)
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}
	if cfg.Namespace != "default" {
		t.Fatalf("expected default namespace, got %q", cfg.Namespace)
	}
	if cfg.Address != "127.0.0.1" {
		t.Fatalf("expected default address, got %q", cfg.Address)
	}
	if cfg.PodWait != time.Minute {
		t.Fatalf("expected default pod wait 1m, got %s", cfg.PodWait)
	}
	if cfg.TargetKind != targetKindPod || cfg.TargetName != "api-server" {
		t.Fatalf("unexpected target %s/%s", cfg.TargetKind, cfg.TargetName)
	}
}

func TestNormalizeProfileValues(t *testing.T) {
	p := restfile.K8sProfile{
		Name:         "api",
		Namespace:    "prod",
		Target:       "deployment:api",
		PortStr:      "https",
		Context:      "cluster-a",
		Kubeconfig:   "/tmp/kube",
		Container:    "api",
		Address:      "0.0.0.0",
		LocalPortStr: "18080",
		PodWaitStr:   "20s",
		RetriesStr:   "2",
		Persist:      restfile.Opt[bool]{Val: true, Set: true},
	}
	cfg, err := NormalizeProfile(p)
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}
	if cfg.Port != 0 || cfg.PortName != "https" || cfg.PortRaw != "https" {
		t.Fatalf("unexpected port: %d (%q)", cfg.Port, cfg.PortRaw)
	}
	if cfg.TargetKind != targetKindDeployment || cfg.TargetName != "api" {
		t.Fatalf("unexpected target %s/%s", cfg.TargetKind, cfg.TargetName)
	}
	if cfg.Pod != "" {
		t.Fatalf("expected pod empty for deployment target, got %q", cfg.Pod)
	}
	if cfg.LocalPort != 18080 || cfg.LocalPortRaw != "18080" {
		t.Fatalf("unexpected local port: %d (%q)", cfg.LocalPort, cfg.LocalPortRaw)
	}
	if cfg.PodWait != 20*time.Second || cfg.PodWaitRaw != "20s" {
		t.Fatalf("unexpected pod wait: %v (%q)", cfg.PodWait, cfg.PodWaitRaw)
	}
	if cfg.Retries != 2 || cfg.RetriesRaw != "2" {
		t.Fatalf("unexpected retries: %d (%q)", cfg.Retries, cfg.RetriesRaw)
	}
	if !cfg.Persist {
		t.Fatalf("expected persist true")
	}
}

func TestNormalizeProfileTrimsWhitespace(t *testing.T) {
	t.Run("target and numeric port", func(t *testing.T) {
		cfg, err := NormalizeProfile(restfile.K8sProfile{
			Namespace: " default ",
			Target:    "pod:api",
			Pod:       " api ",
			PortStr:   " 8080 ",
		})
		if err != nil {
			t.Fatalf("normalize err: %v", err)
		}
		if cfg.Namespace != "default" {
			t.Fatalf("expected default namespace, got %q", cfg.Namespace)
		}
		if cfg.TargetKind != targetKindPod || cfg.TargetName != "api" || cfg.Pod != "api" {
			t.Fatalf("unexpected target %s/%s (%q)", cfg.TargetKind, cfg.TargetName, cfg.Pod)
		}
		if cfg.Port != 8080 || cfg.PortName != "" || cfg.PortRaw != "8080" {
			t.Fatalf("unexpected port parse: %d name=%q raw=%q", cfg.Port, cfg.PortName, cfg.PortRaw)
		}
	})

	t.Run("named port", func(t *testing.T) {
		cfg, err := NormalizeProfile(restfile.K8sProfile{
			Pod:     "api",
			PortStr: " http ",
		})
		if err != nil {
			t.Fatalf("normalize err: %v", err)
		}
		if cfg.Port != 0 || cfg.PortName != "http" || cfg.PortRaw != "http" {
			t.Fatalf("unexpected named port parse: %d name=%q raw=%q", cfg.Port, cfg.PortName, cfg.PortRaw)
		}
	})
}

func TestNormalizeProfileExpandsKubeconfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	p := restfile.K8sProfile{
		Pod:        "api",
		PortStr:    "8080",
		Kubeconfig: "~/.kube/config",
	}
	cfg, err := NormalizeProfile(p)
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}
	want := filepath.Join(home, ".kube", "config")
	if cfg.Kubeconfig != want {
		t.Fatalf("expected kubeconfig %q, got %q", want, cfg.Kubeconfig)
	}
}

func TestNormalizeProfileRejectsInvalid(t *testing.T) {
	t.Run("missing target", func(t *testing.T) {
		_, err := NormalizeProfile(restfile.K8sProfile{PortStr: "8080"})
		if err == nil {
			t.Fatalf("expected target error")
		}
	})
	t.Run("missing port", func(t *testing.T) {
		_, err := NormalizeProfile(restfile.K8sProfile{Pod: "api"})
		if err == nil {
			t.Fatalf("expected port error")
		}
	})
	t.Run("target conflicts with pod", func(t *testing.T) {
		_, err := NormalizeProfile(restfile.K8sProfile{
			Target:  "service:api",
			Pod:     "api-0",
			PortStr: "8080",
		})
		if err == nil {
			t.Fatalf("expected conflict error")
		}
	})
	t.Run("invalid target kind", func(t *testing.T) {
		_, err := NormalizeProfile(restfile.K8sProfile{
			Target:  "job:api",
			PortStr: "8080",
		})
		if err == nil {
			t.Fatalf("expected invalid target kind error")
		}
	})
	t.Run("bad port", func(t *testing.T) {
		_, err := NormalizeProfile(restfile.K8sProfile{Pod: "api", PortStr: "bad port"})
		if err == nil {
			t.Fatalf("expected bad port error")
		}
	})
	t.Run("bad named port token", func(t *testing.T) {
		_, err := NormalizeProfile(restfile.K8sProfile{Pod: "api", PortStr: "!!!"})
		if err == nil {
			t.Fatalf("expected bad named port error")
		}
	})
	t.Run("bad partial template port token", func(t *testing.T) {
		_, err := NormalizeProfile(restfile.K8sProfile{Pod: "api", PortStr: "{{port_name"})
		if err == nil {
			t.Fatalf("expected bad partial template port error")
		}
	})
	t.Run("bad local port", func(t *testing.T) {
		_, err := NormalizeProfile(
			restfile.K8sProfile{Pod: "api", PortStr: "8080", LocalPortStr: "0"},
		)
		if err == nil {
			t.Fatalf("expected bad local port error")
		}
	})
	t.Run("bad pod wait", func(t *testing.T) {
		_, err := NormalizeProfile(
			restfile.K8sProfile{Pod: "api", PortStr: "8080", PodWaitStr: "bad"},
		)
		if err == nil {
			t.Fatalf("expected bad pod wait error")
		}
	})
	t.Run("bad retries", func(t *testing.T) {
		_, err := NormalizeProfile(
			restfile.K8sProfile{Pod: "api", PortStr: "8080", RetriesStr: "-1"},
		)
		if err == nil {
			t.Fatalf("expected bad retries error")
		}
	})
}
