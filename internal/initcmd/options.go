package initcmd

import (
	"io"
	"strings"
)

// Opt describes how the init command should run.
// Fields are plain values so callers can map flags directly.
type Opt struct {
	Dir         string
	Template    string
	Force       bool
	DryRun      bool
	NoGitignore bool
	List        bool
	Out         io.Writer
}

func withDefaults(opt Opt) Opt {
	opt.Dir = strings.TrimSpace(opt.Dir)
	if opt.Dir == "" {
		opt.Dir = DefaultDir
	}
	opt.Template = normalizeTemplateName(opt.Template)
	if opt.Template == "" {
		opt.Template = DefaultTemplate
	}
	return opt
}

func normalizeTemplateName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
