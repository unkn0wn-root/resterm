package ssh

type Plan struct {
	Manager *Manager
	Config  *Cfg
}

func (p *Plan) Active() bool {
	return p != nil && p.Manager != nil && p.Config != nil
}
