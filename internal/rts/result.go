package rts

import "fmt"

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

func (o *resultObj) GetMember(name string) (Value, bool) {
	switch name {
	case "ok":
		return Bool(o.ok), true
	case "value":
		return o.val, true
	case "error":
		if o.ok || o.err == "" {
			return Null(), true
		}
		return Str(o.err), true
	}
	return Null(), false
}

func (o *resultObj) CallMember(name string, args []Value) (Value, error) {
	return Null(), fmt.Errorf("no member call: %s", name)
}

func (o *resultObj) Index(key Value) (Value, error) {
	k, err := toKey(Pos{}, key)
	if err != nil {
		return Null(), err
	}
	if v, ok := o.GetMember(k); ok {
		return v, nil
	}
	return Null(), nil
}

func (o *resultObj) Truthy() bool { return o.ok }

func (o *resultObj) ToInterface() any {
	out := map[string]any{
		"ok":    o.ok,
		"value": toIface(o.val),
	}
	if o.ok || o.err == "" {
		out["error"] = nil
	} else {
		out["error"] = o.err
	}
	return out
}
