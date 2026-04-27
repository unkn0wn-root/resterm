package k8s

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/duration"
	k8starget "github.com/unkn0wn-root/resterm/internal/k8s/target"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/lex"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	dscope "github.com/unkn0wn-root/resterm/internal/parser/directive/scope"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type Directive struct {
	Scope          restfile.K8sScope
	Profile        restfile.K8sProfile
	Spec           *restfile.K8sSpec
	PersistIgnored bool
}

type DirectiveError struct {
	err     error
	Profile restfile.K8sProfile
}

func (e *DirectiveError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *DirectiveError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func ParseDirective(rest string) (Directive, error) {
	res := Directive{}
	trimmed := str.Trim(rest)
	if trimmed == "" {
		return res, fmt.Errorf("@k8s requires options")
	}

	fields := lex.TokenizeFieldsEscaped(trimmed)
	if len(fields) == 0 {
		return res, fmt.Errorf("@k8s requires options")
	}

	scope := restfile.K8sScopeRequest
	idx := 0
	if sc, ok := parseK8sScope(fields[idx]); ok {
		scope = sc
		idx++
	}

	name := "default"
	if idx < len(fields) && !strings.Contains(fields[idx], "=") {
		name = str.Trim(fields[idx])
		idx++
	}
	if name == "" {
		name = "default"
	}

	opts := options.Parse(strings.Join(fields[idx:], " "))
	prof := restfile.K8sProfile{Scope: scope, Name: name}
	profileErr := func(err error) error {
		if err == nil {
			return nil
		}
		if scope != restfile.K8sScopeGlobal && scope != restfile.K8sScopeFile {
			return err
		}
		prof.Scope = scope
		return &DirectiveError{err: err, Profile: prof}
	}
	if err := applyK8sOptions(&prof, opts); err != nil {
		return res, profileErr(err)
	}

	if scope == restfile.K8sScopeRequest {
		// Request-scoped persist is ignored to avoid leaking forwarders.
		res.PersistIgnored = prof.Persist.Set
		prof.Persist = restfile.Opt[bool]{}
	} else {
		if str.Trim(prof.Namespace) == "" {
			prof.Namespace = k8starget.DefaultNamespace
		}
		if err := requireK8sTarget(prof); err != nil {
			err := fmt.Errorf("@k8s %s scope %w", k8sScopeLabel(scope), err)
			return res, profileErr(err)
		}
		res.Scope = scope
		res.Profile = prof
		return res, nil
	}

	use := str.Trim(opts["use"])
	if use == "" {
		if err := requireK8sTarget(prof); err != nil {
			return res, fmt.Errorf("@k8s requires target and port or use=")
		}
		if str.Trim(prof.Namespace) == "" {
			prof.Namespace = k8starget.DefaultNamespace
		}
	}

	inline := buildInlineK8s(prof)
	res.Scope = scope
	res.Profile = prof
	res.Spec = &restfile.K8sSpec{Use: use, Inline: inline}
	return res, nil
}

func parseK8sScope(token string) (restfile.K8sScope, bool) {
	return dscope.Parse(
		token,
		restfile.K8sScopeRequest,
		restfile.K8sScopeFile,
		restfile.K8sScopeGlobal,
	)
}

func applyK8sOptions(prof *restfile.K8sProfile, opts map[string]string) error {
	if ns, ok := options.First(opts, "namespace", "ns"); ok {
		prof.Namespace = ns
	}

	if raw, ok := options.First(opts, "target"); ok {
		k, n, err := k8starget.ParseRef(raw)
		if err != nil {
			return fmt.Errorf("invalid @k8s target: %w", err)
		}
		if err := setK8sTarget(prof, k, n); err != nil {
			return err
		}
	}

	targetAliases := []struct {
		kind k8starget.Kind
		keys []string
	}{
		{kind: k8starget.Pod, keys: []string{"pod"}},
		{kind: k8starget.Service, keys: []string{"service", "svc"}},
		{kind: k8starget.Deployment, keys: []string{"deployment", "deploy"}},
		{kind: k8starget.StatefulSet, keys: []string{"statefulset", "sts"}},
	}
	for _, ta := range targetAliases {
		for _, key := range ta.keys {
			v := str.Trim(opts[key])
			if v == "" {
				continue
			}
			if err := setK8sTarget(prof, ta.kind, v); err != nil {
				return err
			}
		}
	}

	if port, ok := options.First(opts, "port"); ok {
		prof.PortStr = port
		n, err := strconv.Atoi(port)
		if err == nil {
			if n <= 0 || n > 65535 {
				return fmt.Errorf("invalid @k8s port: %q", port)
			}
			prof.Port = n
		} else if !k8starget.IsValidPortName(port) {
			return fmt.Errorf("invalid @k8s port: %q", port)
		}
	}

	if v, ok := options.First(opts, "context", "kube_context", "kube-context"); ok {
		prof.Context = v
	}

	if v, ok := options.First(opts, "kubeconfig", "config"); ok {
		prof.Kubeconfig = v
	}

	if v, ok := options.First(opts, "container"); ok {
		prof.Container = v
	}

	if v, ok := options.First(opts, "address", "bind"); ok {
		prof.Address = v
	}

	if key, raw, ok := options.FirstWithKey(opts, "local_port", "local-port", "localport"); ok {
		prof.LocalPortStr = raw
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 65535 {
			return fmt.Errorf("invalid @k8s %s: %q", key, raw)
		}
		prof.LocalPort = n
	}

	if value, ok := options.Bool(opts, "persist"); ok {
		prof.Persist.Set = true
		prof.Persist.Val = value
	}

	if key, raw, ok := options.FirstWithKey(
		opts,
		"pod_running_timeout",
		"pod-running-timeout",
		"podwait",
	); ok {
		prof.PodWaitStr = raw
		prof.PodWait.Set = true
		d, ok := duration.Parse(raw)
		if !ok || d < 0 {
			return fmt.Errorf("invalid @k8s %s: %q", key, raw)
		}
		prof.PodWait.Val = d
	}

	if raw, ok := options.First(opts, "retries"); ok {
		prof.RetriesStr = raw
		prof.Retries.Set = true
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid @k8s retries: %q", raw)
		}
		prof.Retries.Val = n
	}

	return nil
}

