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
}

type globalObj struct {
	name string
	m    map[string]string
	mut  GlobalMut
}

func newVarsObj(name string, vars map[string]string, globals map[string]string, mut VarsMut, gmut GlobalMut) *varsObj {
	if strings.TrimSpace(name) == "" {
		name = "vars"
	}
	v := &varsObj{
		name: name,
		m:    lowerMap(vars),
		mut:  mut,
	}
	v.g = newGlobalObj(name+".global", globals, gmut)
	return v
}

func newGlobalObj(name string, globals map[string]string, mut GlobalMut) *globalObj {
	if strings.TrimSpace(name) == "" {
		name = "vars.global"
	}
	return &globalObj{name: name, m: lowerMap(globals), mut: mut}
}

func lowerMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[lowerKey(k)] = v
	}
	return out
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
	case "global":
		return Obj(o.g), true
	}

	key := lowerKey(name)
	v, ok := o.m[key]
	if !ok {
		return Null(), false
	}
	return Str(v), true
}

func (o *varsObj) CallMember(name string, args []Value) (Value, error) {
	return Null(), rtErr(nil, Pos{}, "no member call: %s", name)
}

func (o *varsObj) Index(key Value) (Value, error) {
	k, err := toKey(Pos{}, key)
	if err != nil {
		return Null(), err
	}
	lk := lowerKey(k)
	v, ok := o.m[lk]
	if !ok {
		return Null(), nil
	}
	return Str(v), nil
}

func (o *varsObj) getFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".get(name)"); err != nil {
		return Null(), err
	}
	k, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}
	key := lowerKey(k)
	v, ok := o.m[key]
	if !ok {
		return Null(), nil
	}
	return Str(v), nil
}

func (o *varsObj) hasFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".has(name)"); err != nil {
		return Null(), err
	}
	k, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}
	key := lowerKey(k)
	_, ok := o.m[key]
	return Bool(ok), nil
}

func (o *varsObj) setFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, o.name+".set(name, value)"); err != nil {
		return Null(), err
	}
	if o.mut == nil {
		return Null(), rtErr(ctx, pos, "%s is read-only", o.name)
	}
	name, err := keyArg(ctx, pos, args[0], o.name+".set(name, value)")
	if err != nil {
		return Null(), err
	}
	val, err := scalarStr(ctx, pos, args[1], o.name+".set(name, value)")
	if err != nil {
		return Null(), err
	}
	o.mut.SetVar(name, val)
	key := lowerKey(name)
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
	}

	key := lowerKey(name)
	v, ok := o.m[key]
	if !ok {
		return Null(), false
	}
	return Str(v), true
}

func (o *globalObj) CallMember(name string, args []Value) (Value, error) {
	return Null(), rtErr(nil, Pos{}, "no member call: %s", name)
}

func (o *globalObj) Index(key Value) (Value, error) {
	k, err := toKey(Pos{}, key)
	if err != nil {
		return Null(), err
	}
	lk := lowerKey(k)
	v, ok := o.m[lk]
	if !ok {
		return Null(), nil
	}
	return Str(v), nil
}

func (o *globalObj) getFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".get(name)"); err != nil {
		return Null(), err
	}
	k, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}
	key := lowerKey(k)
	v, ok := o.m[key]
	if !ok {
		return Null(), nil
	}
	return Str(v), nil
}

func (o *globalObj) hasFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".has(name)"); err != nil {
		return Null(), err
	}
	k, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}
	key := lowerKey(k)
	_, ok := o.m[key]
	return Bool(ok), nil
}

func (o *globalObj) setFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCountRange(ctx, pos, args, 2, 3, o.name+".set(name, value[, secret])"); err != nil {
		return Null(), err
	}
	if o.mut == nil {
		return Null(), rtErr(ctx, pos, "%s is read-only", o.name)
	}
	name, err := keyArg(ctx, pos, args[0], o.name+".set(name, value[, secret])")
	if err != nil {
		return Null(), err
	}
	val, err := scalarStr(ctx, pos, args[1], o.name+".set(name, value[, secret])")
	if err != nil {
		return Null(), err
	}
	secret := false
	if len(args) == 3 {
		if args[2].K != VBool {
			return Null(), rtErr(ctx, pos, "%s.set(name, value[, secret]) expects secret bool", o.name)
		}
		secret = args[2].B
	}
	o.mut.SetGlobal(name, val, secret)
	key := lowerKey(name)
	o.m[key] = val
	return Null(), nil
}

func (o *globalObj) delFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, o.name+".delete(name)"); err != nil {
		return Null(), err
	}
	if o.mut == nil {
		return Null(), rtErr(ctx, pos, "%s is read-only", o.name)
	}
	name, err := keyArg(ctx, pos, args[0], o.name+".delete(name)")
	if err != nil {
		return Null(), err
	}
	o.mut.DelGlobal(name)
	key := lowerKey(name)
	delete(o.m, key)
	return Null(), nil
}
