package ui

import (
	"fmt"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// startupEnvStatus picks the first message worth showing when a session opens:
// a workspace listing failure, environment files that will never load, files
// discovery does not look for, or the environment picked by default.
func startupEnvStatus(entries []filesvc.FileEntry, ws workspace, fallback string, wsErr error) statusMsg {
	if wsErr != nil {
		return statusMsg{text: fmt.Sprintf("workspace error: %v", wsErr), level: statusWarn}
	}
	if s := inactiveEnvStatus(entries, ws.envFile, ws.recursive); s.text != "" {
		return s
	}
	if s := undiscoveredEnvStatus(ws); s.text != "" {
		return s
	}
	if fallback != "" {
		return statusMsg{text: fmt.Sprintf("Using environment: %s", fallback), level: statusInfo}
	}
	return statusMsg{}
}

// inactiveEnvStatus warns about environment files the navigator lists but never
// loads. One file is resolved per workspace, so a nested one stays inactive even
// when a request beside it is opened. Only a recursive workspace lists them.
func inactiveEnvStatus(entries []filesvc.FileEntry, active string, recursive bool) statusMsg {
	if !recursive {
		return statusMsg{}
	}

	var names []string
	for _, entry := range entries {
		if entry.Kind == filesvc.FileKindEnv && !util.SameFile(entry.Path, active) {
			names = append(names, entry.Name)
		}
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

// undiscoveredEnvStatus points at files that read as environment files but that
// discovery never looks for. A workspace holding only dev.env.json and
// prod.env.json loads no environment at all and otherwise gives no hint why.
// The walk only happens when nothing was discovered, so the common case pays
// nothing.
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
	return statusMsg{level: statusWarn, text: "No environment file was discovered. " + load}
}

// envLookalikes scans for files named like environment files. The entry list a
// session already holds keeps only request and environment files, and these are
// neither.
func envLookalikes(root string, recursive bool) []string {
	files, err := filesvc.ListWorkspaceFiles(root, recursive, filesvc.ListOptions{})
	if err != nil {
		return nil
	}

	var names []string
	for _, f := range files {
		if vars.LooksLikeEnvFile(f.Name) {
			names = append(names, f.Name)
		}
	}
	return names
}
