package ui

import "github.com/unkn0wn-root/resterm/internal/restfile"

type authStore struct {
	st *namedStore[restfile.AuthProfile]
}

func newAuthStore() *authStore {
	ok := func(p restfile.AuthProfile) bool { return p.Scope == restfile.AuthScopeGlobal }
	nm := func(p restfile.AuthProfile) string { return p.Name }
	return &authStore{st: newNamedStore(ok, nm)}
}

func (s *authStore) set(p string, xs []restfile.AuthProfile) {
	if s == nil || s.st == nil {
		return
	}
	s.st.set(p, xs)
}

func (s *authStore) all() []restfile.AuthProfile {
	if s == nil || s.st == nil {
		return nil
	}
	return s.st.all()
}
