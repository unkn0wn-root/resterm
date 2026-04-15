package runtime

import (
	"errors"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/ssh"
)

type Config struct {
	Client     *httpclient.Client
	History    history.Store
	SSHManager *ssh.Manager
	K8sManager *k8s.Manager
}

type Runtime struct {
	gs *Globals
	fs *Files
	ck *Cookies
	ac *authcmd.Manager
	oa *oauth.Manager
	hs history.Store
	sm *ssh.Manager
	km *k8s.Manager
}

func New(cfg Config) *Runtime {
	sm := cfg.SSHManager
	if sm == nil {
		sm = ssh.NewManager()
	}
	km := cfg.K8sManager
	if km == nil {
		km = k8s.NewManager()
	}
	return &Runtime{
		gs: NewGlobals(),
		fs: NewFiles(),
		ck: NewCookies(),
		ac: authcmd.NewManager(),
		oa: oauth.NewManager(cfg.Client),
		hs: cfg.History,
		sm: sm,
		km: km,
	}
}

func (r *Runtime) Globals() *Globals {
	if r == nil {
		return nil
	}
	if r.gs == nil {
		r.gs = NewGlobals()
	}
	return r.gs
}

func (r *Runtime) Files() *Files {
	if r == nil {
		return nil
	}
	if r.fs == nil {
		r.fs = NewFiles()
	}
	return r.fs
}

func (r *Runtime) AuthCmd() *authcmd.Manager {
	if r == nil {
		return nil
	}
	if r.ac == nil {
		r.ac = authcmd.NewManager()
	}
	return r.ac
}

func (r *Runtime) OAuth() *oauth.Manager {
	if r == nil {
		return nil
	}
	if r.oa == nil {
		r.oa = oauth.NewManager(nil)
	}
	return r.oa
}

func (r *Runtime) History() history.Store {
	if r == nil {
		return nil
	}
	return r.hs
}

func (r *Runtime) SSH() *ssh.Manager {
	if r == nil {
		return nil
	}
	if r.sm == nil {
		r.sm = ssh.NewManager()
	}
	return r.sm
}

func (r *Runtime) K8s() *k8s.Manager {
	if r == nil {
		return nil
	}
	if r.km == nil {
		r.km = k8s.NewManager()
	}
	return r.km
}

func (r *Runtime) RuntimeState() engine.RuntimeState {
	if r == nil {
		return engine.RuntimeState{}
	}
	var out engine.RuntimeState
	if gs := r.Globals(); gs != nil {
		out.Globals = gs.Entries()
	}
	if fs := r.Files(); fs != nil {
		out.Files = fs.Entries()
	}
	return out
}

func (r *Runtime) LoadRuntimeState(st engine.RuntimeState) {
	if r == nil {
		return
	}
	if gs := r.Globals(); gs != nil {
		gs.Restore(st.Globals)
	}
	if fs := r.Files(); fs != nil {
		fs.Restore(st.Files)
	}
}

func (r *Runtime) AuthState() engine.AuthState {
	if r == nil {
		return engine.AuthState{}
	}
	var out engine.AuthState
	if oa := r.OAuth(); oa != nil {
		out.OAuth = oa.Snapshot()
	}
	if ac := r.AuthCmd(); ac != nil {
		out.Command = ac.Snapshot()
	}
	return out
}

func (r *Runtime) LoadAuthState(st engine.AuthState) {
	if r == nil {
		return
	}
	if oa := r.OAuth(); oa != nil {
		oa.Restore(st.OAuth)
	}
	if ac := r.AuthCmd(); ac != nil {
		ac.Restore(st.Command)
	}
}

func (r *Runtime) Cookies() *Cookies {
	if r == nil {
		return nil
	}
	return r.ck
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.hs != nil {
		errs = append(errs, r.hs.Close())
	}
	if r.sm != nil {
		errs = append(errs, r.sm.Close())
	}
	if r.km != nil {
		errs = append(errs, r.km.Close())
	}
	return errors.Join(errs...)
}

func envKey(name string) string {
	if strings.TrimSpace(name) == "" {
		return "__default__"
	}
	return strings.ToLower(strings.TrimSpace(name))
}

func stateEnv(key string) string {
	if key == "__default__" {
		return ""
	}
	return key
}

func nameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
