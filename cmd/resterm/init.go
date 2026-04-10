package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/initcmd"
)

func handleInitSubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "init" {
		return false, nil
	}
	if len(args) == 1 && cli.HasFileConflict("init") {
		return true, cli.CommandFileConflict(
			"resterm",
			"init",
			"pass a flag like `resterm init --dir .` to run init",
		)
	}
	return true, runInit(args[1:])
}

func runInit(args []string) error {
	c := newInitCmd()
	if err := c.parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	return c.run()
}

type initCmd struct {
	fs    *flag.FlagSet
	dir   string
	tpl   string
	force bool
	dry   bool
	list  bool
	noGi  bool
}

func newInitCmd() *initCmd {
	c := &initCmd{}
	fs := cli.NewFlagSet("init")
	c.fs = fs
	c.bind()
	fs.Usage = c.usage
	return c
}

func (c *initCmd) bind() {
	c.fs.StringVar(&c.dir, "dir", initcmd.DefaultDir, "Target directory")
	c.fs.StringVar(&c.tpl, "template", initcmd.DefaultTemplate, "Template to use")
	c.fs.BoolVar(&c.force, "force", false, "Overwrite existing files")
	c.fs.BoolVar(&c.dry, "dry-run", false, "Print actions without writing files")
	c.fs.BoolVar(&c.list, "list", false, "List available templates")
	c.fs.BoolVar(&c.noGi, "no-gitignore", false, "Do not touch .gitignore")
}

func (c *initCmd) parse(args []string) error {
	return c.fs.Parse(args)
}

func (c *initCmd) run() error {
	extra := c.fs.Args()
	if c.list {
		if len(extra) > 0 {
			return fmt.Errorf("init: unexpected args: %s", strings.Join(extra, " "))
		}
		return initcmd.Run(initcmd.Opt{List: true, Out: os.Stdout})
	}
	if len(extra) > 0 {
		if c.dir == initcmd.DefaultDir && len(extra) == 1 {
			c.dir = extra[0]
		} else {
			return fmt.Errorf("init: unexpected args: %s", strings.Join(extra, " "))
		}
	}

	o := initcmd.Opt{
		Dir:         c.dir,
		Template:    c.tpl,
		Force:       c.force,
		DryRun:      c.dry,
		NoGitignore: c.noGi,
		Out:         os.Stdout,
	}
	return initcmd.Run(o)
}

func (c *initCmd) usage() {
	fmt.Fprintln(os.Stderr, "Usage: resterm init [flags] [dir]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	out := c.fs.Output()
	c.fs.SetOutput(os.Stderr)
	c.fs.PrintDefaults()
	c.fs.SetOutput(out)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Templates:")
	_ = initcmd.Run(initcmd.Opt{List: true, Out: os.Stderr})
}
