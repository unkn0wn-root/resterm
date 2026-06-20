package intellisense

type source interface {
	Provide(ctx Context, sc Scope) []Item
}

type Engine struct {
	sources []source
}

func New() Engine {
	return Engine{sources: []source{
		directiveSource{},
		methodSource{},
		schemeSource{},
		headerSource{},
		variableSource{},
	}}
}

func (e Engine) Suggest(ctx Context, sc Scope) []Item {
	if ctx.Kind == KindNone {
		return nil
	}
	for _, s := range e.sources {
		if items := s.Provide(ctx, sc); items != nil {
			return items
		}
	}
	return nil
}