func buildInlineK8s(prof restfile.K8sProfile) *restfile.K8sProfile {
	if !k8sInlineSet(prof) {
		return nil
	}
	cp := prof
	cp.Scope = restfile.K8sScopeRequest
	return &cp
}

func k8sInlineSet(prof restfile.K8sProfile) bool {
	return prof.Namespace != "" ||
		prof.Target != "" ||
		prof.Pod != "" ||
		prof.PortStr != "" ||
		prof.Context != "" ||
		prof.Kubeconfig != "" ||
		prof.Container != "" ||
		prof.Address != "" ||
		prof.LocalPortStr != "" ||
		prof.Persist.Set ||
		prof.PodWait.Set ||
		prof.Retries.Set
}

func requireK8sTarget(prof restfile.K8sProfile) error {
	if !hasK8sTarget(prof) || str.Trim(prof.PortStr) == "" {
		return fmt.Errorf("requires target and port")
	}
	return nil
}

func hasK8sTarget(prof restfile.K8sProfile) bool {
	return str.Trim(prof.Pod) != "" || str.Trim(prof.Target) != ""
}

func setK8sTarget(prof *restfile.K8sProfile, kind k8starget.Kind, name string) error {
	k := k8starget.ParseKind(string(kind))
	n := str.Trim(name)
	if k == "" || n == "" {
		return fmt.Errorf("invalid @k8s target")
	}

	ck, cn := currentK8sTarget(*prof)
	if ck != "" && (ck != k || cn != n) {
		return fmt.Errorf("multiple @k8s targets specified")
	}

	prof.Target = k8starget.Format(k, n)
	if k == k8starget.Pod {
		prof.Pod = n
	} else {
		prof.Pod = ""
	}
	return nil
}

func currentK8sTarget(prof restfile.K8sProfile) (k8starget.Kind, string) {
	if raw := str.Trim(prof.Target); raw != "" {
		k, n, err := k8starget.ParseRef(raw)
		if err == nil {
			return k, n
		}
	}
	if p := str.Trim(prof.Pod); p != "" {
		return k8starget.Pod, p
	}
	return "", ""
}

func k8sScopeLabel(scope restfile.K8sScope) string {
	return dscope.Label(
		scope,
		restfile.K8sScopeRequest,
		restfile.K8sScopeFile,
		restfile.K8sScopeGlobal,
	)
}
