package intellisense

type memberSource struct{}

func (memberSource) Provide(ctx Context, _ Scope) []Item {
	if ctx.Kind != KindMember {
		return nil
	}
	items := make([]Item, 0, len(ctx.members))
	for _, m := range ctx.members {
		it := Item{Label: m.Name, Summary: m.Summary}
		switch {
		case ctx.call:
			// Keep the existing arguments or member access.
		case m.Object:
			it.Insert, it.Continue = m.Name+".", true
		default:
			it.Insert, it.Placeholder = m.Name+"("+m.Args+")", m.Args
		}
		items = append(items, it)
	}
	return filter(items, ctx.Query)
}
