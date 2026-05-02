package rts

func testStdlib() map[string]Value {
	return map[string]Value{
		"len": NativeNamed("len", testLen),
		"str": NativeNamed("str", testStr),
	}
}

func testLen(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := NewArgs(ctx, pos, args, "len(x)")
	if err := na.Count(1); err != nil {
		return Null(), err
	}

	switch v := na.Arg(0); v.K {
	case VStr:
		return Num(float64(len(v.S))), nil
	case VList:
		return Num(float64(len(v.L))), nil
	case VDict:
		return Num(float64(len(v.M))), nil
	default:
		return Null(), Errf(ctx, pos, "len(x) unsupported")
	}
}

func testStr(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := NewArgs(ctx, pos, args, "str(x)")
	if err := na.Count(1); err != nil {
		return Null(), err
	}

	s, err := na.ToStr(0)
	if err != nil {
		return Null(), err
	}
	return Str(s), nil
}
