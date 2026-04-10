package ui

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/ssh"
)

type globalStore = rtrun.Globals
type globalValue = rtrun.GlobalValue
type fileStore = rtrun.Files
type fileVariable = rtrun.FileValue

func newRuntime(cfg rtrun.Config) *rtrun.Runtime {
	return rtrun.New(cfg)
}

func normalizeNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (m *Model) runtimeSvc() *rtrun.Runtime {
	if m == nil {
		return nil
	}
	if m.run == nil {
		m.run = newRuntime(rtrun.Config{Client: m.client})
	}
	return m.run
}

func (m *Model) globalsStore() *globalStore {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	return rt.Globals()
}

func (m *Model) fileStore() *fileStore {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	return rt.Files()
}

func (m *Model) authCmdMgr() *authcmd.Manager {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	return rt.AuthCmd()
}

func (m *Model) oauthMgr() *oauth.Manager {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	return rt.OAuth()
}

func (m *Model) historyStore() history.Store {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	return rt.History()
}

func (m *Model) sshManager() *ssh.Manager {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	return rt.SSH()
}

func (m *Model) k8sManager() *k8s.Manager {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	return rt.K8s()
}
