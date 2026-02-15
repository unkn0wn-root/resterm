package k8s

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	k8starget "github.com/unkn0wn-root/resterm/internal/k8s/target"
	"github.com/unkn0wn-root/resterm/internal/profileutil"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const (
	defaultNamespace = k8starget.DefaultNamespace
	defaultAddress   = "127.0.0.1"
	defaultPodWait   = time.Minute
	defaultTTL       = 10 * time.Minute
)

type TargetKind = k8starget.Kind

const (
	targetKindPod         TargetKind = k8starget.Pod
	targetKindService     TargetKind = k8starget.Service
	targetKindDeployment  TargetKind = k8starget.Deployment
	targetKindStatefulSet TargetKind = k8starget.StatefulSet
)

type Cfg struct {
	Name       string
	Namespace  string
	TargetKind TargetKind
	TargetName string
	Pod        string
	Port       int
	PortName   string
	Context    string
	Kubeconfig string
	Container  string
	Address    string
	LocalPort  int
	Persist    bool
	PodWait    time.Duration
	Retries    int

	PortRaw      string
	LocalPortRaw string
	PodWaitRaw   string
	RetriesRaw   string
	Label        string
}

func NormalizeProfile(p restfile.K8sProfile) (Cfg, error) {
	cfg := baseCfg(p)
	cfg.Name = profileutil.Fallback(cfg.Name, "default")
	if err := parseTarget(&cfg, p); err != nil {
		return Cfg{}, err
	}

	if err := parseCfg(&cfg, p); err != nil {
		return Cfg{}, err
	}
	if cfg.Port == 0 && cfg.PortName == "" {
		return Cfg{}, errors.New("k8s: port is required")
	}

	if cfg.Kubeconfig != "" {
		path, err := profileutil.ExpandPath(
			cfg.Kubeconfig,
			"cannot resolve home directory for kubeconfig path",
		)
		if err != nil {
			return Cfg{}, err
		}
		cfg.Kubeconfig = path
	}

	return normalizeCfg(cfg), nil
}

func baseCfg(p restfile.K8sProfile) Cfg {
	return Cfg{
		Name:      strings.TrimSpace(p.Name),
		Namespace: profileutil.Fallback(strings.TrimSpace(p.Namespace), defaultNamespace),
		Pod:       strings.TrimSpace(p.Pod),
		// Keep numeric Port as a fallback for programmatic callers that set only Port.
		Port:         p.Port,
		Context:      strings.TrimSpace(p.Context),
		Kubeconfig:   strings.TrimSpace(p.Kubeconfig),
		Container:    strings.TrimSpace(p.Container),
		Address:      profileutil.Fallback(strings.TrimSpace(p.Address), defaultAddress),
		LocalPort:    p.LocalPort,
		Persist:      p.Persist.Set && p.Persist.Val,
		PodWait:      defaultPodWait,
		Retries:      0,
		PortRaw:      strings.TrimSpace(p.PortStr),
		LocalPortRaw: strings.TrimSpace(p.LocalPortStr),
		PodWaitRaw:   strings.TrimSpace(p.PodWaitStr),
		RetriesRaw:   strings.TrimSpace(p.RetriesStr),
	}
}

func trimStrings(fields ...*string) {
	for _, field := range fields {
		*field = strings.TrimSpace(*field)
	}
}

func normalizeCfg(cfg Cfg) Cfg {
	trimStrings(
		&cfg.Name,
		&cfg.Namespace,
		&cfg.TargetName,
		&cfg.Pod,
		&cfg.PortName,
		&cfg.Context,
		&cfg.Kubeconfig,
		&cfg.Container,
		&cfg.Address,
		&cfg.PortRaw,
		&cfg.LocalPortRaw,
		&cfg.PodWaitRaw,
		&cfg.RetriesRaw,
		&cfg.Label,
	)
	cfg.TargetKind = normalizeTargetKind(cfg.TargetKind)
	return cfg
}

func parseCfg(cfg *Cfg, p restfile.K8sProfile) error {
	if err := parsePortRef(cfg, p); err != nil {
		return err
	}
	if err := profileutil.ParsePort(
		"k8s local",
		&cfg.LocalPort,
		&cfg.LocalPortRaw,
		p.LocalPortStr,
	); err != nil {
		return err
	}
	if err := profileutil.ParseDuration(
		"k8s pod wait",
		&cfg.PodWait,
		&cfg.PodWaitRaw,
		p.PodWaitStr,
	); err != nil {
		return err
	}
	if err := profileutil.ParseRetries(
		"k8s",
		&cfg.Retries,
		&cfg.RetriesRaw,
		p.RetriesStr,
	); err != nil {
		return err
	}
	return nil
}

func parseTarget(cfg *Cfg, p restfile.K8sProfile) error {
	rawTarget := strings.TrimSpace(p.Target)
	rawPod := strings.TrimSpace(p.Pod)

	var (
		k TargetKind
		n string
	)
	if rawTarget != "" {
		pk, pn, err := k8starget.ParseRef(rawTarget)
		if err != nil {
			return err
		}
		k, n = pk, pn
	}
	if rawPod != "" {
		if k != "" && (k != targetKindPod || n != rawPod) {
			return errors.New("k8s: target conflicts with pod")
		}
		k = targetKindPod
		n = rawPod
	}
	if k == "" || n == "" {
		return errors.New("k8s: target is required")
	}

	cfg.TargetKind = k
	cfg.TargetName = n
	if k == targetKindPod {
		cfg.Pod = n
	} else {
		cfg.Pod = ""
	}
	return nil
}

func parsePortRef(cfg *Cfg, p restfile.K8sProfile) error {
	val := strings.TrimSpace(p.PortStr)
	if val == "" {
		if cfg.Port > 0 && cfg.PortRaw == "" {
			cfg.PortRaw = strconv.Itoa(cfg.Port)
		}
		return nil
	}

	cfg.PortRaw = val
	n, err := strconv.Atoi(val)
	if err == nil {
		if n <= 0 || n > 65535 {
			return fmt.Errorf("k8s: invalid port %q", val)
		}
		cfg.Port = n
		cfg.PortName = ""
		return nil
	}

	if !k8starget.IsValidPortName(val) {
		return fmt.Errorf("k8s: invalid port %q", val)
	}
	cfg.Port = 0
	cfg.PortName = val
	return nil
}

func (c Cfg) target() (TargetKind, string) {
	k := c.TargetKind
	n := strings.TrimSpace(c.TargetName)
	if k == "" && strings.TrimSpace(c.Pod) != "" {
		k = targetKindPod
		n = strings.TrimSpace(c.Pod)
	}
	k = normalizeTargetKind(k)
	return k, n
}

func normalizeTargetKind(k TargetKind) TargetKind {
	switch k {
	case targetKindPod, targetKindService, targetKindDeployment, targetKindStatefulSet:
		return k
	default:
		return k8starget.ParseKind(string(k))
	}
}

func (c Cfg) targetRef() string {
	k, n := c.target()
	if k == "" || n == "" {
		return ""
	}
	return string(k) + "/" + n
}

func (c Cfg) portRef() string {
	if c.Port > 0 {
		return strconv.Itoa(c.Port)
	}
	return strings.TrimSpace(c.PortName)
}
