package rts

type resultObj struct {
	ok  bool
	val Value
	err string
}

func newResult(ok bool, val Value, err error) Value {
	r := &resultObj{ok: ok}
	if ok {
		r.val = val
	} else {
		r.val = Null()
		if err != nil {
			r.err = err.Error()
		}
	}
	return Obj(r)
}

func (o *resultObj) TypeName() string { return "result" }

func (o *resultObj) Member(ctx *Ctx, pos Pos, name string) (Value, bool, error) {
	switch name {
	case "ok":
		return Bool(o.ok), true, nil
	case "value":
		return o.val, true, nil
	case "error":
		if o.ok || o.err == "" {
			return Null(), true, nil
		}
		if err := CheckStr(ctx, pos, o.err); err != nil {
			return Null(), true, err
		}
		return Str(o.err), true, nil
	}
	return Null(), false, nil
}

func (o *resultObj) Index(ctx *Ctx, pos Pos, key Value) (Value, error) {
	k, err := Key(pos, key)
	if err != nil {
		return Null(), err
	}
	v, ok, err := o.Member(ctx, pos, k)
	if err != nil {
		return Null(), err
	}
	if ok {
		return v, nil
	}
	return Null(), nil
}

func (o *resultObj) Truthy() bool { return o.ok }

func (o *resultObj) ToInterface() any {
	out := map[string]any{
		"ok":    o.ok,
		"value": ToIface(o.val),
	}
	if o.ok || o.err == "" {
		out["error"] = nil
	} else {
		out["error"] = o.err
	}
	return out
}
