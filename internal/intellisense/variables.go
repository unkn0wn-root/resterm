package intellisense

import "github.com/unkn0wn-root/resterm/internal/vars/dynamic"

// builtinVars mirrors the helper registry, so a new helper needs no second
// list here.
var builtinVars = builtinItems()

func builtinItems() []Item {
	helpers := dynamic.Helpers()
	items := make([]Item, 0, len(helpers))
	for _, h := range helpers {
		items = append(items, Item{
			Label:   h.Name(),
			Aliases: h.Aliases(),
			Summary: helperSummary(h),
		})
	}
	return items
}

// helperSummary shows the call form, since the label alone does not say a
// helper takes arguments.
func helperSummary(h dynamic.Descriptor) string {
	if h.Usage() == "" {
		return h.Summary()
	}
	return h.Summary() + ", e.g. " + h.Usage()
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
