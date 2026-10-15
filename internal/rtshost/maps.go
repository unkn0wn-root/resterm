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
	evalObj
	vals    vars.NameMap[string]
	resolve vars.Lookup
	members map[string]rts.Value
}

func newMapObj(st *evalState, name string, vals vars.NameView[string]) *mapObj {
	o := &mapObj{evalObj: evalObj{st, name}, vals: vals.Clone()}
	o.members = map[string]rts.Value{
		"get":     o.method(o.getDef()),
		"has":     o.method(o.hasDef()),
		"require": o.method(o.requireDef()),
	}
	return o
}

func (o *mapObj) value(name string) (string, bool, error) {
	if o.resolve != nil {
		if v, ok, err := o.resolve(name); ok || err != nil {
			return v, ok, err
		}
	}
	v, ok := o.vals.Get(name)
	return v, ok, nil
}

func (o *mapObj) Member(ctx *rts.Ctx, pos rts.Pos, name string) (rts.Value, bool, error) {
	if v, ok := o.members[name]; ok {
		return v, true, nil
	}
	v, ok, err := o.value(name)
	if err != nil || !ok {
		return rts.Null(), ok, err
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
	v, ok, err := o.value(name)
	if err != nil || !ok {
		return rts.Null(), err
	}
	return native.StringValue(ctx, pos, v)
}

func (o *mapObj) getDef() native.Def {
	sig := o.name + ".get(name)"
	return native.Fn1(o.name+".get", sig, nameArg,
		func(call native.Call, name string) (rts.Value, error) {
			v, ok, err := o.value(name)
			if err != nil || !ok {
				return rts.Null(), err
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
			v, ok, err := o.value(name)
			if err != nil {
				return rts.Null(), err
			}
			if ok && strings.TrimSpace(v) != "" {
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

func newEnvObj(st *evalState, scope Scope) *mapObj {
	o := newMapObj(st, "env", scope.Env)
	o.members["meta"] = rts.Obj(&envMetaObj{
		evalObj: evalObj{st, "env.meta"},
		meta:    scope.Meta,
		groups:  newMapObj(st, "env.meta.groups", scope.Meta.Groups),
	})
	return o
}

type envMetaObj struct {
	evalObj
	meta   EnvMeta
	groups *mapObj
}

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

func newVarsObj(st *evalState, scope Scope) *mapObj {
	o := newMapObj(st, "vars", scope.Vars)
	o.resolve = scope.Resolve
	o.members["set"] = o.method(varsSetDef(o))
	o.members["interpolate"] = o.method(varsInterpolateDef(o.value))
	o.members["global"] = rts.Obj(newGlobalObj(st, scope.Globals))
	return o
}

func varsInterpolateDef(look vars.Lookup) native.Def {
	sig := "vars.interpolate(text)"
	return native.Fn1("vars.interpolate", sig, native.String,
		func(call native.Call, text string) (rts.Value, error) {
			out, err := vars.Interpolate(text, func(name string) (string, bool, error) {
				if err := rts.Tick(call.Ctx, call.Pos); err != nil {
					return "", false, err
				}
				return look(name)
			}, vars.InterpolateOptions{MaxLen: call.Ctx.Lim.MaxStr})
			switch {
			case rts.IsAbort(err):
				return rts.Null(), err
			case err != nil:
				return rts.Null(), call.Errorf("vars.interpolate: %v", err)
			}
			return native.StringValue(call.Ctx, call.Pos, out)
		},
	)
}

func varsSetDef(o *mapObj) native.Def {
	sig := "vars.set(name, value)"
	return native.Fn2("vars.set", sig, nameArg, native.String,
		func(call native.Call, name, value string) (rts.Value, error) {
			mut, err := mutator(call, o.name)
			if err != nil {
				return rts.Null(), err
			}
			mut.SetVar(name, value)
			o.vals.Set(name, value)
			return rts.Null(), nil
		},
	)
}

func newGlobalObj(st *evalState, vals vars.NameView[string]) *mapObj {
	o := newMapObj(st, "vars.global", vals)
	o.members["set"] = o.method(globalSetDef(o))
	o.members["delete"] = o.method(globalDeleteDef(o))
	return o
}

func globalSetDef(o *mapObj) native.Def {
	sig := "vars.global.set(name, value[, secret])"
	return native.Fn2Optional("vars.global.set", sig, nameArg, native.String, native.Bool,
		func(call native.Call, name, value string, secret native.Optional[bool]) (rts.Value, error) {
			mut, err := mutator(call, o.name)
			if err != nil {
				return rts.Null(), err
			}
			mut.SetGlobal(name, value, secret.Set && secret.Value)
			o.vals.Set(name, value)
			return rts.Null(), nil
		},
	)
}

func globalDeleteDef(o *mapObj) native.Def {
	sig := "vars.global.delete(name)"
	return native.Fn1("vars.global.delete", sig, nameArg,
		func(call native.Call, name string) (rts.Value, error) {
			mut, err := mutator(call, o.name)
			if err != nil {
				return rts.Null(), err
			}
			mut.DelGlobal(name)
			o.vals.Delete(name)
			return rts.Null(), nil
		},
	)
}
