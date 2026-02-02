package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	srcDirect = "direct"
	srcBrew   = "homebrew"
)

const (
	cmdDirect = "resterm --update"
	cmdBrew   = "brew upgrade resterm"
)

// installSource can be set at build time with:
// -ldflags "-X main.installSource=homebrew"
var installSource = ""

type envFn func(string) string

func installSrc() string {
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	return installSrcFor(runtime.GOOS, installSource, exe, os.Getenv)
}

func installSrcFor(goos, src, exe string, env envFn) string {
	s := normSrc(src)
	if s != "" {
		return s
	}
	if goos == "windows" {
		return srcDirect
	}
	exe = resolveExecPath(exe)
	if isBrewExe(goos, exe, env) {
		return srcBrew
	}
	return srcDirect
}

func normSrc(src string) string {
	s := strings.ToLower(strings.TrimSpace(src))
	if s == "" {
		return ""
	}
	switch s {
	case "brew", "homebrew":
		return srcBrew
	default:
		return s
	}
}

func updCmd(src string) string {
	if normSrc(src) == srcBrew {
		return cmdBrew
	}
	return cmdDirect
}

func updHint(cmd string) string {
	return fmt.Sprintf("Run `%s` to install.", cmd)
}

func updBlock(cmd string) string {
	return fmt.Sprintf("This resterm was installed via Homebrew. Use `%s`.", cmd)
}

func isBrewExe(goos, exe string, env envFn) bool {
	if exe == "" || goos == "windows" {
		return false
	}
	exe = filepath.Clean(exe)
	for _, dir := range brewDirs(goos, env) {
		if hasPathPrefix(exe, dir) {
			return true
		}
	}
	return false
}

func brewDirs(goos string, env envFn) []string {
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		out = append(out, filepath.Clean(p))
	}
	if env != nil {
		add(env("HOMEBREW_CELLAR"))
		pfx := strings.TrimSpace(env("HOMEBREW_PREFIX"))
		if pfx != "" {
			add(filepath.Join(pfx, "Cellar"))
			add(filepath.Join(pfx, "opt"))
		}
	}
	for _, p := range defBrewPfx(goos) {
		add(filepath.Join(p, "Cellar"))
		add(filepath.Join(p, "opt"))
	}
	return uniqPaths(out)
}

func defBrewPfx(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"/opt/homebrew", "/usr/local"}
	case "linux":
		return []string{"/home/linuxbrew/.linuxbrew", "/usr/local"}
	default:
		return nil
	}
}

func uniqPaths(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func hasPathPrefix(p, pre string) bool {
	if p == "" || pre == "" {
		return false
	}
	if p == pre {
		return true
	}
	sep := string(os.PathSeparator)
	if !strings.HasSuffix(pre, sep) {
		pre += sep
	}
	return strings.HasPrefix(p, pre)
}
