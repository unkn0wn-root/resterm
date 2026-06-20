package intellisense

var builtinVars = []Item{
	{Label: "$uuid", Aliases: []string{"$guid"}, Summary: "Random UUID v4"},
	{Label: "$timestamp", Summary: "Current Unix time in seconds"},
	{Label: "$timestampISO8601", Summary: "Current time, RFC3339 UTC"},
	{Label: "$timestampms", Summary: "Current Unix time in milliseconds"},
	{Label: "$randomInt", Summary: "Random integer"},
}

type variableSource struct{}

func (variableSource) Provide(ctx Context, sc Scope) []Item {
	if ctx.Kind != KindVariable {
		return nil
	}
	items := make([]Item, 0, len(sc.Variables)+len(builtinVars))
	for _, v := range sc.Variables {
		items = append(items, Item{Label: v.Name, Summary: varSummary(v)})
	}
	items = append(items, builtinVars...)
	return filter(items, ctx.Query)
}

func varSummary(v VarRef) string {
	if v.Secret {
		return v.Origin + " (secret)"
	}
	return v.Origin
}
