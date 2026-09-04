package intellisense

import "github.com/unkn0wn-root/resterm/internal/directive"

var directives = directiveItems()

func directiveItems() []Item {
	specs := directive.Specs()
	items := make([]Item, len(specs))
	for i, spec := range specs {
		aliases := make([]string, len(spec.Aliases))
		for j, alias := range spec.Aliases {
			aliases[j] = alias.Tag()
		}
		items[i] = Item{
			Label:           spec.Name.Tag(),
			Aliases:         aliases,
			Summary:         spec.Summary,
			Continue:        !argsFor(spec.Name).empty(),
			noTrailingSpace: spec.Args == directive.ArgNone,
		}
		if spec.Name == directive.RTS {
			items[i].Insert = directive.RTS.Tag() + " pre-request"
		}
	}
	return items
}

// @query and @variables require "<" before a file path.
var loadMarker = Item{Label: "<", Summary: "Load from a file", Continue: true}

type directiveSource struct{}

func (directiveSource) Provide(ctx Context, sc Scope) []Item {
	switch ctx.Kind {
	case KindDirective:
		return directiveNameItems(ctx)
	case KindDirectiveArg:
		if ctx.arg != nil {
			return valueItems(ctx.arg, ctx, sc)
		}
		return argumentItems(ctx, sc)
	default:
		return nil
	}
}

func directiveNameItems(ctx Context) []Item {
	items := filter(directives, ctx.Query)
	if !ctx.bare {
		return items
	}
	for i, item := range items {
		items[i] = item.withInsertPrefix(directive.CommentPrefix)
	}
	return items
}

func argumentItems(ctx Context, sc Scope) []Item {
	table := argsFor(ctx.name)
	if table.empty() {
		return nil
	}
	items := make([]Item, 0, len(table.named)+1)
	for i := range table.named {
		arg := &table.named[i]
		if !arg.repeat && ctx.completed.has(arg.key) {
			continue
		}
		items = append(items, arg.items()...)
	}
	if v := table.value; v != nil {
		switch {
		case v.value.kind == valuePath && v.value.path.form == pathLoadOnly:
			items = append(items, loadMarker)
		case v.value.kind != valuePath && (v.repeat || !ctx.completed.has(v.key)):
			items = append(items, valueItems(v, ctx, sc)...)
		}
	}
	if table.extraItems != nil {
		items = append(items, table.extraItems(ctx, sc)...)
	}
	return filter(items, ctx.Query)
}

func valueItems(arg *argument, ctx Context, sc Scope) []Item {
	var items []Item
	// The editor handles KindPath by reading the filesystem.
	switch arg.value.kind {
	case valueChoice:
		items = labeledItems(arg.value.choices, "value")
	case valueText:
		items = arg.exampleItems()
	case valueNames:
		items = labeledItems(arg.value.names(ctx, sc))
	}

	single := argsFor(ctx.name).single
	out := make([]Item, 0, len(items))
	for _, item := range filter(items, ctx.Query) {
		if ctx.completed.holds(arg.key, item.Label) {
			continue
		}
		if arg.value.kind == valueNames {
			item.Insert = directive.Quote(item.Label)
		}
		switch {
		case arg.repeat && arg.form != formWord:
			// Keep the caret next to the value for a following list separator.
			item = item.WithoutTrailingSpace()
		case !single && item.Placeholder == "":
			item.Continue = true
		}
		out = append(out, item)
	}
	return out
}

func labeledItems(names []string, summary string) []Item {
	out := make([]Item, len(names))
	for i, name := range names {
		out[i] = Item{Label: name, Summary: summary}
	}
	return out
}
