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

type LoadOpt struct {
	ExecPolicy    ExecPolicy
	ExecAllowlist []string
	// Keep explicit-value + explicit-set to preserve API stability and let callers
	// choose between default behavior and an explicit false.
	StdinUnavailable       bool
	StdinUnavailableSet    bool
	StdinUnavailableReason string
}

type loadCfg struct {
	policy       ExecPolicy
	allowlist    []string
	stdinUnavail bool
	stdinMsg     string
}

func ParseExecPolicy(raw string) (ExecPolicy, error) {
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

func RawConfig(cfg Cfg, opt LoadOpt) (clientcmdapi.Config, error) {
	raw, _, err := loadRaw(cfg)
	if err != nil {
		return clientcmdapi.Config{}, err
	}

	cf, err := normalizeLoadOpt(opt)
	if err != nil {
		return clientcmdapi.Config{}, err
	}
	applyExecPolicy(&raw, cf)
	return raw, nil
}

func ClientConfig(cfg Cfg, opt LoadOpt) (clientcmd.ClientConfig, error) {
	raw, ovs, err := loadRaw(cfg)
	if err != nil {
		return nil, err
	}

	cf, err := normalizeLoadOpt(opt)
	if err != nil {
		return nil, err
	}
	applyExecPolicy(&raw, cf)

	return clientcmd.NewNonInteractiveClientConfig(raw, ovs.CurrentContext, ovs, nil), nil
}

func RESTConfig(cfg Cfg, opt LoadOpt) (*rest.Config, error) {
	cc, err := ClientConfig(cfg, opt)
	if err != nil {
		return nil, err
	}

	out, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("k8s: build kube client config: %w", err)
	}
	return out, nil
}

func loadRaw(cfg Cfg) (clientcmdapi.Config, *clientcmd.ConfigOverrides, error) {
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

func normalizeLoadOpt(opt LoadOpt) (loadCfg, error) {
	pl := opt.ExecPolicy
	if pl == "" {
		pl = ExecPolicyAllowAll
	}
	pp, err := ParseExecPolicy(string(pl))
	if err != nil {
		return loadCfg{}, err
	}

	al := normalizeAllowlist(opt.ExecAllowlist)
	if len(al) > 0 && pp != ExecPolicyAllowlist {
		return loadCfg{}, fmt.Errorf("k8s: exec allowlist requires policy allowlist")
	}
	if pp == ExecPolicyAllowlist && len(al) == 0 {
		return loadCfg{}, fmt.Errorf("k8s: exec allowlist policy requires at least one allowlist entry")
	}

	noIn := true
	if opt.StdinUnavailableSet {
		noIn = opt.StdinUnavailable
	}
	msg := strings.TrimSpace(opt.StdinUnavailableReason)
	if noIn && msg == "" {
		msg = defaultExecStdinMsg
	}

	return loadCfg{
		policy:       pp,
		allowlist:    al,
		stdinUnavail: noIn,
		stdinMsg:     msg,
	}, nil
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

func applyExecPolicy(raw *clientcmdapi.Config, cf loadCfg) {
	if raw == nil {
		return
	}
	pol := policyFor(cf)

	for name, auth := range raw.AuthInfos {
		if auth == nil || auth.Exec == nil {
			continue
		}
		ex := auth.Exec
		ex.PluginPolicy = pol
		ex.StdinUnavailable = cf.stdinUnavail
		if cf.stdinMsg != "" {
			ex.StdinUnavailableMessage = cf.stdinMsg
		}
		raw.AuthInfos[name].Exec = ex
	}
}

func policyFor(cf loadCfg) clientcmdapi.PluginPolicy {
	out := clientcmdapi.PluginPolicy{
		PolicyType: clientcmdapi.PluginPolicyAllowAll,
	}
	switch cf.policy {
	case ExecPolicyDenyAll:
		out.PolicyType = clientcmdapi.PluginPolicyDenyAll
	case ExecPolicyAllowlist:
		out.PolicyType = clientcmdapi.PluginPolicyAllowlist
		out.Allowlist = make([]clientcmdapi.AllowlistEntry, 0, len(cf.allowlist))
		for _, name := range cf.allowlist {
			out.Allowlist = append(out.Allowlist, clientcmdapi.AllowlistEntry{Name: name})
		}
	}
	return out
}
