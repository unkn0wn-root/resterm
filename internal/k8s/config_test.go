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
	cfg, err := normalizeProfile(p)
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
	if cfg.Target.Kind != TargetPod || cfg.Target.Name != "api-server" {
		t.Fatalf("unexpected target %s/%s", cfg.Target.Kind, cfg.Target.Name)
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
	cfg, err := normalizeProfile(p)
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}
	if cfg.Port.Number != 0 || cfg.Port.Name != "https" {
		t.Fatalf("unexpected port: %+v", cfg.Port)
	}
	if cfg.Target.Kind != TargetDeployment || cfg.Target.Name != "api" {
		t.Fatalf("unexpected target %s/%s", cfg.Target.Kind, cfg.Target.Name)
	}
	if cfg.LocalPort != 18080 {
		t.Fatalf("unexpected local port: %d", cfg.LocalPort)
	}
	if cfg.PodWait != 20*time.Second {
		t.Fatalf("unexpected pod wait: %v", cfg.PodWait)
	}
	if cfg.Retries != 2 {
		t.Fatalf("unexpected retries: %d", cfg.Retries)
	}
	if !cfg.Persist {
		t.Fatalf("expected persist true")
	}
}

func TestNormalizeProfileTrimsWhitespace(t *testing.T) {
	t.Run("target and numeric port", func(t *testing.T) {
		cfg, err := normalizeProfile(restfile.K8sProfile{
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
		if cfg.Target.Kind != TargetPod || cfg.Target.Name != "api" {
			t.Fatalf("unexpected target %s/%s", cfg.Target.Kind, cfg.Target.Name)
		}
		if cfg.Port.Number != 8080 || cfg.Port.Name != "" {
			t.Fatalf(
				"unexpected port parse: %+v",
				cfg.Port,
			)
		}
	})

	t.Run("named port", func(t *testing.T) {
		cfg, err := normalizeProfile(restfile.K8sProfile{
			Pod:     "api",
			PortStr: " http ",
		})
		if err != nil {
			t.Fatalf("normalize err: %v", err)
		}
		if cfg.Port.Number != 0 || cfg.Port.Name != "http" {
			t.Fatalf(
				"unexpected named port parse: %+v",
				cfg.Port,
			)
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
	cfg, err := normalizeProfile(p)
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
		_, err := normalizeProfile(restfile.K8sProfile{PortStr: "8080"})
		if err == nil {
			t.Fatalf("expected target error")
		}
	})
	t.Run("missing port", func(t *testing.T) {
		_, err := normalizeProfile(restfile.K8sProfile{Pod: "api"})
		if err == nil {
			t.Fatalf("expected port error")
		}
	})
	t.Run("target conflicts with pod", func(t *testing.T) {
		_, err := normalizeProfile(restfile.K8sProfile{
			Target:  "service:api",
			Pod:     "api-0",
			PortStr: "8080",
		})
		if err == nil {
			t.Fatalf("expected conflict error")
		}
	})
	t.Run("invalid target kind", func(t *testing.T) {
		_, err := normalizeProfile(restfile.K8sProfile{
			Target:  "job:api",
			PortStr: "8080",
		})
		if err == nil {
			t.Fatalf("expected invalid target kind error")
		}
	})
	t.Run("bad port", func(t *testing.T) {
		_, err := normalizeProfile(restfile.K8sProfile{Pod: "api", PortStr: "bad port"})
		if err == nil {
			t.Fatalf("expected bad port error")
		}
	})
	t.Run("bad named port token", func(t *testing.T) {
		_, err := normalizeProfile(restfile.K8sProfile{Pod: "api", PortStr: "!!!"})
		if err == nil {
			t.Fatalf("expected bad named port error")
		}
	})
	t.Run("bad partial template port token", func(t *testing.T) {
		_, err := normalizeProfile(restfile.K8sProfile{Pod: "api", PortStr: "{{port_name"})
		if err == nil {
			t.Fatalf("expected bad partial template port error")
		}
	})
	t.Run("bad local port", func(t *testing.T) {
		_, err := normalizeProfile(
			restfile.K8sProfile{Pod: "api", PortStr: "8080", LocalPortStr: "0"},
		)
		if err == nil {
			t.Fatalf("expected bad local port error")
		}
	})
	t.Run("bad pod wait", func(t *testing.T) {
		_, err := normalizeProfile(
			restfile.K8sProfile{Pod: "api", PortStr: "8080", PodWaitStr: "bad"},
		)
		if err == nil {
			t.Fatalf("expected bad pod wait error")
		}
	})
	t.Run("bad retries", func(t *testing.T) {
		_, err := normalizeProfile(
			restfile.K8sProfile{Pod: "api", PortStr: "8080", RetriesStr: "-1"},
		)
		if err == nil {
			t.Fatalf("expected bad retries error")
		}
	})
}
