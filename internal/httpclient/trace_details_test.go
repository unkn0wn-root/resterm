package httpclient

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestFormatK8sUsesNormalizedResolveOutput(t *testing.T) {
	cfg, err := k8s.Resolve(
		&restfile.K8sSpec{
			Inline: &restfile.K8sProfile{
				Namespace: " default ",
				Target:    " svc : api ",
				PortStr:   " http ",
				Context:   " kind-dev ",
			},
		},
		nil,
		nil,
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("resolve k8s: %v", err)
	}

	got := formatK8s(&k8s.Plan{Manager: &k8s.Manager{}, Config: cfg})
	want := "kind-dev default/service/api:http"
	if got != want {
		t.Fatalf("unexpected k8s format: got %q want %q", got, want)
	}
}

func TestFormatK8sUsesNormalizedPodResolveOutput(t *testing.T) {
	cfg, err := k8s.Resolve(
		&restfile.K8sSpec{
			Inline: &restfile.K8sProfile{
				Namespace: " default ",
				Pod:       " api ",
				PortStr:   " 8080 ",
			},
		},
		nil,
		nil,
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("resolve k8s: %v", err)
	}

	got := formatK8s(&k8s.Plan{Manager: &k8s.Manager{}, Config: cfg})
	want := "default/api:8080"
	if got != want {
		t.Fatalf("unexpected k8s format: got %q want %q", got, want)
	}
}
