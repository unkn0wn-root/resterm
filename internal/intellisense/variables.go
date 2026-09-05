package intellisense

import "github.com/unkn0wn-root/resterm/internal/vars/dynamic"

var builtinVars = builtinItems()

func builtinItems() []Item {
	helpers := dynamic.Helpers()
	items := make([]Item, 0, len(helpers))
	for _, h := range helpers {
		item := Item{
			Label:   h.Name(),
			Aliases: h.Aliases(),
			Summary: helperSummary(h),
		}
		if usage := h.Usage(); usage != "" {
			item.Insert = usage
			_, item.Placeholder = cutCall(usage)
		}
		items = append(items, item)
	}
	return items
}

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
	items = filter(items, ctx.Query)
	for i := range items {
		if ctx.call {
			items[i].Insert, items[i].Placeholder = items[i].Label, ""
			continue
		}
		items[i].Insert = items[i].InsertText() + ctx.closing
	}
	return items
}

func varSummary(v VarRef) string {
	if v.Secret {
		return v.Origin + " (secret)"
	}
	return v.Origin
}
