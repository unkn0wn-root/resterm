package rts

func callFn(ctx *Ctx, pos Pos, fn Value, args []Value) (Value, error) {
	vm := &VM{ctx: ctx}
	return vm.callVal(pos, fn, args)
}

func fnChk(ctx *Ctx, pos Pos, v Value, sig string) error {
	if v.K == VFunc || v.K == VNative {
		return nil
	}
	return rtErr(ctx, pos, "%s expects function", sig)
}

func ctxTick(ctx *Ctx, pos Pos) error {
	if ctx == nil {
		return nil
	}
	return ctx.tick(pos)
}
