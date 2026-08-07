package k8s

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
	k8starget "github.com/unkn0wn-root/resterm/internal/k8s/target"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type Directive struct {
	Scope          directive.Scope
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
	head, ok, err := directive.ParseProfileHeader(directive.K8s, rest)
	if !ok {
		return res, fmt.Errorf("@k8s requires options")
	}
	if err != nil {
		return res, err
	}
	scope, opts := head.Scope, head.Options
	name := head.Name
	if name == "" {
		name = connprofile.DefaultName
	}

	prof := restfile.K8sProfile{Scope: scope, Name: name}
	profileErr := func(err error) error {
		if err == nil {
			return nil
		}
		if scope != directive.ScopeGlobal && scope != directive.ScopeFile {
			return err
		}
		prof.Scope = scope
		return &DirectiveError{err: err, Profile: prof}
	}
	if err := applyK8sOptions(&prof, opts); err != nil {
		return res, profileErr(err)
	}
	use := opts.Pop("use")
	left := errors.Join(opts.Unknown(directive.K8s), opts.Conflicts(directive.K8s))

	if scope == directive.ScopeRequest {
		// Request-scoped persist is ignored to avoid leaking forwarders.
		res.PersistIgnored = prof.Persist.Set
		prof.Persist = restfile.Opt[bool]{}
	} else {
		if str.Trim(prof.Namespace) == "" {
			prof.Namespace = k8starget.DefaultNamespace
		}
		if err := requireK8sTarget(prof); err != nil {
			err := fmt.Errorf("@k8s %s scope %w", scope.String(), err)
			return res, profileErr(err)
		}
		res.Scope = scope
		res.Profile = prof
		return res, left
	}

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
	return res, left
}

func applyK8sOptions(prof *restfile.K8sProfile, opts directive.Options) error {
	if ns, ok := opts.PopAny("namespace", "ns"); ok {
		prof.Namespace = ns
	}

	if raw, ok := opts.PopAny("target"); ok {
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
			v := opts.Pop(key)
			if v == "" {
				continue
			}
			if err := setK8sTarget(prof, ta.kind, v); err != nil {
				return err
			}
		}
	}

	if port, ok := opts.PopAny("port"); ok {
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

	if v, ok := opts.PopAny("context", "kube_context", "kube-context"); ok {
		prof.Context = v
	}

	if v, ok := opts.PopAny("kubeconfig", "config"); ok {
		prof.Kubeconfig = v
	}

	if v, ok := opts.PopAny("container"); ok {
		prof.Container = v
	}

	if v, ok := opts.PopAny("address", "bind"); ok {
		prof.Address = v
	}

	if key, raw, ok := opts.PopKey("local_port", "local-port", "localport"); ok {
		prof.LocalPortStr = raw
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 65535 {
			return fmt.Errorf("invalid @k8s %s: %q", key, raw)
		}
		prof.LocalPort = n
	}

	if value, ok, bad := opts.PopBool("persist"); ok {
		if bad != "" {
			return fmt.Errorf("invalid @k8s persist: %q", bad)
		}
		prof.Persist.Set = true
		prof.Persist.Val = value
	}

	if key, raw, ok := opts.PopKey(
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

	if raw, ok := opts.PopAny("retries"); ok {
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
	cp.Scope = directive.ScopeRequest
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
