package ui

import (
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func newSSHStore() *namedStore[restfile.SSHProfile] {
	ok := func(p restfile.SSHProfile) bool { return p.Scope == restfile.SSHScopeGlobal }
	nm := func(p restfile.SSHProfile) string { return p.Name }
	return newNamedStore(ok, nm)
}
