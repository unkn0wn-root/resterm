package k8s

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
	k8starget "github.com/unkn0wn-root/resterm/internal/k8s/target"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Directive struct {
	Scope          directive.Scope
	Profile        restfile.K8sProfile
	Spec           *restfile.K8sSpec
	PersistIgnored bool
}

func ParseDirective(rest string) (res Directive, err error) {
	head, ok, headErr := directive.ParseProfileHeader(directive.K8s, rest)
	if !ok {
		return res, fmt.Errorf("@k8s requires options")
	}
	defer func() { err = errors.Join(headErr, err) }()
	scope, opts := head.Scope, head.Options
	name := head.Name
	if name == "" {
		name = connprofile.DefaultName
	}

	prof := restfile.K8sProfile{Scope: scope, Name: name}
	optErr := applyK8sOptions(&prof, opts)
	use := opts.Pop("use")
	// Everything valid has been popped by now, so Leftover reports only mistakes.
	left := opts.Leftover(directive.K8s)
	if optErr != nil {
		// The caller keeps a shared profile with an error as invalid.
		res.Profile = prof
		return res, errors.Join(optErr, left)
	}
	target := hasK8sTarget(prof) || opts.Bare("target")
	for _, ta := range k8sTargetKinds {
		target = target || slices.ContainsFunc(ta.keys, opts.Bare)
	}
	met := target && (strings.TrimSpace(prof.PortStr) != "" || opts.Bare("port"))

	if scope == directive.ScopeRequest {
		// Request-scoped persist is ignored to avoid leaking forwarders.
		res.PersistIgnored = prof.Persist.Set
		prof.Persist = restfile.Opt[bool]{}
	} else {
		if strings.TrimSpace(prof.Namespace) == "" {
			prof.Namespace = k8starget.DefaultNamespace
		}
		res.Scope = scope
		res.Profile = prof
		if !met {
			return res, errors.Join(fmt.Errorf("@k8s %s scope requires target and port", scope.String()), left)
		}
		return res, left
	}

	if use == "" {
		if !met && !opts.Bare("use") {
			return res, errors.Join(errors.New("@k8s requires target and port or use="), left)
		}
		if strings.TrimSpace(prof.Namespace) == "" {
			prof.Namespace = k8starget.DefaultNamespace
		}
	}

	inline := buildInlineK8s(prof)
	res.Scope = scope
	res.Profile = prof
	res.Spec = &restfile.K8sSpec{Use: use, Inline: inline}
	return res, left
}

var k8sTargetKinds = []struct {
	kind k8starget.Kind
	keys []string
}{
	{kind: k8starget.Pod, keys: []string{"pod"}},
	{kind: k8starget.Service, keys: []string{"service", "svc"}},
	{kind: k8starget.Deployment, keys: []string{"deployment", "deploy"}},
	{kind: k8starget.StatefulSet, keys: []string{"statefulset", "sts"}},
}

// Every option is read even after one fails, so two mistakes are both reported.
func applyK8sOptions(prof *restfile.K8sProfile, opts directive.Options) error {
	if ns, ok := opts.PopAny("namespace", "ns"); ok {
		prof.Namespace = ns
	}

	var targetErr error
	if raw, ok := opts.PopAny("target"); ok {
		k, n, err := k8starget.ParseRef(raw)
		if err != nil {
			targetErr = fmt.Errorf("invalid @k8s target: %w", err)
		} else {
			targetErr = setK8sTarget(prof, k, n)
		}
	}

	for _, ta := range k8sTargetKinds {
		if name, ok := opts.PopAny(ta.keys...); ok && targetErr == nil {
			targetErr = setK8sTarget(prof, ta.kind, name)
		}
	}
	errs := []error{targetErr}

	if port, ok := opts.PopAny("port"); ok {
		prof.PortStr = port
		n, err := strconv.Atoi(port)
		if err == nil {
			if n <= 0 || n > 65535 {
				errs = append(errs, fmt.Errorf("invalid @k8s port: %q", port))
			}
			prof.Port = n
		} else if !k8starget.IsValidPortName(port) {
			errs = append(errs, fmt.Errorf("invalid @k8s port: %q", port))
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
			errs = append(errs, fmt.Errorf("invalid @k8s %s: %q", key, raw))
		}
		prof.LocalPort = n
	}

	if value, ok, bad := opts.PopBool("persist"); ok {
		if bad != "" {
			errs = append(errs, fmt.Errorf("invalid @k8s persist: %q", bad))
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
			errs = append(errs, fmt.Errorf("invalid @k8s %s: %q", key, raw))
		}
		prof.PodWait.Val = d
	}

	if raw, ok := opts.PopAny("retries"); ok {
		prof.RetriesStr = raw
		prof.Retries.Set = true
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			errs = append(errs, fmt.Errorf("invalid @k8s retries: %q", raw))
		}
		prof.Retries.Val = n
	}

	return errors.Join(errs...)
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

func hasK8sTarget(prof restfile.K8sProfile) bool {
	return strings.TrimSpace(prof.Pod) != "" || strings.TrimSpace(prof.Target) != ""
}

func setK8sTarget(prof *restfile.K8sProfile, kind k8starget.Kind, name string) error {
	k := k8starget.ParseKind(string(kind))
	n := strings.TrimSpace(name)
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
	if raw := strings.TrimSpace(prof.Target); raw != "" {
		k, n, err := k8starget.ParseRef(raw)
		if err == nil {
			return k, n
		}
	}
	if p := strings.TrimSpace(prof.Pod); p != "" {
		return k8starget.Pod, p
	}
	return "", ""
}
