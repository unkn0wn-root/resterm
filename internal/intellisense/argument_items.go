package intellisense

import (
	"cmp"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

func (a argument) label(key string) string {
	return key + a.op.String()
}

func (a argument) lead(key string) string {
	if a.op == directive.OpNone {
		return key + " "
	}
	return a.label(key)
}

func (a argument) items() []Item {
	items := a.itemsNamed(a.key)
	for _, alias := range a.aliases {
		items = append(items, a.itemsNamed(alias)...)
	}
	return items
}

func (a argument) itemsNamed(key string) []Item {
	if len(a.examples) == 0 {
		return []Item{a.itemNamed(key, argExample{})}
	}
	items := make([]Item, len(a.examples))
	for i, example := range a.examples {
		items[i] = a.itemNamed(key, example)
	}
	return items
}

func (a argument) itemNamed(key string, example argExample) Item {
	it := Item{Label: a.label(key), Summary: cmp.Or(example.summary, a.summary), Placeholder: example.placeholder}
	if key != a.key {
		it.Summary = "Alias for " + a.key
	}
	switch {
	case example.text == "" && a.takesValue():
		it.Continue = true
		if a.op != directive.OpNone {
			it.Insert = it.Label
			it = it.WithoutTrailingSpace()
		}
	case example.text == "":
		it.Continue = a.chain
	case example.call != "" && a.op == directive.OpNone:
		// The call already includes the argument name.
		it.Insert = example.text
	default:
		if example.call != "" {
			it.Label += example.call
		}
		it.Insert = a.lead(key) + example.text
	}
	return it
}

func (a argument) exampleItems() []Item {
	var items []Item
	for _, example := range a.examples {
		items = append(items, Item{
			Label:       example.text,
			Summary:     cmp.Or(example.summary, a.summary),
			Placeholder: example.placeholder,
		})
	}
	return items
}
