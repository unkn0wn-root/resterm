package k8s

type Plan struct {
	Manager *Manager
	Config  *Config
}

func (p *Plan) Active() bool {
	return p != nil && p.Manager != nil && p.Config != nil
}
