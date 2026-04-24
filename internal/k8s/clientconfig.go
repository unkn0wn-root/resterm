package k8s

import (
	"fmt"
	"slices"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const defaultExecStdinMsg = "resterm disables stdin for kube exec credential plugins"

type ExecPolicy string

const (
	ExecPolicyAllowAll  ExecPolicy = "allow-all"
	ExecPolicyDenyAll   ExecPolicy = "deny-all"
	ExecPolicyAllowlist ExecPolicy = "allowlist"
)

type LoadOptions struct {
	ExecPolicy             ExecPolicy
	ExecAllowlist          []string
	StdinUnavailable       bool
	StdinUnavailableSet    bool
	StdinUnavailableReason string
}

type loadSettings struct {
	policy       ExecPolicy
	allowlist    []string
	stdinUnavail bool
	stdinMsg     string
}

func parseExecPolicy(raw string) (ExecPolicy, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	v = strings.ReplaceAll(v, "_", "-")
	switch v {
	case "", string(ExecPolicyAllowAll):
		return ExecPolicyAllowAll, nil
	case string(ExecPolicyDenyAll):
		return ExecPolicyDenyAll, nil
	case "allow-list", string(ExecPolicyAllowlist):
		return ExecPolicyAllowlist, nil
	default:
		return "", fmt.Errorf("k8s: invalid exec policy %q", raw)
	}
}

func rawConfig(cfg Config, opt LoadOptions) (clientcmdapi.Config, error) {
	raw, _, err := loadRaw(cfg)
	if err != nil {
		return clientcmdapi.Config{}, err
	}

	st, err := normalizeLoadOptions(opt)
	if err != nil {
		return clientcmdapi.Config{}, err
	}
	applyExecPolicy(&raw, st)
	return raw, nil
}

func clientConfig(cfg Config, opt LoadOptions) (clientcmd.ClientConfig, error) {
	raw, ovs, err := loadRaw(cfg)
	if err != nil {
		return nil, err
	}

	st, err := normalizeLoadOptions(opt)
	if err != nil {
		return nil, err
	}
	applyExecPolicy(&raw, st)

	return clientcmd.NewNonInteractiveClientConfig(raw, ovs.CurrentContext, ovs, nil), nil
}

func restConfig(cfg Config, opt LoadOptions) (*rest.Config, error) {
	cc, err := clientConfig(cfg, opt)
	if err != nil {
		return nil, err
	}

	out, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("k8s: build kube client config: %w", err)
	}
	return out, nil
}

func loadRaw(cfg Config) (clientcmdapi.Config, *clientcmd.ConfigOverrides, error) {
	cfg = cfg.normalize()

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		rules.ExplicitPath = cfg.Kubeconfig
	}

	ovs := &clientcmd.ConfigOverrides{}
	if cfg.Context != "" {
		ovs.CurrentContext = cfg.Context
	}
	if cfg.Namespace != "" {
		ovs.Context.Namespace = cfg.Namespace
	}

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, ovs)
	raw, err := cc.RawConfig()
	if err != nil {
		return clientcmdapi.Config{}, nil, fmt.Errorf("k8s: load kubeconfig: %w", err)
	}
	return raw, ovs, nil
}

func normalizeLoadOptions(opt LoadOptions) (loadSettings, error) {
	pol := opt.ExecPolicy
	if pol == "" {
		pol = ExecPolicyAllowAll
	}
	policy, err := parseExecPolicy(string(pol))
	if err != nil {
		return loadSettings{}, err
	}

	al := normalizeAllowlist(opt.ExecAllowlist)
	if len(al) > 0 && policy != ExecPolicyAllowlist {
		return loadSettings{}, fmt.Errorf("k8s: exec allowlist requires policy allowlist")
	}
	if policy == ExecPolicyAllowlist && len(al) == 0 {
		return loadSettings{}, fmt.Errorf(
			"k8s: exec allowlist policy requires at least one allowlist entry",
		)
	}

	noIn := true
	if opt.StdinUnavailableSet {
		noIn = opt.StdinUnavailable
	}
	msg := strings.TrimSpace(opt.StdinUnavailableReason)
	if noIn && msg == "" {
		msg = defaultExecStdinMsg
	}

	return loadSettings{
		policy:       policy,
		allowlist:    al,
		stdinUnavail: noIn,
		stdinMsg:     msg,
	}, nil
}

func loadOptionsFromSettings(st loadSettings) LoadOptions {
	return LoadOptions{
		ExecPolicy:             st.policy,
		ExecAllowlist:          append([]string(nil), st.allowlist...),
		StdinUnavailable:       st.stdinUnavail,
		StdinUnavailableSet:    true,
		StdinUnavailableReason: st.stdinMsg,
	}
}

func normalizeAllowlist(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	var out []string
	for _, item := range raw {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		k := strings.ToLower(v)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, func(a, b string) int {
		la := strings.ToLower(a)
		lb := strings.ToLower(b)
		if la == lb {
			return strings.Compare(a, b)
		}
		return strings.Compare(la, lb)
	})
	return out
}

func applyExecPolicy(raw *clientcmdapi.Config, st loadSettings) {
	if raw == nil {
		return
	}
	pol := policyFor(st)

	for name, auth := range raw.AuthInfos {
		if auth == nil || auth.Exec == nil {
			continue
		}
		ex := auth.Exec
		ex.PluginPolicy = pol
		ex.StdinUnavailable = st.stdinUnavail
		if st.stdinMsg != "" {
			ex.StdinUnavailableMessage = st.stdinMsg
		}
		raw.AuthInfos[name].Exec = ex
	}
}

func policyFor(st loadSettings) clientcmdapi.PluginPolicy {
	out := clientcmdapi.PluginPolicy{
		PolicyType: clientcmdapi.PluginPolicyAllowAll,
	}
	switch st.policy {
	case ExecPolicyDenyAll:
		out.PolicyType = clientcmdapi.PluginPolicyDenyAll
	case ExecPolicyAllowlist:
		out.PolicyType = clientcmdapi.PluginPolicyAllowlist
		out.Allowlist = make([]clientcmdapi.AllowlistEntry, 0, len(st.allowlist))
		for _, name := range st.allowlist {
			out.Allowlist = append(out.Allowlist, clientcmdapi.AllowlistEntry{Name: name})
		}
	}
	return out
}
