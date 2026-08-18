package ui

import (
	"fmt"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// startupEnvStatus picks the first message worth showing when a session opens.
func startupEnvStatus(entries []files.Entry, ws workspace, fallback string, wsErr error) statusMsg {
	if wsErr != nil {
		return statusMsg{text: fmt.Sprintf("workspace error: %v", wsErr), level: statusWarn}
	}
	if s := envFileWarning(entries, ws); s.text != "" {
		return s
	}
	if fallback != "" {
		return statusMsg{text: fmt.Sprintf("Using environment: %s", fallback), level: statusInfo}
	}
	return statusMsg{}
}

// envFileWarning surfaces environment files that will not take effect. Startup
// and workspace moves share it, so both warn the same way.
func envFileWarning(entries []files.Entry, ws workspace) statusMsg {
	if s := inactiveEnvStatus(entries, ws.envFile, ws.recursive); s.text != "" {
		return s
	}
	return undiscoveredEnvStatus(ws)
}

// inactiveEnvStatus warns about environment files the navigator lists but
// never loads, because one file is resolved per workspace.
func inactiveEnvStatus(entries []files.Entry, active string, recursive bool) statusMsg {
	if !recursive {
		return statusMsg{}
	}

	var names []string
	for _, entry := range entries {
		if entry.Kind != files.KindEnv || util.SameFile(entry.Path, active) {
			continue
		}
		// The private companion of the active public file is loaded as part of
		// that file, so it is not a separate inactive environment file.
		if active != "" && vars.IsPrivateEnvFileName(entry.Path) &&
			sameDir(entry.Path, active) {
			continue
		}
		names = append(names, entry.Name)
	}
	if len(names) == 0 {
		return statusMsg{}
	}

	subject := fmt.Sprintf("%d environment files are inactive", len(names))
	if len(names) == 1 {
		subject = names[0] + " is inactive"
	}
	using := "No environment file was loaded"
	if active != "" {
		using = "This workspace uses " + filepath.Base(active)
	}
	return statusMsg{level: statusWarn, text: subject + ". " + using}
}

// undiscoveredEnvStatus points at files that read as environment files but
// that discovery never opens. Without it a workspace holding only dev.env.json
// loads nothing and gives no hint why.
func undiscoveredEnvStatus(ws workspace) statusMsg {
	if ws.envFile != "" || !ws.cat.Empty() {
		return statusMsg{}
	}

	names := envLookalikes(ws.root, ws.recursive)
	if len(names) == 0 {
		return statusMsg{}
	}

	load := fmt.Sprintf("Load one of %d *%s files with --env-file", len(names), vars.EnvJSONSuffix)
	if len(names) == 1 {
		load = "Load " + names[0] + " with --env-file"
	}
	return statusMsg{level: statusWarn, text: load + ". No environment file was discovered."}
}

// envLookalikes walks again because the session's entry list keeps only
// request and environment files, and these are neither.
func envLookalikes(root string, recursive bool) []string {
	entries, err := files.ListWorkspace(root, files.ListOptions{Recursive: recursive})
	if err != nil {
		return nil
	}

	var names []string
	for _, f := range entries {
		if vars.LooksLikeEnvFile(f.Name) {
			names = append(names, f.Name)
		}
	}
	return names
}

// sameDir reports whether a and b live in the same directory.
func sameDir(a, b string) bool {
	return filepath.Dir(a) == filepath.Dir(b)
}
