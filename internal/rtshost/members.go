package rtshost

import (
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
)

// Modules can retain host objects after a block returns. Reject those stale objects.
type evalObj struct {
	st   *evalState
	name string
}

func (o evalObj) TypeName() string { return o.name }

func (o evalObj) Unbound() string {
	if !o.st.ended {
		return ""
	}
	return o.name + " belongs to an evaluation that has ended"
}

// Saved methods bypass object lookup, so they need their own lifetime check.
func (o evalObj) method(d native.Def) rts.Value {
	fn := d.Func()
	return rts.NativeNamed(d.Name(), func(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
		if why := o.Unbound(); why != "" {
			return rts.Null(), rts.Abortf(ctx, pos, "%s", why)
		}
		return fn(ctx, pos, args)
	})
}

func (o evalObj) fn0(member string, fn func(native.Call) (rts.Value, error)) rts.Value {
	name := o.name + "." + member
	return o.method(native.Fn0(name, name+"()", fn))
}
