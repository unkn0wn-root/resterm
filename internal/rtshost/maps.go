package rtshost

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type mapObj struct {
	name string
	vals vars.NameMap[string]
}

func newMapObj(name string, vals vars.NameMap[string]) *mapObj {
	return &mapObj{name: name, vals: vals.Clone()}
}

func (o *mapObj) TypeName() string { return o.name }

func (o *mapObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	switch name {
	case "get":
		return o.getDef().Value(), true, nil
	case "has":
		return o.hasDef().Value(), true, nil
	case "require":
		return o.requireDef().Value(), true, nil
	}
	v, ok := o.vals.Get(name)
	if !ok {
		return rts.Null(), false, nil
	}
	out, err := native.StringValue(ctx, pos, v)
	return out, true, err
}

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

type envObj struct {
	*mapObj
	meta *envMetaObj
}

func newEnvObj(scope Scope) *envObj {
	return &envObj{
		mapObj: newMapObj("env", scope.Env),
		meta:   &envMetaObj{meta: scope.Meta, groups: newMapObj("env.meta.groups", scope.Meta.Groups)},
	}
}

func (o *envObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	if name == "meta" {
		return rts.Obj(o.meta), true, nil
	}
	return o.mapObj.Member(ctx, pos, name)
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

type varsObj struct {
	*mapObj
	glob *globalObj
	mut  VarsMutator
}

func newVarsObj(scope Scope, varsMut VarsMutator, globalMut GlobalMutator) *varsObj {
	return &varsObj{
		mapObj: newMapObj("vars", scope.Vars),
		glob: &globalObj{
			mapObj: newMapObj("vars.global", scope.Globals),
			mut:    globalMut,
		},
		mut: varsMut,
	}
}

func (o *varsObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	switch name {
	case "set":
		return o.setDef().Value(), true, nil
	case "global":
		return rts.Obj(o.glob), true, nil
	default:
		return o.mapObj.Member(ctx, pos, name)
	}
}

func (o *varsObj) setDef() native.Def {
	sig := "vars.set(name, value)"
	return native.Fn2("vars.set", sig, nameArg, native.String,
		func(call native.Call, name, value string) (rts.Value, error) {
			if o.mut == nil {
				return rts.Null(), call.Errorf("vars is read-only")
			}
			o.mut.SetVar(name, value)
			o.vals.Set(name, value)
			return rts.Null(), nil
		},
	)
}

type globalObj struct {
	*mapObj
	mut GlobalMutator
}

func (o *globalObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	switch name {
	case "set":
		return o.setDef().Value(), true, nil
	case "delete":
		return o.deleteDef().Value(), true, nil
	default:
		return o.mapObj.Member(ctx, pos, name)
	}
}

func (o *globalObj) setDef() native.Def {
	sig := "vars.global.set(name, value[, secret])"
	return native.Fn2Optional("vars.global.set", sig, nameArg, native.String, native.Bool,
		func(call native.Call, name, value string, secret native.Optional[bool]) (rts.Value, error) {
			if o.mut == nil {
				return rts.Null(), call.Errorf("vars.global is read-only")
			}
			o.mut.SetGlobal(name, value, secret.Set && secret.Value)
			o.vals.Set(name, value)
			return rts.Null(), nil
		},
	)
}

func (o *globalObj) deleteDef() native.Def {
	sig := "vars.global.delete(name)"
	return native.Fn1("vars.global.delete", sig, nameArg,
		func(call native.Call, name string) (rts.Value, error) {
			if o.mut == nil {
				return rts.Null(), call.Errorf("vars.global is read-only")
			}
			o.mut.DelGlobal(name)
			o.vals.Delete(name)
			return rts.Null(), nil
		},
	)
}
