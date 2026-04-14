package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
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
