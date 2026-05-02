package rts

// CheckStr verifies s fits within the runtime string limit.
func CheckStr(ctx *Ctx, pos Pos, s string) error {
	if ctx == nil || ctx.Lim.MaxStr <= 0 {
		return nil
	}
	if len(s) > ctx.Lim.MaxStr {
		return Errf(ctx, pos, "string too long")
	}
	return nil
}

// CheckList verifies n fits within the runtime list length limit.
func CheckList(ctx *Ctx, pos Pos, n int) error {
	if ctx == nil || ctx.Lim.MaxList <= 0 {
		return nil
	}
	if n > ctx.Lim.MaxList {
		return Errf(ctx, pos, "list too large")
	}
	return nil
}

// CheckDict verifies n fits within the runtime dictionary size limit.
func CheckDict(ctx *Ctx, pos Pos, n int) error {
	if ctx == nil || ctx.Lim.MaxDict <= 0 {
		return nil
	}
	if n > ctx.Lim.MaxDict {
		return Errf(ctx, pos, "dict too large")
	}
	return nil
}
