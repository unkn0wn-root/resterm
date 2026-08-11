package rtshost

import (
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// SecretPolicy decides whether secret globals reach the runtime.
type SecretPolicy int

const (
	// OmitSecrets is the zero value so an unset policy fails safe.
	OmitSecrets SecretPolicy = iota
	IncludeSecrets
)

// RuntimeGlobals returns global values keyed the way RTS variable lookup expects.
func RuntimeGlobals(globals vars.Globals, policy SecretPolicy) map[string]string {
	out := make(map[string]string, globals.Len())
	for name, mut := range globals.All() {
		if mut.Delete || (policy == OmitSecrets && mut.Secret) {
			continue
		}
		out[util.Lower(name)] = mut.Value
	}
	return out
}
