package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

func NewFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func NewSubcommandFlagSet(app, name string, w io.Writer) *flag.FlagSet {
	if w == nil {
		w = os.Stderr
	}
	fs := NewFlagSet(name)
	fs.Usage = func() {
		PrintFlagSetUsage(w, app, fs)
	}
	return fs
}

func PrintFlagSetUsage(w io.Writer, app string, fs *flag.FlagSet) {
	if w == nil || fs == nil {
		return
	}
	if _, err := fmt.Fprintf(w, "Usage: %s %s [flags]\n", app, fs.Name()); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "Flags:"); err != nil {
		return
	}
	out := fs.Output()
	fs.SetOutput(w)
	fs.PrintDefaults()
	fs.SetOutput(out)
}

type trimmedStringValue struct {
	dst *string
}

func (v trimmedStringValue) String() string {
	if v.dst == nil {
		return ""
	}
	return *v.dst
}

func (v trimmedStringValue) Set(s string) error {
	if v.dst != nil {
		*v.dst = str.Trim(s)
	}
	return nil
}

func StringVar(fs *flag.FlagSet, dst *string, name, value, usage string) {
	if fs == nil || dst == nil {
		return
	}
	*dst = str.Trim(value)
	fs.Var(trimmedStringValue{dst: dst}, name, usage)
}
