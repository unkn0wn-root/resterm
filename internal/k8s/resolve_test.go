package k8s

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestResolveUseMissingWithInline(t *testing.T) {
	spec := &restfile.K8sSpec{
		Use: "missing",
		Inline: &restfile.K8sProfile{
			Pod:     "api",
			PortStr: "8080",
		},
	}

	if _, err := Resolve(spec, nil, nil, nil, ""); err == nil {
		t.Fatalf("expected error for missing profile")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveUseInvalidProfileReportsDefinitionError(t *testing.T) {
	spec := &restfile.K8sSpec{Use: "cluster-api"}
	globalProfiles := []restfile.K8sProfile{{
		Scope:   restfile.K8sScopeGlobal,
		Name:    "cluster-api",
		Line:    1,
		Invalid: true,
		Error:   "@k8s global scope requires target and port",
	}}

	_, err := Resolve(spec, nil, globalProfiles, nil, "")
	if err == nil {
		t.Fatal("expected error for invalid profile")
	}
	msg := err.Error()
	for _, want := range []string{
		`profile "cluster-api" is invalid`,
		"line 1",
		"@k8s global scope requires target and port",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in error %q", want, msg)
		}
	}
}

func TestResolveValidationErrorOmitsPackagePrefix(t *testing.T) {
	spec := &restfile.K8sSpec{
		Inline: &restfile.K8sProfile{Pod: "api"},
	}

	_, err := Resolve(spec, nil, nil, nil, "")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got, want := err.Error(), "port is required"; got != want {
		t.Fatalf("unexpected error: got %q want %q", got, want)
	}
}

func TestResolveUseWithInlineOverrides(t *testing.T) {
	spec := &restfile.K8sSpec{
		Use: "api",
		Inline: &restfile.K8sProfile{
			Pod:     "api-override",
			PortStr: "9090",
		},
	}
	fileProfiles := []restfile.K8sProfile{
		{
			Scope:     restfile.K8sScopeFile,
			Name:      "api",
			Namespace: "dev",
			Pod:       "api-0",
			Port:      8080,
			PortStr:   "8080",
		},
	}

	cfg, err := Resolve(spec, fileProfiles, nil, nil, "dev")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Target.Kind != TargetPod || cfg.Target.Name != "api-override" {
		t.Fatalf("expected inline pod override, got %s/%s", cfg.Target.Kind, cfg.Target.Name)
	}
	if cfg.Port.Number != 9090 {
		t.Fatalf("expected inline port override, got %d", cfg.Port.Number)
	}
}

func TestResolveExpandsEnvAndTemplates(t *testing.T) {
	t.Setenv("K8S_NS", "prod")
	spec := &restfile.K8sSpec{
		Inline: &restfile.K8sProfile{
			Namespace: "env:K8S_NS",
			Target:    "pod:{{pod_name}}",
			PortStr:   "8080",
		},
	}
	res := vars.NewResolver(vars.NewMapProvider("x", map[string]string{"pod_name": "api-x"}))

	cfg, err := Resolve(spec, nil, nil, res, "")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Namespace != "prod" {
		t.Fatalf("expected namespace prod, got %q", cfg.Namespace)
	}
	if cfg.Target.Kind != TargetPod || cfg.Target.Name != "api-x" {
		t.Fatalf("expected pod target api-x, got %s/%s", cfg.Target.Kind, cfg.Target.Name)
	}
}

func TestResolveTrimsExpandedWhitespace(t *testing.T) {
	t.Setenv("K8S_POD", " api-x ")
	t.Setenv("K8S_PORT", " 8080 ")

	spec := &restfile.K8sSpec{
		Inline: &restfile.K8sProfile{
			Namespace: " default ",
			Pod:       "env:K8S_POD",
			PortStr:   "env:K8S_PORT",
		},
	}
	cfg, err := Resolve(spec, nil, nil, nil, "")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Namespace != "default" {
		t.Fatalf("expected namespace default, got %q", cfg.Namespace)
	}
	if cfg.Target.Kind != TargetPod || cfg.Target.Name != "api-x" {
		t.Fatalf("unexpected target %s/%s", cfg.Target.Kind, cfg.Target.Name)
	}
	if cfg.Port.Number != 8080 || cfg.Port.Name != "" {
		t.Fatalf("unexpected port parse: %+v", cfg.Port)
	}
}

func TestResolvePrefersFileScopeProfile(t *testing.T) {
	spec := &restfile.K8sSpec{Use: "api"}
	fileProfiles := []restfile.K8sProfile{
		{
			Scope:   restfile.K8sScopeFile,
			Name:    "api",
			Pod:     "file-pod",
			Port:    8080,
			PortStr: "8080",
		},
	}
	globalProfiles := []restfile.K8sProfile{
		{
			Scope:   restfile.K8sScopeGlobal,
			Name:    "api",
			Pod:     "global-pod",
			Port:    8080,
			PortStr: "8080",
		},
	}

	cfg, err := Resolve(spec, fileProfiles, globalProfiles, nil, "")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Target.Kind != TargetPod || cfg.Target.Name != "file-pod" {
		t.Fatalf("expected file scoped pod, got %s/%s", cfg.Target.Kind, cfg.Target.Name)
	}
}

func TestResolveInlinePodClearsBaseNonPodTarget(t *testing.T) {
	spec := &restfile.K8sSpec{
		Use: "api",
		Inline: &restfile.K8sProfile{
			Pod:     "api-0",
			PortStr: "8081",
		},
	}
	fileProfiles := []restfile.K8sProfile{
		{
			Scope:   restfile.K8sScopeFile,
			Name:    "api",
			Target:  "service:api",
			PortStr: "http",
		},
	}
	cfg, err := Resolve(spec, fileProfiles, nil, nil, "")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Target.Kind != TargetPod || cfg.Target.Name != "api-0" {
		t.Fatalf("expected pod target override, got %s/%s", cfg.Target.Kind, cfg.Target.Name)
	}
	if cfg.Port.Number != 8081 {
		t.Fatalf("expected numeric port override, got %d", cfg.Port.Number)
	}
}

func TestResolveInlineTargetPodOverridesBasePod(t *testing.T) {
	spec := &restfile.K8sSpec{
		Use: "api",
		Inline: &restfile.K8sProfile{
			Target:  "pod:api-1",
			PortStr: "9090",
		},
	}
	fileProfiles := []restfile.K8sProfile{
		{
			Scope:   restfile.K8sScopeFile,
			Name:    "api",
			Pod:     "api-0",
			PortStr: "8080",
		},
	}
	cfg, err := Resolve(spec, fileProfiles, nil, nil, "")
	if err != nil {
		t.Fatalf("resolve err: %v", err)
	}
	if cfg.Target.Kind != TargetPod || cfg.Target.Name != "api-1" {
		t.Fatalf(
			"expected pod target api-1, got %s/%s",
			cfg.Target.Kind,
			cfg.Target.Name,
		)
	}
	if cfg.Port.Number != 9090 {
		t.Fatalf("expected numeric port override, got %d", cfg.Port.Number)
	}
}
