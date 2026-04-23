package k8s

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	k8starget "github.com/unkn0wn-root/resterm/internal/k8s/target"
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
	TargetPod         TargetKind = k8starget.Pod
	TargetService     TargetKind = k8starget.Service
	TargetDeployment  TargetKind = k8starget.Deployment
	TargetStatefulSet TargetKind = k8starget.StatefulSet
)

type TargetRef struct {
	Kind TargetKind
	Name string
}

type PortRef struct {
	Number int
	Name   string
}

type Config struct {
	Name       string
	Namespace  string
	Target     TargetRef
	Port       PortRef
	Context    string
	Kubeconfig string
	Container  string
	Address    string
	LocalPort  int
	Persist    bool
	PodWait    time.Duration
	Retries    int
	Label      string
}

type execConfig struct {
	Config
}

func prepareExecConfig(cfg Config) (execConfig, error) {
	cfg = cfg.normalize()
	if err := cfg.validate(); err != nil {
		return execConfig{}, fmt.Errorf("k8s: %w", err)
	}
	return execConfig{Config: cfg}, nil
}

func normalizeProfile(p restfile.K8sProfile) (Config, error) {
	cfg := baseConfig(p)

	tg, err := targetFromProfile(p)
	if err != nil {
		return Config{}, err
	}
	cfg.Target = tg

	if err := parseProfileOptions(&cfg, p); err != nil {
		return Config{}, err
	}
	if err := expandKubeconfigPath(&cfg); err != nil {
		return Config{}, err
	}
	cfg = cfg.normalize()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func baseConfig(p restfile.K8sProfile) Config {
	return Config{
		Name:       strings.TrimSpace(p.Name),
		Namespace:  connprofile.Fallback(strings.TrimSpace(p.Namespace), defaultNamespace),
		Context:    strings.TrimSpace(p.Context),
		Kubeconfig: strings.TrimSpace(p.Kubeconfig),
		Container:  strings.TrimSpace(p.Container),
		Address:    connprofile.Fallback(strings.TrimSpace(p.Address), defaultAddress),
		LocalPort:  p.LocalPort,
		Persist:    p.Persist.Set && p.Persist.Val,
		PodWait:    defaultPodWait,
		Retries:    0,
		Port: PortRef{
			Number: p.Port,
		},
	}
}

func (c Config) normalize() Config {
	c.Name = strings.TrimSpace(c.Name)
	c.Namespace = connprofile.Fallback(strings.TrimSpace(c.Namespace), defaultNamespace)
	c.Target = c.Target.normalize()
	c.Port = c.Port.normalize()
	c.Context = strings.TrimSpace(c.Context)
	c.Kubeconfig = strings.TrimSpace(c.Kubeconfig)
	c.Container = strings.TrimSpace(c.Container)
	c.Address = connprofile.Fallback(strings.TrimSpace(c.Address), defaultAddress)
	c.Label = strings.TrimSpace(c.Label)
	return c
}

func (c Config) validate() error {
	if err := c.Target.validate(); err != nil {
		return err
	}
	if err := c.Port.validate(); err != nil {
		return err
	}
	if c.LocalPort < 0 || c.LocalPort > 65535 {
		return errors.New("local port out of range")
	}
	return nil
}

func (t TargetRef) normalize() TargetRef {
	raw := strings.TrimSpace(string(t.Kind))
	t.Name = strings.TrimSpace(t.Name)

	switch {
	case raw == "" && t.Name != "":
		t.Kind = TargetPod
	case raw == "":
		t.Kind = ""
	default:
		if kind := normalizeTargetKind(TargetKind(raw)); kind != "" {
			t.Kind = kind
		} else {
			t.Kind = TargetKind(raw)
		}
	}
	return t
}

func (t TargetRef) validate() error {
	t = t.normalize()
	if t.Name == "" {
		return errors.New("target is required")
	}
	if kind := normalizeTargetKind(t.Kind); kind == "" {
		return fmt.Errorf("unsupported target kind %q", strings.TrimSpace(string(t.Kind)))
	}
	return nil
}

func (p PortRef) normalize() PortRef {
	p.Name = strings.TrimSpace(p.Name)
	if p.Number > 0 {
		p.Name = ""
	}
	return p
}

func (p PortRef) validate() error {
	p = p.normalize()
	switch {
	case p.Number < 0 || p.Number > 65535:
		return errors.New("port out of range")
	case p.Number > 0:
		return nil
	case p.Name == "":
		return errors.New("port is required")
	case !k8starget.IsValidPortName(p.Name):
		return fmt.Errorf("invalid port %q", p.Name)
	default:
		return nil
	}
}

func (p PortRef) String() string {
	p = p.normalize()
	return portRefString(p)
}

func parseProfileOptions(cfg *Config, p restfile.K8sProfile) error {
	if err := parsePortRef(&cfg.Port, p.PortStr); err != nil {
		return err
	}

	var raw string
	if err := connprofile.ParsePort("local", &cfg.LocalPort, &raw, p.LocalPortStr); err != nil {
		return err
	}
	if err := connprofile.ParseDuration("pod wait", &cfg.PodWait, &raw, p.PodWaitStr); err != nil {
		return err
	}
	if err := connprofile.ParseRetries("k8s", &cfg.Retries, &raw, p.RetriesStr); err != nil {
		return err
	}
	return nil
}

func targetFromProfile(p restfile.K8sProfile) (TargetRef, error) {
	rawTarget := strings.TrimSpace(p.Target)
	rawPod := strings.TrimSpace(p.Pod)

	var tg TargetRef
	if rawTarget != "" {
		kind, name, err := k8starget.ParseRef(rawTarget)
		if err != nil {
			return TargetRef{}, err
		}
		tg = TargetRef{Kind: kind, Name: name}
	}
	if rawPod != "" {
		if tg.Name != "" && (tg.Kind != TargetPod || tg.Name != rawPod) {
			return TargetRef{}, errors.New("target conflicts with pod")
		}
		tg = TargetRef{Kind: TargetPod, Name: rawPod}
	}
	if tg.Name == "" {
		return TargetRef{}, errors.New("target is required")
	}
	return tg, nil
}

func expandKubeconfigPath(cfg *Config) error {
	if cfg == nil || cfg.Kubeconfig == "" {
		return nil
	}

	path, err := connprofile.ExpandPath(
		cfg.Kubeconfig,
		"cannot resolve home directory for kubeconfig path",
	)
	if err != nil {
		return err
	}
	cfg.Kubeconfig = path
	return nil
}

func parsePortRef(ref *PortRef, raw string) error {
	if ref == nil {
		return nil
	}

	val := strings.TrimSpace(raw)
	if val == "" {
		return nil
	}

	n, err := strconv.Atoi(val)
	if err == nil {
		if n <= 0 || n > 65535 {
			return fmt.Errorf("invalid port %q", val)
		}
		ref.Number = n
		ref.Name = ""
		return nil
	}

	if !k8starget.IsValidPortName(val) {
		return fmt.Errorf("invalid port %q", val)
	}
	ref.Number = 0
	ref.Name = val
	return nil
}

func normalizeTargetKind(k TargetKind) TargetKind {
	switch k {
	case TargetPod, TargetService, TargetDeployment, TargetStatefulSet:
		return k
	default:
		return k8starget.ParseKind(string(k))
	}
}

func (c Config) DisplayRef() string {
	c = c.normalize()
	ref := targetDisplayRef(c.Target)
	if ref == "" {
		return ""
	}
	if c.Namespace != "" {
		ref = c.Namespace + "/" + ref
	}
	if port := portRefString(c.Port); port != "" {
		ref += ":" + port
	}
	if c.Context == "" {
		return ref
	}
	return c.Context + " " + ref
}

func (c execConfig) targetRef() string {
	return targetRefKey(c.Target)
}

func (c execConfig) portRef() string {
	return portRefString(c.Port)
}

func targetRefKey(t TargetRef) string {
	if t.Kind == "" || t.Name == "" {
		return ""
	}
	return string(t.Kind) + "/" + t.Name
}

func targetDisplayRef(t TargetRef) string {
	if t.Kind == "" || t.Name == "" {
		return ""
	}
	if t.Kind == TargetPod {
		return t.Name
	}
	return string(t.Kind) + "/" + t.Name
}

func portRefString(p PortRef) string {
	if p.Number > 0 {
		return strconv.Itoa(p.Number)
	}
	return p.Name
}
