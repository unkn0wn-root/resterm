package rtshost

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// mapObj provides shared lookup methods for env, vars, and vars.global.
// Constructors add the members specific to each object.
type mapObj struct {
	name    string
	vals    vars.NameMap[string]
	members map[string]rts.Value
}

func newMapObj(name string, vals vars.NameView[string]) *mapObj {
	o := &mapObj{name: name, vals: vals.Clone()}
	o.members = map[string]rts.Value{
		"get":     o.getDef().Value(),
		"has":     o.hasDef().Value(),
		"require": o.requireDef().Value(),
	}
	return o
}

func (o *mapObj) TypeName() string { return o.name }

func (o *mapObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	if v, ok := o.members[name]; ok {
		return v, true, nil
	}
	v, ok := o.vals.Get(name)
	if !ok {
		return rts.Null(), false, nil
	}
	out, err := native.StringValue(ctx, pos, v)
	return out, true, err
}

// Index reads values only. A value whose name is also a member, such as a
// variable called get, stays reachable through the index form.
func (o *mapObj) Index(ctx *rts.Ctx, pos rts.Pos, key rts.Value) (rts.Value, error) {
	name, err := rts.Key(pos, key)
	if err != nil {
		return rts.Null(), err
	}
	v, ok := o.vals.Get(name)
	if !ok {
		return rts.Null(), nil
	}
	return native.StringValue(ctx, pos, v)
}

func (o *mapObj) getDef() native.Def {
	sig := o.name + ".get(name)"
	return native.Fn1(o.name+".get", sig, nameArg,
		func(call native.Call, name string) (rts.Value, error) {
			v, ok := o.vals.Get(name)
			if !ok {
				return rts.Null(), nil
			}
			return native.StringValue(call.Ctx, call.Pos, v)
		},
	)
}

func (o *mapObj) hasDef() native.Def {
	sig := o.name + ".has(name)"
	return native.Fn1(o.name+".has", sig, nameArg,
		func(_ native.Call, name string) (rts.Value, error) {
			return rts.Bool(o.vals.Has(name)), nil
		},
	)
}

func (o *mapObj) requireDef() native.Def {
	sig := o.name + ".require(name[, message])"
	return native.Fn1Optional(o.name+".require", sig, nameArg, native.String,
		func(call native.Call, name string, msg native.Optional[string]) (rts.Value, error) {
			if v, ok := o.vals.Get(name); ok && strings.TrimSpace(v) != "" {
				return native.StringValue(call.Ctx, call.Pos, v)
			}
			text := ""
			if msg.Set {
				text = strings.TrimSpace(msg.Value)
			}
			if text == "" {
				text = fmt.Sprintf("missing required %s: %s", o.name, name)
			}
			return rts.Null(), call.Errorf("%s", text)
		},
	)
}

func nameArg(call native.Call, v rts.Value) (string, error) {
	name, err := native.String(call, v)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", call.Errorf("%s expects a non-empty name", call.Sig)
	}
	return name, nil
}

func newEnvObj(scope Scope) *mapObj {
	o := newMapObj("env", scope.Env)
	o.members["meta"] = rts.Obj(&envMetaObj{
		meta:   scope.Meta,
		groups: newMapObj("env.meta.groups", scope.Meta.Groups),
	})
	return o
}

type envMetaObj struct {
	meta   EnvMeta
	groups *mapObj
}

func (o *envMetaObj) TypeName() string { return "env.meta" }

func (o *envMetaObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	switch name {
	case "name":
		v, err := native.StringValue(ctx, pos, o.meta.Name)
		return v, true, err
	case "groups":
		return rts.Obj(o.groups), true, nil
	default:
		return rts.Null(), false, nil
	}
}

func (o *envMetaObj) Index(ctx *rts.Ctx, pos rts.Pos, key rts.Value) (rts.Value, error) {
	name, err := rts.Key(pos, key)
	if err != nil {
		return rts.Null(), err
	}
	v, ok, err := o.Member(ctx, pos, name)
	if err != nil || !ok {
		return rts.Null(), err
	}
	return v, nil
}

func newVarsObj(scope Scope, varsMut VarsMutator, globalMut GlobalMutator) *mapObj {
	o := newMapObj("vars", scope.Vars)
	o.members["set"] = varsSetDef(o, varsMut).Value()
	o.members["global"] = rts.Obj(newGlobalObj(scope.Globals, globalMut))
	return o
}

func varsSetDef(o *mapObj, mut VarsMutator) native.Def {
	sig := "vars.set(name, value)"
	return native.Fn2("vars.set", sig, nameArg, native.String,
		func(call native.Call, name, value string) (rts.Value, error) {
			if mut == nil {
				return rts.Null(), call.Errorf("vars is read-only")
			}
			mut.SetVar(name, value)
			o.vals.Set(name, value)
			return rts.Null(), nil
		},
	)
}

func newGlobalObj(vals vars.NameView[string], mut GlobalMutator) *mapObj {
	o := newMapObj("vars.global", vals)
	o.members["set"] = globalSetDef(o, mut).Value()
	o.members["delete"] = globalDeleteDef(o, mut).Value()
	return o
}

func globalSetDef(o *mapObj, mut GlobalMutator) native.Def {
	sig := "vars.global.set(name, value[, secret])"
	return native.Fn2Optional("vars.global.set", sig, nameArg, native.String, native.Bool,
		func(call native.Call, name, value string, secret native.Optional[bool]) (rts.Value, error) {
			if mut == nil {
				return rts.Null(), call.Errorf("vars.global is read-only")
			}
			mut.SetGlobal(name, value, secret.Set && secret.Value)
			o.vals.Set(name, value)
			return rts.Null(), nil
		},
	)
}

func globalDeleteDef(o *mapObj, mut GlobalMutator) native.Def {
	sig := "vars.global.delete(name)"
	return native.Fn1("vars.global.delete", sig, nameArg,
		func(call native.Call, name string) (rts.Value, error) {
			if mut == nil {
				return rts.Null(), call.Errorf("vars.global is read-only")
			}
			mut.DelGlobal(name)
			o.vals.Delete(name)
			return rts.Null(), nil
		},
	)
}
