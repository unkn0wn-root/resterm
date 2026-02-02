package initcmd

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Command runs the init command with injectable dependencies.
type Command struct {
	fs        FS
	templates TemplateStore
	out       io.Writer
}

func New() *Command {
	return &Command{
		fs:        OSFS{},
		templates: BuiltinTemplates{},
		out:       os.Stdout,
	}
}

// Run keeps the existing API.
func Run(o Opt) error {
	return New().Run(o)
}

func (c *Command) Run(o Opt) error {
	o = withDefaults(o)
	if o.Out == nil {
		o.Out = c.out
	}

	if o.List {
		return c.listTemplates(o.Out)
	}

	tpl, ok := c.templates.Find(o.Template)
	if !ok {
		return unknownTemplateErr(c.templates, o.Template)
	}

	r := runner{fs: c.fs, o: o, t: tpl}
	return r.run()
}

func (c *Command) listTemplates(w io.Writer) error {
	tpls := c.templates.List()
	width := c.templates.Width()
	for _, t := range tpls {
		if _, err := fmt.Fprintf(w, "%-*s  %s\n", width, t.Name, t.Description); err != nil {
			return fmt.Errorf("init: list templates: %w", err)
		}
	}
	return nil
}

func unknownTemplateErr(tpls TemplateStore, name string) error {
	if name == "" {
		name = "(empty)"
	}
	return fmt.Errorf(
		"init: unknown template %q (available: %s)",
		name,
		strings.Join(tpls.Names(), ", "),
	)
}
