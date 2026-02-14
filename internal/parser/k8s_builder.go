package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/duration"
	k8starget "github.com/unkn0wn-root/resterm/internal/k8s/target"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type k8sDirective struct {
	scope   restfile.K8sScope
	profile restfile.K8sProfile
	spec    *restfile.K8sSpec
}

func (b *documentBuilder) handleK8s(line int, rest string) {
	res, err := parseK8sDirective(rest)
	if err != nil {
		b.addError(line, err.Error())
		return
	}

	if res.scope == restfile.K8sScopeRequest {
		b.ensureRequest(line)
		if b.request.ssh != nil {
			b.addError(line, "@k8s cannot be combined with @ssh on the same request")
			return
		}
		if b.request.k8s != nil {
			b.addError(line, "@k8s already defined for this request")
			return
		}
		b.request.k8s = res.spec
		return
	}

	if res.scope == restfile.K8sScopeGlobal || res.scope == restfile.K8sScopeFile {
		res.profile.Scope = res.scope
		b.k8sDefs = append(b.k8sDefs, res.profile)
	}
}

func parseK8sDirective(rest string) (k8sDirective, error) {
	res := k8sDirective{}
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return res, fmt.Errorf("@k8s requires options")
	}

	fields := tokenizeOptionTokens(trimmed)
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
		name = strings.TrimSpace(fields[idx])
		idx++
	}
	if name == "" {
		name = "default"
	}

	opts := parseOptionTokens(strings.Join(fields[idx:], " "))
	prof := restfile.K8sProfile{Scope: scope, Name: name}
	if err := applyK8sOptions(&prof, opts); err != nil {
		return res, err
	}

	if scope == restfile.K8sScopeRequest {
		// Request-scoped persist is ignored to avoid leaking forwarders.
		prof.Persist = restfile.Opt[bool]{}
	} else {
		if strings.TrimSpace(prof.Namespace) == "" {
			prof.Namespace = k8starget.DefaultNamespace
		}
		if err := requireK8sTarget(prof); err != nil {
			return res, fmt.Errorf("@k8s %s scope %w", k8sScopeLabel(scope), err)
		}
		res.scope = scope
		res.profile = prof
		return res, nil
	}

	use := strings.TrimSpace(opts["use"])
	if use == "" {
		if err := requireK8sTarget(prof); err != nil {
			return res, fmt.Errorf("@k8s requires target and port or use=")
		}
		if strings.TrimSpace(prof.Namespace) == "" {
			prof.Namespace = k8starget.DefaultNamespace
		}
	}

	inline := buildInlineK8s(prof)
	res.scope = scope
	res.profile = prof
	res.spec = &restfile.K8sSpec{Use: use, Inline: inline}
	return res, nil
}

func parseK8sScope(token string) (restfile.K8sScope, bool) {
	return parseDirectiveScope(
		token,
		restfile.K8sScopeRequest,
		restfile.K8sScopeFile,
		restfile.K8sScopeGlobal,
	)
}

func applyK8sOptions(prof *restfile.K8sProfile, opts map[string]string) error {
	if ns, ok := firstOpt(opts, "namespace", "ns"); ok {
		prof.Namespace = ns
	}

	if raw, ok := firstOpt(opts, "target"); ok {
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
		v, ok := firstOpt(opts, ta.keys...)
		if !ok {
			continue
		}
		if err := setK8sTarget(prof, ta.kind, v); err != nil {
			return err
		}
	}

	if port, ok := firstOpt(opts, "port"); ok {
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

	if v, ok := firstOpt(opts, "context", "kube_context", "kube-context"); ok {
		prof.Context = v
	}

	if v, ok := firstOpt(opts, "kubeconfig", "config"); ok {
		prof.Kubeconfig = v
	}

	if v, ok := firstOpt(opts, "container"); ok {
		prof.Container = v
	}

	if v, ok := firstOpt(opts, "address", "bind"); ok {
		prof.Address = v
	}

	if key, raw, ok := firstOptWithKey(opts, "local_port", "local-port", "localport"); ok {
		prof.LocalPortStr = raw
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 65535 {
			return fmt.Errorf("invalid @k8s %s: %q", key, raw)
		}
		prof.LocalPort = n
	}

	setOptBool(&prof.Persist, opts, "persist")

	if key, raw, ok := firstOptWithKey(opts, "pod_running_timeout", "pod-running-timeout", "podwait"); ok {
		prof.PodWaitStr = raw
		prof.PodWait.Set = true
		d, ok := duration.Parse(raw)
		if !ok || d < 0 {
			return fmt.Errorf("invalid @k8s %s: %q", key, raw)
		}
		prof.PodWait.Val = d
	}

	if raw, ok := firstOpt(opts, "retries"); ok {
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
	if !hasK8sTarget(prof) || strings.TrimSpace(prof.PortStr) == "" {
		return fmt.Errorf("requires target and port")
	}
	return nil
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

func k8sScopeLabel(scope restfile.K8sScope) string {
	return directiveScopeLabel(
		scope,
		restfile.K8sScopeRequest,
		restfile.K8sScopeFile,
		restfile.K8sScopeGlobal,
	)
}
