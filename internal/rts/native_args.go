package rts

type nativeArgs struct {
	ctx  *Ctx
	pos  Pos
	args []Value
	sig  string
}

func newNativeArgs(ctx *Ctx, pos Pos, args []Value, sig string) nativeArgs {
	return nativeArgs{ctx: ctx, pos: pos, args: args, sig: sig}
}

func (a nativeArgs) count(want int) error {
	return argCount(a.ctx, a.pos, a.args, want, a.sig)
}

func (a nativeArgs) countRange(min, max int) error {
	return argCountRange(a.ctx, a.pos, a.args, min, max, a.sig)
}

func (a nativeArgs) arg(i int) Value {
	return a.args[i]
}

func (a nativeArgs) str(i int) (string, error) {
	return strArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a nativeArgs) toStr(i int) (string, error) {
	return toStr(a.ctx, a.pos, a.args[i])
}

func (a nativeArgs) num(i int) (float64, error) {
	return numArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a nativeArgs) list(i int) ([]Value, error) {
	return listArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a nativeArgs) dict(i int) (map[string]Value, error) {
	return dictArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a nativeArgs) key(i int) (string, error) {
	return keyArg(a.ctx, a.pos, a.args[i], a.sig)
}

func (a nativeArgs) mapKey(key string) (string, error) {
	return mapKey(a.ctx, a.pos, key, a.sig)
}
