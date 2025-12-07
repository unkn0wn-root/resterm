package hint

// Source provides hints for a given context.
type Source interface {
	Match(Context) bool
	Options(Context) []Hint
}

// Manager aggregates hint sources.
type Manager struct {
	sources []Source
}

func NewManager(sources ...Source) Manager {
	return Manager{sources: sources}
}

func (m Manager) Options(ctx Context) []Hint {
	for _, src := range m.sources {
		if src.Match(ctx) {
			return src.Options(ctx)
		}
	}
	return nil
}
