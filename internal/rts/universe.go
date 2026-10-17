package rts

// newUniverse returns the universe, the scope of the predeclared names in pre.
// It binds each of them as a constant.
func newUniverse(pre map[string]Value) *Env {
	uni := NewEnv(nil)
	for name, v := range pre {
		uni.DefConst(name, v)
	}
	return uni
}

// newTopEnv returns a new top-level scope inside uni, the one that a module's
// or an evaluation's own declarations go into. A declaration there may shadow
// a predeclared name. rts and its deprecated alias stdlib are bound here and
// not in uni, so a top-level declaration cannot hide them and rts.json still
// reaches a shadowed json.
func newTopEnv(uni *Env) *Env {
	env := NewEnv(uni)
	env.DefConst("rts", Obj(universeObj{name: "rts", uni: uni}))
	env.DefConst("stdlib", Obj(universeObj{name: "stdlib", uni: uni}))
	return env
}

// universeObj is the universe seen as an object. Its members are the
// predeclared names.
type universeObj struct {
	name string
	uni  *Env
}

func (o universeObj) TypeName() string { return o.name }

func (o universeObj) Member(_ *Ctx, _ Pos, name string) (Value, bool, error) {
	v, ok := o.uni.Get(name)
	return v, ok, nil
}

func (o universeObj) Index(_ *Ctx, pos Pos, key Value) (Value, error) {
	k, err := Key(pos, key)
	if err != nil {
		return Null(), err
	}
	v, _ := o.uni.Get(k)
	return v, nil
}
