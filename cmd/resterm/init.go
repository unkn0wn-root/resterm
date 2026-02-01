package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/initcmd"
)

func handleInitSubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "init" {
		return false, nil
	}
	if len(args) == 1 && initTargetExists() {
		return true, fmt.Errorf(
			"init: found file named \"init\" in the current directory; use `resterm -- init` or `resterm ./init` to open it, or pass a flag like `resterm init --dir .` to run init",
		)
	}
	return true, runInit(args[1:])
}

func initTargetExists() bool {
	info, err := os.Stat("init")
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: resterm init [flags] [dir]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Templates:")
		_ = initcmd.Run(initcmd.Opt{List: true, Out: os.Stderr})
	}

	var (
		dir   string
		tpl   string
		force bool
		dry   bool
		list  bool
		noGi  bool
	)

	fs.StringVar(&dir, "dir", initcmd.DefaultDir, "Target directory")
	fs.StringVar(&tpl, "template", initcmd.DefaultTemplate, "Template to use")
	fs.BoolVar(&force, "force", false, "Overwrite existing files")
	fs.BoolVar(&dry, "dry-run", false, "Print actions without writing files")
	fs.BoolVar(&list, "list", false, "List available templates")
	fs.BoolVar(&noGi, "no-gitignore", false, "Do not touch .gitignore")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if list {
		return initcmd.Run(initcmd.Opt{List: true, Out: os.Stdout})
	}

	extra := fs.Args()
	if len(extra) > 0 {
		if dir == initcmd.DefaultDir && len(extra) == 1 {
			dir = extra[0]
		} else {
			return fmt.Errorf("init: unexpected args: %s", strings.Join(extra, " "))
		}
	}

	op := initcmd.Opt{
		Dir:         dir,
		Template:    tpl,
		Force:       force,
		DryRun:      dry,
		NoGitignore: noGi,
		Out:         os.Stdout,
	}
	return initcmd.Run(op)
}
