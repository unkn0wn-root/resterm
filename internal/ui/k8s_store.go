package ui

import (
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func newK8sStore() *namedStore[restfile.K8sProfile] {
	ok := func(p restfile.K8sProfile) bool { return p.Scope == restfile.K8sScopeGlobal }
	nm := func(p restfile.K8sProfile) string { return p.Name }
	return newNamedStore(ok, nm)
}
