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
func RuntimeGlobals(globals map[string]vars.GlobalMutation, policy SecretPolicy) map[string]string {
	out := make(map[string]string, len(globals))
	for key, mut := range globals {
		if mut.Delete || (policy == OmitSecrets && mut.Secret) {
			continue
		}
		name := util.FirstTrimmed(mut.Name, key)
		if name == "" {
			continue
		}
		out[util.Lower(name)] = mut.Value
	}
	return out
}
