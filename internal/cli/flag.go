package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

const aliasUsagePrefix = "Alias for --"

// ErrHelp is returned when Parse sees -h or --help.
var ErrHelp = flag.ErrHelp

// FlagSet accepts flags before or after positional arguments.
// String flags are trimmed.
type FlagSet struct {
	*flag.FlagSet
	args []string
}

type stringValue struct {
	dst *string
}

func (v stringValue) String() string {
	if v.dst == nil {
		return ""
	}
	return *v.dst
}

func (v stringValue) Set(s string) error {
	*v.dst = str.Trim(s)
	return nil
}

type stringListValue struct {
	dst *[]string
}

func (v stringListValue) String() string {
	if v.dst == nil {
		return ""
	}
	return strings.Join(*v.dst, ",")
}

func (v stringListValue) Set(s string) error {
	*v.dst = append(*v.dst, str.Trim(s))
	return nil
}

func NewFlagSet(name string) *FlagSet {
	fs := &FlagSet{FlagSet: flag.NewFlagSet(name, flag.ContinueOnError)}
	fs.SetOutput(io.Discard)
	return fs
}

func NewSubcommandFlagSet(app, name string, w io.Writer) *FlagSet {
	fs := NewFlagSet(name)
	fs.Usage = func() {
		PrintFlagSetUsage(w, app, fs)
	}
	return fs
}

// Parse accepts flags before or after positional arguments until "--".
func (f *FlagSet) Parse(args []string) error {
	flags, positional := f.split(args)
	f.args = positional
	return f.FlagSet.Parse(flags)
}

// Args returns the positional arguments in their original order.
func (f *FlagSet) Args() []string {
	return f.args
}

func (f *FlagSet) Arg(i int) string {
	if i < 0 || i >= len(f.args) {
		return ""
	}
	return f.args[i]
}

func (f *FlagSet) NArg() int {
	return len(f.args)
}

// UnexpectedArgs returns an error if the command received positional arguments.
func (f *FlagSet) UnexpectedArgs() error {
	if len(f.args) == 0 {
		return nil
	}
	return fmt.Errorf("%s: unexpected args: %s", f.Name(), strings.Join(f.args, " "))
}

func (f *FlagSet) split(args []string) (flags, positional []string) {
	flags = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return flags, append(positional, args[i+1:]...)
		}

		name, inline, ok := flagArg(arg)
		if !ok {
			positional = append(positional, arg)
			continue
		}

		flags = append(flags, arg)
		if inline || i+1 >= len(args) {
			continue
		}
		// A non-boolean flag consumes the next argument, even if it starts with "-".
		if def := f.Lookup(name); def != nil && !isBoolFlag(def.Value) {
			i++
			flags = append(flags, args[i])
		}
	}
	return flags, positional
}

func flagArg(arg string) (name string, inline, ok bool) {
	if len(arg) < 2 || arg[0] != '-' {
		return "", false, false
	}

	name = strings.TrimPrefix(arg[1:], "-")
	name, _, inline = strings.Cut(name, "=")
	return name, inline, true
}

func isBoolFlag(value flag.Value) bool {
	b, ok := value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

func (f *FlagSet) StringVar(dst *string, name, value, usage string) {
	*dst = str.Trim(value)
	f.Var(stringValue{dst: dst}, name, usage)
}

func (f *FlagSet) StringVarAliases(dst *string, value, usage string, names ...string) {
	registerAliases(names, usage, func(name, usage string) {
		f.StringVar(dst, name, value, usage)
	})
}

func (f *FlagSet) StringListVarAliases(dst *[]string, usage string, names ...string) {
	registerAliases(names, usage, func(name, usage string) {
		f.Var(stringListValue{dst: dst}, name, usage)
	})
}

func (f *FlagSet) BoolVarAliases(dst *bool, value bool, usage string, names ...string) {
	registerAliases(names, usage, func(name, usage string) {
		f.BoolVar(dst, name, value, usage)
	})
}

func (f *FlagSet) IntVarAliases(dst *int, value int, usage string, names ...string) {
	registerAliases(names, usage, func(name, usage string) {
		f.IntVar(dst, name, value, usage)
	})
}

func (f *FlagSet) DurationVarAliases(dst *time.Duration, value time.Duration, usage string, names ...string) {
	registerAliases(names, usage, func(name, usage string) {
		f.DurationVar(dst, name, value, usage)
	})
}

// The first name is shown in help. Later names are marked as aliases so help can combine them.
func registerAliases(names []string, usage string, bind func(name, usage string)) {
	for i, name := range names {
		flagUsage := usage
		if i > 0 {
			flagUsage = aliasUsagePrefix + names[0]
		}
		bind(name, flagUsage)
	}
}
