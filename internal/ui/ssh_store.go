package ui

import (
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type sshStore struct {
	st *namedStore[restfile.SSHProfile]
}

func newSSHStore() *sshStore {
	ok := func(p restfile.SSHProfile) bool { return p.Scope == restfile.SSHScopeGlobal }
	nm := func(p restfile.SSHProfile) string { return p.Name }
	return &sshStore{st: newNamedStore(ok, nm)}
}

func (s *sshStore) set(path string, profiles []restfile.SSHProfile) {
	if s == nil || s.st == nil {
		return
	}
	s.st.set(path, profiles)
}

func (s *sshStore) all() []restfile.SSHProfile {
	if s == nil || s.st == nil {
		return nil
	}
	return s.st.all()
}
