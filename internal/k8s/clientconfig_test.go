package k8s

import (
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestParseExecPolicy(t *testing.T) {
	cases := map[string]ExecPolicy{
		"":           ExecPolicyAllowAll,
		"allow-all":  ExecPolicyAllowAll,
		"deny-all":   ExecPolicyDenyAll,
		"allowlist":  ExecPolicyAllowlist,
		"allow-list": ExecPolicyAllowlist,
		"ALLOW_LIST": ExecPolicyAllowlist,
	}
	for raw, want := range cases {
		got, err := ParseExecPolicy(raw)
		if err != nil {
			t.Fatalf("parse %q err: %v", raw, err)
		}
		if got != want {
			t.Fatalf("parse %q expected %q, got %q", raw, want, got)
		}
	}
	if _, err := ParseExecPolicy("bad"); err == nil {
		t.Fatalf("expected parse error for bad policy")
	}
}

func TestNormalizeLoadOpt(t *testing.T) {
	cf, err := normalizeLoadOpt(LoadOpt{})
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}
	if cf.policy != ExecPolicyAllowAll {
		t.Fatalf("expected allow-all default, got %q", cf.policy)
	}
	if !cf.stdinUnavail {
		t.Fatalf("expected stdin unavailable default true")
	}
	if cf.stdinMsg == "" {
		t.Fatalf("expected default stdin unavailable message")
	}
}

func TestNormalizeLoadOptAllowlistValidation(t *testing.T) {
	if _, err := normalizeLoadOpt(LoadOpt{ExecPolicy: ExecPolicyAllowlist}); err == nil {
		t.Fatalf("expected allowlist policy validation error")
	}
	_, err := normalizeLoadOpt(LoadOpt{
		ExecPolicy:    ExecPolicyDenyAll,
		ExecAllowlist: []string{"aws"},
	})
	if err == nil {
		t.Fatalf("expected allowlist + non-allowlist policy validation error")
	}
}

func TestApplyExecPolicy(t *testing.T) {
	raw := clientcmdapi.Config{
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user": {
				Exec: &clientcmdapi.ExecConfig{
					Command: "aws",
				},
			},
		},
	}

	cf, err := normalizeLoadOpt(LoadOpt{
		ExecPolicy:    ExecPolicyAllowlist,
		ExecAllowlist: []string{"aws", "kubelogin", "aws"},
	})
	if err != nil {
		t.Fatalf("normalize err: %v", err)
	}
	applyExecPolicy(&raw, cf)

	ex := raw.AuthInfos["user"].Exec
	if ex == nil {
		t.Fatalf("expected exec config")
	}
	if ex.PluginPolicy.PolicyType != clientcmdapi.PluginPolicyAllowlist {
		t.Fatalf("expected allowlist policy, got %q", ex.PluginPolicy.PolicyType)
	}
	if len(ex.PluginPolicy.Allowlist) != 2 {
		t.Fatalf("expected 2 allowlist entries, got %d", len(ex.PluginPolicy.Allowlist))
	}
	if !ex.StdinUnavailable {
		t.Fatalf("expected stdin unavailable true")
	}
	if strings.TrimSpace(ex.StdinUnavailableMessage) == "" {
		t.Fatalf("expected stdin unavailable message")
	}
}

func TestNormalizeAllowlistDedupAndSortCaseInsensitive(t *testing.T) {
	got := normalizeAllowlist([]string{"kubelogin", "AWS", "aws", "Az", "az"})
	want := []string{"AWS", "Az", "kubelogin"}
	if len(got) != len(want) {
		t.Fatalf("unexpected allowlist length: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected allowlist order: got %v want %v", got, want)
		}
	}
}

func TestRawConfigAppliesContextOverrideAndExecPolicy(t *testing.T) {
	cfgData := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster": {Server: "https://127.0.0.1:6443"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user": {
				Exec: &clientcmdapi.ExecConfig{Command: "aws"},
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"ctx-a": {Cluster: "cluster", AuthInfo: "user", Namespace: "a"},
			"ctx-b": {Cluster: "cluster", AuthInfo: "user", Namespace: "b"},
		},
		CurrentContext: "ctx-a",
	}
	path := t.TempDir() + "/config"
	if err := clientcmd.WriteToFile(cfgData, path); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	cfg := Cfg{
		Kubeconfig: path,
		Context:    "ctx-b",
		Namespace:  "ns-override",
	}
	raw, err := RawConfig(cfg, LoadOpt{
		ExecPolicy:    ExecPolicyAllowlist,
		ExecAllowlist: []string{"aws"},
	})
	if err != nil {
		t.Fatalf("raw config err: %v", err)
	}

	cc := clientcmd.NewNonInteractiveClientConfig(raw, "ctx-b", &clientcmd.ConfigOverrides{
		CurrentContext: "ctx-b",
		Context:        clientcmdapi.Context{Namespace: "ns-override"},
	}, nil)
	ns, _, err := cc.Namespace()
	if err != nil {
		t.Fatalf("namespace resolve err: %v", err)
	}
	if ns != "ns-override" {
		t.Fatalf("expected namespace override, got %q", ns)
	}

	ex := raw.AuthInfos["user"].Exec
	if ex.PluginPolicy.PolicyType != clientcmdapi.PluginPolicyAllowlist {
		t.Fatalf("expected allowlist policy, got %q", ex.PluginPolicy.PolicyType)
	}
}
