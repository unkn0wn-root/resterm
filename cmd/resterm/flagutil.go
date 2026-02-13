package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func newSubcommandFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	// Errors are formatted manually so each subcommand can keep a clear
	// and consistent prefix in user-facing output.
	fs.SetOutput(io.Discard)
	// Help output still goes to stderr so `-h` behaves like a normal CLI.
	fs.Usage = func() {
		printFlagSetUsage(os.Stderr, fs)
	}
	return fs
}

func printFlagSetUsage(w io.Writer, fs *flag.FlagSet) {
	if _, err := fmt.Fprintf(w, "Usage: resterm %s [flags]\n", fs.Name()); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
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
