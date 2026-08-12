package rts

type ModObj struct {
	name string
	exp  map[string]Value
}

func NewModObj(name string, exp map[string]Value) *ModObj {
	return &ModObj{name: name, exp: CloneDict(exp)}
}

func (m *ModObj) TypeName() string { return "module:" + m.name }

func (m *ModObj) Member(_ *Ctx, _ Pos, name string) (Value, bool, error) {
	v, ok := m.exp[name]
	return v, ok, nil
}

func (m *ModObj) Index(_ *Ctx, pos Pos, key Value) (Value, error) {
	k, err := Key(pos, key)
	if err != nil {
		return Null(), err
	}

	v, ok := m.exp[k]
	if !ok {
		return Null(), nil
	}
	return v, nil
}
