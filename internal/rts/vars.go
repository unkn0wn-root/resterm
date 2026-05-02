package rts

import "strings"

type VarsMut interface {
	SetVar(name, value string)
}

type GlobalMut interface {
	SetGlobal(name, value string, secret bool)
	DelGlobal(name string)
}

type varsObj struct {
	name string
	m    map[string]string
	g    *globalObj
	mut  VarsMut
	s    ms
}

type globalObj struct {
	name string
	m    map[string]string
	mut  GlobalMut
	s    ms
}

func newVarsObj(
	name string,
	vars map[string]string,
	globals map[string]string,
	mut VarsMut,
	gmut GlobalMut,
) *varsObj {
	if strings.TrimSpace(name) == "" {
		name = "vars"
	}
	v := &varsObj{
		name: name,
		m:    lowerMap(vars),
		mut:  mut,
		s:    newMS(name),
	}
	v.g = newGlobalObj(name+".global", globals, gmut)
	return v
}

func newGlobalObj(name string, globals map[string]string, mut GlobalMut) *globalObj {
	if strings.TrimSpace(name) == "" {
		name = "vars.global"
	}
	return &globalObj{name: name, m: lowerMap(globals), mut: mut, s: newMS(name)}
}

func (o *varsObj) TypeName() string { return o.name }

func (o *varsObj) GetMember(name string) (Value, bool) {
	switch name {
	case "get":
		return NativeNamed(o.name+".get", o.getFn), true
	case "has":
		return NativeNamed(o.name+".has", o.hasFn), true
	case "set":
		return NativeNamed(o.name+".set", o.setFn), true
	case "require":
		return NativeNamed(o.name+".require", o.requireFn), true
	case "global":
		return Obj(o.g), true
	}

	return mapMember(o.m, name)
}

func (o *varsObj) Index(key Value) (Value, error) {
	return mapIndex(o.m, key)
}

func (o *varsObj) getFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapGet(ctx, pos, args, o.s.g, o.m)
}

func (o *varsObj) hasFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapHas(ctx, pos, args, o.s.h, o.m)
}

func (o *varsObj) requireFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapRequire(ctx, pos, args, o.s.r, o.name, o.m)
}

func (o *varsObj) setFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := o.name + ".set(name, value)"
	na := NewArgs(ctx, pos, args, sig)
	if err := na.Count(2); err != nil {
		return Null(), err
	}
	if o.mut == nil {
		return Null(), Errf(ctx, pos, "%s is read-only", o.name)
	}
	name, err := na.Key(0)
	if err != nil {
		return Null(), err
	}
	val, err := na.ScalarStr(1)
	if err != nil {
		return Null(), err
	}
	o.mut.SetVar(name, val)
	key := lookupKey(name)
	o.m[key] = val
	return Null(), nil
}

func (o *globalObj) TypeName() string { return o.name }

func (o *globalObj) GetMember(name string) (Value, bool) {
	switch name {
	case "get":
		return NativeNamed(o.name+".get", o.getFn), true
	case "has":
		return NativeNamed(o.name+".has", o.hasFn), true
	case "set":
		return NativeNamed(o.name+".set", o.setFn), true
	case "delete":
		return NativeNamed(o.name+".delete", o.delFn), true
	case "require":
		return NativeNamed(o.name+".require", o.requireFn), true
	}

	return mapMember(o.m, name)
}

func (o *globalObj) Index(key Value) (Value, error) {
	return mapIndex(o.m, key)
}

func (o *globalObj) getFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapGet(ctx, pos, args, o.s.g, o.m)
}

func (o *globalObj) hasFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapHas(ctx, pos, args, o.s.h, o.m)
}

func (o *globalObj) requireFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return mapRequire(ctx, pos, args, o.s.r, o.name, o.m)
}

func (o *globalObj) setFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := o.name + ".set(name, value[, secret])"
	na := NewArgs(ctx, pos, args, sig)
	if err := na.CountRange(2, 3); err != nil {
		return Null(), err
	}
	if o.mut == nil {
		return Null(), Errf(ctx, pos, "%s is read-only", o.name)
	}
	name, err := na.Key(0)
	if err != nil {
		return Null(), err
	}
	val, err := na.ScalarStr(1)
	if err != nil {
		return Null(), err
	}
	secret := false
	if na.Has(2) {
		secret, err = na.Bool(2)
		if err != nil {
			return Null(), err
		}
	}
	o.mut.SetGlobal(name, val, secret)
	key := lookupKey(name)
	o.m[key] = val
	return Null(), nil
}

func (o *globalObj) delFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := o.name + ".delete(name)"
	na := NewArgs(ctx, pos, args, sig)
	if err := na.Count(1); err != nil {
		return Null(), err
	}
	if o.mut == nil {
		return Null(), Errf(ctx, pos, "%s is read-only", o.name)
	}
	name, err := na.Key(0)
	if err != nil {
		return Null(), err
	}
	o.mut.DelGlobal(name)
	key := lookupKey(name)
	delete(o.m, key)
	return Null(), nil
}
