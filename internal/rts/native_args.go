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

func (a nativeArgs) none() error {
	return a.count(0)
}

func (a nativeArgs) countRange(min, max int) error {
	return argCountRange(a.ctx, a.pos, a.args, min, max, a.sig)
}

func (a nativeArgs) len() int {
	return len(a.args)
}

func (a nativeArgs) has(i int) bool {
	return i >= 0 && i < len(a.args)
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

func (a nativeArgs) finiteNum(i int) (float64, error) {
	return numF(a.ctx, a.pos, a.args[i], a.sig)
}

func (a nativeArgs) bool(i int) (bool, error) {
	v := a.args[i]
	if v.K != VBool {
		return false, rtErr(a.ctx, a.pos, "%s expects bool", a.sig)
	}
	return v.B, nil
}

func (a nativeArgs) scalarStr(i int) (string, error) {
	return scalarStr(a.ctx, a.pos, a.args[i], a.sig)
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

func (a nativeArgs) fn(i int) (Value, error) {
	v := a.args[i]
	if err := fnChk(a.ctx, a.pos, v, a.sig); err != nil {
		return Null(), err
	}
	return v, nil
}
