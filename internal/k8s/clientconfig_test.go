package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
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
		got, err := parseExecPolicy(raw)
		if err != nil {
			t.Fatalf("parse %q err: %v", raw, err)
		}
		if got != want {
			t.Fatalf("parse %q expected %q, got %q", raw, want, got)
		}
	}
	if _, err := parseExecPolicy("bad"); err == nil {
		t.Fatalf("expected parse error for bad policy")
	}
}

func TestNormalizeLoadOpt(t *testing.T) {
	cf, err := normalizeLoadOptions(LoadOptions{})
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
	if _, err := normalizeLoadOptions(LoadOptions{ExecPolicy: ExecPolicyAllowlist}); err == nil {
		t.Fatalf("expected allowlist policy validation error")
	}
	_, err := normalizeLoadOptions(LoadOptions{
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

	cf, err := normalizeLoadOptions(LoadOptions{
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

	cfg := Config{
		Kubeconfig: path,
		Context:    "ctx-b",
		Namespace:  "ns-override",
	}
	cc, err := clientConfig(cfg, LoadOptions{
		ExecPolicy:    ExecPolicyAllowlist,
		ExecAllowlist: []string{"aws"},
	})
	if err != nil {
		t.Fatalf("client config err: %v", err)
	}
	raw, err := cc.RawConfig()
	if err != nil {
		t.Fatalf("raw config err: %v", err)
	}

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

func newTestRESTAPI(t *testing.T, h http.Handler) *restAPI {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	api, err := newRESTAPI(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("newRESTAPI: %v", err)
	}
	return api
}

func writeJSON(t *testing.T, w http.ResponseWriter, code int, body any) {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(raw); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestRESTAPIRequestPathsAndDecoding(t *testing.T) {
	var got string
	api := newTestRESTAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
		switch {
		case strings.HasSuffix(r.URL.Path, "/pods/web"):
			writeJSON(t, w, 200, corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web"}})
		case strings.HasSuffix(r.URL.Path, "/pods"):
			got += "?" + r.URL.Query().Get("labelSelector")
			writeJSON(t, w, 200, corev1.PodList{
				Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "web-1"}}},
			})
		case strings.HasSuffix(r.URL.Path, "/services/api"):
			writeJSON(t, w, 200, corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api"}})
		case strings.HasSuffix(r.URL.Path, "/deployments/dep"):
			writeJSON(t, w, 200, appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "dep"}})
		case strings.HasSuffix(r.URL.Path, "/statefulsets/sts"):
			writeJSON(t, w, 200, appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "sts"}})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))

	ctx := context.Background()
	pod, err := api.getPod(ctx, "prod", "web")
	if err != nil || pod.Name != "web" {
		t.Fatalf("getPod = %v, %v", pod, err)
	}
	if want := "/api/v1/namespaces/prod/pods/web"; got != want {
		t.Errorf("pod path = %q, want %q", got, want)
	}

	pods, err := api.listPods(ctx, "prod", "app=web")
	if err != nil || len(pods) != 1 || pods[0].Name != "web-1" {
		t.Fatalf("listPods = %v, %v", pods, err)
	}
	if want := "/api/v1/namespaces/prod/pods?app=web"; got != want {
		t.Errorf("list path = %q, want %q", got, want)
	}

	if svc, err := api.getService(ctx, "prod", "api"); err != nil || svc.Name != "api" {
		t.Fatalf("getService = %v, %v", svc, err)
	}
	if want := "/api/v1/namespaces/prod/services/api"; got != want {
		t.Errorf("service path = %q, want %q", got, want)
	}

	if dep, err := api.getDeployment(ctx, "prod", "dep"); err != nil || dep.Name != "dep" {
		t.Fatalf("getDeployment = %v, %v", dep, err)
	}
	if want := "/apis/apps/v1/namespaces/prod/deployments/dep"; got != want {
		t.Errorf("deployment path = %q, want %q", got, want)
	}

	if sts, err := api.getStatefulSet(ctx, "prod", "sts"); err != nil || sts.Name != "sts" {
		t.Fatalf("getStatefulSet = %v, %v", sts, err)
	}
	if want := "/apis/apps/v1/namespaces/prod/statefulsets/sts"; got != want {
		t.Errorf("statefulset path = %q, want %q", got, want)
	}
}

// The resolver treats a missing target as "not found" rather than an error, so
// the trimmed scheme has to keep decoding API errors into a StatusError.
func TestRESTAPINotFoundStaysRecognisable(t *testing.T) {
	api := newTestRESTAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 404, metav1.Status{
			TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
			Status:   metav1.StatusFailure,
			Reason:   metav1.StatusReasonNotFound,
			Code:     404,
			Message:  `pods "gone" not found`,
		})
	}))

	ctx := context.Background()
	for name, call := range map[string]func() error{
		"pod":         func() error { _, err := api.getPod(ctx, "prod", "gone"); return err },
		"service":     func() error { _, err := api.getService(ctx, "prod", "gone"); return err },
		"deployment":  func() error { _, err := api.getDeployment(ctx, "prod", "gone"); return err },
		"statefulset": func() error { _, err := api.getStatefulSet(ctx, "prod", "gone"); return err },
	} {
		err := call()
		if err == nil {
			t.Fatalf("%s: expected an error", name)
		}
		if !apierrors.IsNotFound(err) {
			t.Errorf("%s: IsNotFound(%v) = false", name, err)
		}
	}
}

func TestRESTAPIPortForwardURL(t *testing.T) {
	api := newTestRESTAPI(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	got := api.portForwardURL("prod", "web").Path
	if want := "/api/v1/namespaces/prod/pods/web/portforward"; got != want {
		t.Errorf("portForwardURL = %q, want %q", got, want)
	}
}

func TestNewRESTAPIRejectsMissingConfig(t *testing.T) {
	if _, err := newRESTAPI(nil); err == nil {
		t.Fatal("expected an error for a nil config")
	}
}
