package intellisense

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

// Bare directives complete only their names because "@name = value" defines a variable.
func analyzeDirective(cur []rune, from, col int, bare bool) (Context, bool) {
	at := skipSpace(cur, from)
	if at >= col || cur[at] != '@' {
		return Context{}, false
	}

	ctx, ok := analyzeDirectiveArea(cur[at+1:], col-at-1)
	if !ok || (bare && ctx.Kind != KindDirective) {
		return Context{}, false
	}
	// Convert offsets to the full line and include '@' when replacing the name.
	ctx.Start += at + 1
	ctx.End += at + 1
	if ctx.Kind == KindDirective {
		ctx.Start = at
	}
	ctx.bare = bare
	return ctx, true
}

func analyzeDirectiveArea(area []rune, caret int) (Context, bool) {
	caret = clamp(caret, 0, len(area))
	if len(area) == 0 {
		return Context{Kind: KindDirective}, true
	}

	// The parser accepts both "@auth:bea" and "@auth bea".
	sep := -1
	for i, r := range area {
		if directive.IsArgSep(r) {
			sep = i
			break
		}
		if !isQueryRune(r) {
			return Context{}, false
		}
	}
	if sep == -1 || caret <= sep {
		end := len(area)
		if sep >= 0 {
			end = sep
		}
		return Context{Kind: KindDirective, Query: string(area[:caret]), End: end}, true
	}
	if sep == 0 {
		return Context{}, false
	}

	name := directive.Name(strings.ToLower(string(area[:sep]))).Canonical()
	start := min(skipArgSep(area, sep), caret)
	ctx := analyzeArguments(name, area[start:], caret-start)
	ctx.Start += start
	ctx.End += start
	return ctx, true
}

func analyzeArguments(name directive.Name, text []rune, caret int) Context {
	caret = clamp(caret, 0, len(text))
	table := argsFor(name)
	ctx := Context{
		Kind:      KindDirectiveArg,
		name:      name,
		Start:     caret,
		End:       caret,
		completed: completed{},
	}

	fields := scanFields(text)
	if arg, start, ok := table.loadValue(fields, text); ok && caret >= start {
		ctx.Start, ctx.End = start, len(text)
		ctx.Query = string(text[start:caret])
		ctx.setValue(arg, table.single)
		return ctx
	}

	read := table.read(fields)
	cur := fieldAt(fields, caret)
	for i, s := range read.slots {
		if i != cur {
			ctx.record(s, fields[i])
		}
	}

	if cur < 0 {
		switch {
		case read.open != nil:
			ctx.setValue(read.open, table.single)
		case table.single && len(ctx.completed) > 0:
			ctx.Kind = KindNone
		case table.value != nil && !table.value.loadsLine() && !read.taken && !table.value.repeat:
			ctx.setValue(table.value, table.single)
		}
		return ctx
	}

	s, f := read.slots[cur], fields[cur]
	ctx.Start, ctx.End = f.start, f.end
	ctx.Query = decodedPrefix(text[f.start:caret])
	switch {
	case f.eq >= 0 && caret > f.eq:
		if s.arg == nil {
			return Context{Kind: KindNone}
		}
		ctx.Start = f.eq + 1
		ctx.Query = optionValue(ctx.Query)
		ctx.setValue(s.arg, table.single)
		if s.arg.repeat {
			ctx.narrowToSegment(s.arg, text, caret)
		}
	case s.value && f.eq < 0:
		ctx.setValue(s.arg, table.single)
	}
	if ctx.Kind == KindPath && ctx.Path.List {
		ctx.Path.Suffix = strings.TrimPrefix(decodedPrefix(text[ctx.Start:ctx.End]), ctx.Query)
	}
	return ctx
}

// fieldAt returns the field index at the caret, or -1 between fields.
func fieldAt(fields []field, caret int) int {
	for i, f := range fields {
		if caret >= f.start && caret <= f.end {
			return i
		}
	}
	return -1
}

func (ctx *Context) record(s slot, f field) {
	if s.arg == nil {
		return
	}
	switch {
	case s.value && f.eq >= 0:
		ctx.completed.add(s.arg.key, optionValue(f.text))
	case s.value, !s.arg.takesValue():
		ctx.completed.add(s.arg.key, f.text)
	}
}

func (ctx *Context) setValue(arg *argument, single bool) {
	ctx.arg = arg
	if arg.value.kind != valuePath {
		return
	}
	path := arg.value.path
	ctx.Kind = KindPath
	ctx.Path = PathContext{
		Kind:      path.kind,
		Quote:     path.form == pathWord,
		List:      path.list,
		Home:      path.home,
		Continues: path.form == pathWord && !single,
	}
}

// narrowToSegment completes only the list entry at the caret.
// Entries repeat the key, as in "@apply use=one,use=two".
func (ctx *Context) narrowToSegment(arg *argument, text []rune, caret int) {
	start := ctx.Start
	raw := string(text[start:ctx.End])
	cursor := len(string(text[start:caret]))
	offset := 0
	for part := range strings.SplitSeq(raw, ",") {
		end := offset + len(part)
		valueAt, value := cutSegmentKey(arg, part)
		if cursor < offset || cursor > end {
			if v := directive.TrimQuotes(strings.TrimSpace(value)); v != "" {
				ctx.completed.add(arg.key, v)
			}
			offset = end + 1
			continue
		}

		from := offset + valueAt
		if from > cursor {
			// The caret is still in the repeated key, before its value begins.
			from = offset
			ctx.arg = nil
		}
		ctx.Start = start + utf8.RuneCountInString(raw[:from])
		ctx.End = start + utf8.RuneCountInString(raw[:end])
		ctx.Query = decodedPrefix(text[ctx.Start:caret])
		offset = end + 1
	}
}

// cutSegmentKey removes a repeated key and returns the value's byte offset and text.
func cutSegmentKey(arg *argument, part string) (int, string) {
	value := strings.TrimLeftFunc(part, unicode.IsSpace)
	if key, rest, ok := strings.Cut(value, "="); ok && arg.matches(key) {
		value = rest
	}
	return len(part) - len(value), value
}

func optionValue(text string) string {
	_, value, _ := strings.Cut(text, "=")
	return value
}

// Match the decoded prefix so quotes do not affect suggestion filtering.
func decodedPrefix(raw []rune) string {
	var value string
	for field := range directive.ScanFields(string(raw)) {
		value = field.Value
	}
	return value
}

func skipArgSep(area []rune, start int) int {
	for start < len(area) && directive.IsArgSep(area[start]) {
		start++
	}
	return start
}
