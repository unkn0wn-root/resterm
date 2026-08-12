package rtshost

import (
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
)

// hostFn0 uses the same qualified name for lookup, stack traces, and errors.
func hostFn0(obj, member string, fn func(native.Call) (rts.Value, error)) rts.Value {
	name := obj + "." + member
	return native.Fn0(name, name+"()", fn).Value()
}
