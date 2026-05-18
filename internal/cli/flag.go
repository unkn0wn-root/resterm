package cli

import (
	"flag"
	"io"
	"time"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

const aliasUsagePrefix = "Alias for --"

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

func NewFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func NewSubcommandFlagSet(app, name string, w io.Writer) *flag.FlagSet {
	fs := NewFlagSet(name)
	fs.Usage = func() {
		PrintFlagSetUsage(w, app, fs)
	}
	return fs
}

func StringVar(fs *flag.FlagSet, dst *string, name, value, usage string) {
	*dst = str.Trim(value)
	fs.Var(stringValue{dst: dst}, name, usage)
}

func StringVarAliases(fs *flag.FlagSet, dst *string, value, usage string, names ...string) {
	registerAliases(names, usage, func(name, usage string) {
		StringVar(fs, dst, name, value, usage)
	})
}

func BoolVarAliases(fs *flag.FlagSet, dst *bool, value bool, usage string, names ...string) {
	registerAliases(names, usage, func(name, usage string) {
		fs.BoolVar(dst, name, value, usage)
	})
}

func IntVarAliases(fs *flag.FlagSet, dst *int, value int, usage string, names ...string) {
	registerAliases(names, usage, func(name, usage string) {
		fs.IntVar(dst, name, value, usage)
	})
}

func DurationVarAliases(
	fs *flag.FlagSet,
	dst *time.Duration,
	value time.Duration,
	usage string,
	names ...string,
) {
	registerAliases(names, usage, func(name, usage string) {
		fs.DurationVar(dst, name, value, usage)
	})
}

// registerAliases binds names[0] as the canonical flag and every later name
// as an alias; the alias usage string is what PrintFlagDefaults folds back
// into the canonical flag's row.
func registerAliases(names []string, usage string, bind func(name, usage string)) {
	for i, name := range names {
		flagUsage := usage
		if i > 0 {
			flagUsage = aliasUsagePrefix + names[0]
		}
		bind(name, flagUsage)
	}
}
