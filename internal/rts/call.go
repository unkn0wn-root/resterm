package rts

// CallValue invokes an RTS function or native value from host/runtime code.
func CallValue(ctx *Ctx, pos Pos, fn Value, args []Value) (Value, error) {
	vm := &VM{ctx: ctx}
	return vm.callVal(pos, fn, args)
}

// CheckFunc validates that v can be called as an RTS function.
func CheckFunc(ctx *Ctx, pos Pos, v Value, sig string) error {
	if v.K == VFunc || v.K == VNative {
		return nil
	}
	return Errf(ctx, pos, "%s expects function", sig)
}

// Tick charges one runtime step for native loops.
func Tick(ctx *Ctx, pos Pos) error {
	if ctx == nil {
		return nil
	}
	return ctx.tick(pos)
}
