package main

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/skills"
)

// Each agent reads skills from $env/skills, or from ~/home/skills when $env is empty.
var skillAgents = map[string]struct{ env, home string }{
	"claude": {env: "CLAUDE_CONFIG_DIR", home: ".claude"},
	"codex":  {home: ".agents"},
}

var skillAgentNames = strings.Join(slices.Sorted(maps.Keys(skillAgents)), ", ")

func handleSkillSubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "skill" {
		return false, nil
	}
	if len(args) == 1 && cli.HasFileConflict("skill") {
		return true, cli.CommandFileConflict(
			"resterm",
			"skill",
			"pass `resterm skill --` to print the skill",
		)
	}
	return true, runSkill(args[1:])
}

func runSkill(args []string) error {
	var (
		dir    string
		agents []string
	)
	fs := newSkillFlags(&dir, &agents)
	if len(args) > 0 && cli.IsHelpArg(args[0]) {
		printSkillUsage(os.Stdout, fs)
		return nil
	}
	if done, err := parseSubcommand(fs, args); done || err != nil {
		return err
	}

	if !fs.WasSet("dir") && len(agents) == 0 {
		data, err := skills.FS.ReadFile(path.Join(skills.Name, "SKILL.md"))
		if err != nil {
			return err
		}
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("skill: write output: %w", err)
		}
		return nil
	}

	var dirs []string
	if fs.WasSet("dir") {
		if len(agents) > 0 {
			return cli.UsageError(errors.New("skill: use --agent or --dir, not both"))
		}
		if dir == "" {
			return cli.UsageError(errors.New("skill: --dir needs a directory"))
		}
		dirs = append(dirs, dir)
	}
	for _, name := range agents {
		if _, ok := skillAgents[name]; !ok {
			return cli.UsageError(
				fmt.Errorf("skill: unknown agent %q. Use %s, or --dir for other agents", name, skillAgentNames),
			)
		}
	}
	for _, name := range agents {
		agent := skillAgents[name]
		var base string
		if agent.env != "" {
			base = os.Getenv(agent.env)
		}
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("skill: %w", err)
			}
			base = filepath.Join(home, agent.home)
		}
		dirs = append(dirs, filepath.Join(base, "skills"))
	}

	var errs []error
	for _, d := range dirs {
		dst, err := skills.Install(d)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := writef(os.Stdout, "Installed the resterm skill in %s\n", dst); err != nil {
			return fmt.Errorf("skill: write output: %w", err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("skill: %w", err)
	}
	return nil
}

func newSkillFlags(dir *string, agents *[]string) *cli.FlagSet {
	fs := cli.NewFlagSet("skill")
	fs.StringVar(dir, "dir", "", "Skills directory to install the skill into")
	fs.StringListVarAliases(agents, "Agent to install for: "+skillAgentNames+" (repeatable)", "agent")
	fs.Usage = func() { printSkillUsage(os.Stderr, fs) }
	return fs
}

func printSkillUsage(w io.Writer, fs *cli.FlagSet) {
	_, _ = fmt.Fprintln(w, "Usage: resterm skill [--agent <name>... | --dir <dir>]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Print the resterm skill for coding agents.")
	_, _ = fmt.Fprintln(
		w,
		"With --agent, install it where that agent reads skills. With --dir, install it in <dir>/resterm.",
	)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Flags:")
	cli.PrintFlagDefaults(w, fs)
}
