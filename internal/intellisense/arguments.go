package intellisense

import (
	"slices"
	"strings"
)

type argForm uint8

const (
	formWord   argForm = iota // bare word, followed by a value if required
	formOption                // key=value
	formBudget                // key<=value, the @trace latency budgets
)

type pathForm uint8

const (
	// pathWord is one argument, quoted if it contains spaces.
	pathWord pathForm = iota
	// pathLoad uses the rest of the line, with an optional "<" marker.
	pathLoad
	// pathLoadOnly requires "<"; without it, the value is inline content.
	pathLoadOnly
)

type valueKind uint8

const (
	valueNone   valueKind = iota // a bare flag with no value
	valueText                    // free-form, so only the registered example is offered
	valueChoice                  // one of a fixed set
	valueNames                   // names supplied by the current workspace scope
	valuePath
)

type nameSource func(Context, Scope) ([]string, string)

type pathSpec struct {
	kind PathKind
	form pathForm
	list bool
	home bool // expand a leading "~"
}

type value struct {
	kind    valueKind
	choices []string
	names   nameSource
	path    pathSpec
}

// argument describes an option, a bare word, or a directive's positional value.
type argument struct {
	key     string
	aliases []string
	summary string
	form    argForm
	value   value

	repeat bool
	chain  bool // accepting the word opens suggestions for the next argument

	examples []argExample
}

type argExample struct {
	text        string
	placeholder string // text selected for replacement when the example is inserted
	summary     string // overrides the argument's description
	call        string
}

type args struct {
	// value is positional, such as the module path in @use.
	value *argument
	named []argument
	// single allows one setting per line, as "key value" or "key=value".
	single bool
	// extraItems adds shortcuts that use the same filtering as other suggestions.
	extraItems func(Context, Scope) []Item
}

func (a args) empty() bool {
	return a.value == nil && len(a.named) == 0 && a.extraItems == nil
}

func (a args) find(pred func(argument) bool) *argument {
	if i := slices.IndexFunc(a.named, pred); i >= 0 {
		return &a.named[i]
	}
	return nil
}

func (a args) findOption(key string, form argForm) *argument {
	return a.find(func(arg argument) bool {
		return arg.form == form && arg.matches(key)
	})
}

// findWord also accepts option names for directives that allow "key value".
func (a args) findWord(text string) *argument {
	return a.find(func(arg argument) bool {
		return arg.form != formBudget && (arg.form == formWord || a.single) && arg.matches(text)
	})
}

func (a argument) matches(key string) bool {
	if strings.EqualFold(a.key, key) {
		return true
	}
	return slices.ContainsFunc(a.aliases, func(alias string) bool {
		return strings.EqualFold(alias, key)
	})
}

func (a argument) takesValue() bool {
	return a.value.kind != valueNone
}

func (a argument) loadsLine() bool {
	return a.value.kind == valuePath && a.value.path.form != pathWord
}

func word(key, summary string) argument {
	return argument{key: key, summary: summary, form: formWord}
}

func opt(key, summary, example string) argument {
	return argument{
		key:      key,
		summary:  summary,
		form:     formOption,
		value:    value{kind: valueText},
		examples: []argExample{plainExample(example)},
	}
}

func optValue(key, summary string, v value) argument {
	return argument{key: key, summary: summary, form: formOption, value: v}
}

func choice(key, summary string, choices ...string) argument {
	return optValue(key, summary, value{kind: valueChoice, choices: choices})
}

func flag(key, summary string) argument {
	return choice(key, summary, "true", "false")
}

// toggle uses "key true" or "key false" instead of "key=value".
func toggle(key, summary string) argument {
	a := flag(key, summary)
	a.form = formWord
	return a
}

func filePath(key, summary string, kind PathKind, form pathForm) argument {
	return optValue(key, summary, pathValue(kind, form))
}

func budget(key, summary, example string) argument {
	a := opt(key, summary, example)
	a.form = formBudget
	return a
}

func directiveValue(a argument) *argument {
	a.form = formWord
	return &a
}

func (a argument) alias(names ...string) argument {
	a.aliases = append(a.aliases, names...)
	return a
}

func (a argument) withExample(text string) argument {
	a.examples = []argExample{plainExample(text)}
	return a
}

// withOption selects only the option value, as in "@auth command argv=[...]".
func (a argument) withOption(option string) argument {
	a.examples = []argExample{{text: option, placeholder: optionValue(option)}}
	return a
}

func (a argument) call(usage string) argument {
	name, argv := cutCall(usage)
	a.examples = []argExample{{text: usage, placeholder: argv, call: name}}
	return a
}

func (a argument) chains() argument {
	a.chain = true
	return a
}

func (a argument) repeats() argument {
	a.repeat = true
	return a
}

func (a argument) home() argument {
	a.value.path.home = true
	return a
}

func (a argument) list() argument {
	a.value.path.list = true
	return a
}

func (a argument) takes(v value) argument {
	a.value = v
	return a
}

func plainExample(text string) argExample {
	return argExample{text: text, placeholder: text}
}

func namesValue(source nameSource) value {
	return value{kind: valueNames, names: source}
}

func pathValue(kind PathKind, form pathForm) value {
	return value{kind: valuePath, path: pathSpec{kind: kind, form: form}}
}
