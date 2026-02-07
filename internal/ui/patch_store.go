package ui

import "github.com/unkn0wn-root/resterm/internal/restfile"

type patchStore struct {
	st *namedStore[restfile.PatchProfile]
}

func newPatchStore() *patchStore {
	ok := func(p restfile.PatchProfile) bool { return p.Scope == restfile.PatchScopeGlobal }
	nm := func(p restfile.PatchProfile) string { return p.Name }
	return &patchStore{st: newNamedStore(ok, nm)}
}

func (s *patchStore) set(p string, xs []restfile.PatchProfile) {
	if s == nil || s.st == nil {
		return
	}
	s.st.set(p, xs)
}

func (s *patchStore) all() []restfile.PatchProfile {
	if s == nil || s.st == nil {
		return nil
	}
	return s.st.all()
}
