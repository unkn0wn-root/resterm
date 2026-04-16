package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func defaultTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return 30 * time.Second
}

func resolveRequestTimeout(req *restfile.Request, base time.Duration) time.Duration {
	if req != nil {
		if raw, ok := req.Settings["timeout"]; ok {
			if dur, err := time.ParseDuration(raw); err == nil && dur > 0 {
				return dur
			}
		}
	}
	return base
}

func (m *Model) buildResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	extraVals map[string]rts.Value,
	extras ...map[string]string,
) *vars.Resolver {
	return m.buildResolverWithGlobals(ctx, doc, req, envName, base, extraVals, nil, extras...)
}

func (m *Model) buildResolverWithGlobals(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	extraVals map[string]rts.Value,
	globals map[string]scripts.GlobalValue,
	extras ...map[string]string,
) *vars.Resolver {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	providers := make([]vars.Provider, 0, 9)

	if doc != nil && len(doc.Constants) > 0 {
		constValues := make(map[string]string, len(doc.Constants))
		for _, c := range doc.Constants {
			constValues[c.Name] = c.Value
		}
		providers = append(providers, vars.NewMapProvider("const", constValues))
	}

	for _, extra := range extras {
		if len(extra) > 0 {
			providers = append(providers, vars.NewMapProvider("script", extra))
		}
	}

	if req != nil {
		reqVars := make(map[string]string)
		for _, v := range req.Variables {
			reqVars[v.Name] = v.Value
		}
		if len(reqVars) > 0 {
			providers = append(providers, vars.NewMapProvider("request", reqVars))
		}
	}

	if globals != nil {
		if values := globalValueMap(globals); len(values) > 0 {
			providers = append(providers, vars.NewMapProvider("global", values))
		}
	} else if gs := m.globalsStore(); gs != nil {
		if snapshot := gs.Snapshot(resolvedEnv); len(snapshot) > 0 {
			values := make(map[string]string, len(snapshot))
			for key, entry := range snapshot {
				name := entry.Name
				if strings.TrimSpace(name) == "" {
					name = key
				}
				values[name] = entry.Value
			}
			providers = append(providers, vars.NewMapProvider("global", values))
		}
	}

	if doc != nil {
		globalVars := make(map[string]string)
		for _, v := range doc.Globals {
			globalVars[v.Name] = v.Value
		}
		if len(globalVars) > 0 {
			providers = append(providers, vars.NewMapProvider("document-global", globalVars))
		}
	}

	fileVars := make(map[string]string)
	if doc != nil {
		for _, v := range doc.Variables {
			fileVars[v.Name] = v.Value
		}
	}
	m.mergeFileRuntimeVars(fileVars, doc, resolvedEnv)
	if len(fileVars) > 0 {
		providers = append(providers, vars.NewMapProvider("file", fileVars))
	}

	if envValues := vars.EnvValues(m.cfg.EnvironmentSet, resolvedEnv); len(envValues) > 0 {
		providers = append(providers, vars.NewMapProvider("environment", envValues))
	}

	providers = append(providers, vars.EnvProvider{})
	res := vars.NewResolver(providers...)
	res.AddRefResolver(vars.EnvRefResolver)
	res.SetExprEval(m.rtsEval(ctx, doc, req, resolvedEnv, base, false, extraVals, extras...))
	res.SetExprPos(m.rtsPos(doc, req))
	return res
}

// buildDisplayResolver is a best-effort resolver for UI/status rendering that
// avoids expanding secret values.
func (m *Model) buildDisplayResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	extraVals map[string]rts.Value,
	extras ...map[string]string,
) *vars.Resolver {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	providers := make([]vars.Provider, 0, 9)

	if doc != nil && len(doc.Constants) > 0 {
		constValues := make(map[string]string, len(doc.Constants))
		for _, c := range doc.Constants {
			constValues[c.Name] = c.Value
		}
		providers = append(providers, vars.NewMapProvider("const", constValues))
	}

	for _, extra := range extras {
		if len(extra) > 0 {
			providers = append(providers, vars.NewMapProvider("script", extra))
		}
	}

	if req != nil {
		reqVars := make(map[string]string)
		for _, v := range req.Variables {
			if v.Secret {
				continue
			}
			reqVars[v.Name] = v.Value
		}
		if len(reqVars) > 0 {
			providers = append(providers, vars.NewMapProvider("request", reqVars))
		}
	}

	if gs := m.globalsStore(); gs != nil {
		if snapshot := gs.Snapshot(resolvedEnv); len(snapshot) > 0 {
			values := make(map[string]string, len(snapshot))
			for key, entry := range snapshot {
				if entry.Secret {
					continue
				}
				name := entry.Name
				if strings.TrimSpace(name) == "" {
					name = key
				}
				values[name] = entry.Value
			}
			if len(values) > 0 {
				providers = append(providers, vars.NewMapProvider("global", values))
			}
		}
	}

	if doc != nil {
		globalVars := make(map[string]string)
		for _, v := range doc.Globals {
			if v.Secret {
				continue
			}
			globalVars[v.Name] = v.Value
		}
		if len(globalVars) > 0 {
			providers = append(providers, vars.NewMapProvider("document-global", globalVars))
		}
	}

	fileVars := make(map[string]string)
	if doc != nil {
		for _, v := range doc.Variables {
			if v.Secret {
				continue
			}
			fileVars[v.Name] = v.Value
		}
	}
	m.mergeFileRuntimeVarsSafe(fileVars, doc, resolvedEnv)
	if len(fileVars) > 0 {
		providers = append(providers, vars.NewMapProvider("file", fileVars))
	}

	if envValues := vars.EnvValues(m.cfg.EnvironmentSet, resolvedEnv); len(envValues) > 0 {
		providers = append(providers, vars.NewMapProvider("environment", envValues))
	}

	providers = append(providers, vars.EnvProvider{})
	res := vars.NewResolver(providers...)
	res.AddRefResolver(vars.EnvRefResolver)
	res.SetExprEval(m.rtsEval(ctx, doc, req, resolvedEnv, base, true, extraVals, extras...))
	res.SetExprPos(m.rtsPos(doc, req))
	return res
}

func (m *Model) resolveSSH(
	doc *restfile.Document,
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
) (*ssh.Plan, error) {
	if req == nil || req.SSH == nil {
		return nil, nil
	}
	manager := m.ensureSSHManager()
	fileProfiles, globalProfiles := m.registryIndex().SSH(doc)
	cfg, err := ssh.Resolve(req.SSH, fileProfiles, globalProfiles, resolver, envName)
	if err != nil {
		return nil, err
	}
	if cfg != nil && !cfg.Strict {
		m.setStatusMessage(statusMsg{
			text:  "@ssh strict_hostkey=false (insecure)",
			level: statusWarn,
		})
	}
	return &ssh.Plan{Manager: manager, Config: cfg}, nil
}

func (m *Model) resolveK8s(
	doc *restfile.Document,
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
) (*k8s.Plan, error) {
	if req == nil || req.K8s == nil {
		return nil, nil
	}
	manager := m.ensureK8sManager()
	fileProfiles, globalProfiles := m.registryIndex().K8s(doc)
	cfg, err := k8s.Resolve(req.K8s, fileProfiles, globalProfiles, resolver, envName)
	if err != nil {
		return nil, err
	}
	return &k8s.Plan{Manager: manager, Config: cfg}, nil
}

func (m *Model) documentRuntimePath(doc *restfile.Document) string {
	if doc != nil && strings.TrimSpace(doc.Path) != "" {
		return doc.Path
	}
	return m.currentFile
}

func (m *Model) ensureSSHManager() *ssh.Manager {
	return m.sshManager()
}

func (m *Model) ensureK8sManager() *k8s.Manager {
	return m.k8sManager()
}

func (m *Model) mergeFileRuntimeVars(
	target map[string]string,
	doc *restfile.Document,
	envName string,
) {
	fs := m.fileStore()
	if target == nil || fs == nil {
		return
	}
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	path := m.documentRuntimePath(doc)
	if snapshot := fs.Snapshot(resolvedEnv, path); len(snapshot) > 0 {
		for key, entry := range snapshot {
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				name = key
			}
			target[name] = entry.Value
		}
	}
}

// mergeFileRuntimeVarsSafe merges runtime file vars while skipping secrets so UI
// previews do not leak them.
func (m *Model) mergeFileRuntimeVarsSafe(
	target map[string]string,
	doc *restfile.Document,
	envName string,
) {
	fs := m.fileStore()
	if target == nil || fs == nil {
		return
	}
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	path := m.documentRuntimePath(doc)
	if snapshot := fs.Snapshot(resolvedEnv, path); len(snapshot) > 0 {
		for key, entry := range snapshot {
			if entry.Secret {
				continue
			}
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				name = key
			}
			target[name] = entry.Value
		}
	}
}

func (m *Model) collectVariables(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
) map[string]string {
	return m.collectVariablesWithStoreGlobals(
		doc,
		req,
		envName,
		m.collectStoredGlobalValues(envName),
	)
}

func (m *Model) collectVariablesWithStoreGlobals(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
	storeGlobals map[string]scripts.GlobalValue,
) map[string]string {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	result := make(map[string]string)
	if env := vars.EnvValues(m.cfg.EnvironmentSet, resolvedEnv); env != nil {
		for k, v := range env {
			result[k] = v
		}
	}

	if doc != nil {
		for _, v := range doc.Variables {
			result[v.Name] = v.Value
		}
		for _, v := range doc.Globals {
			result[v.Name] = v.Value
		}
	}

	m.mergeFileRuntimeVars(result, doc, resolvedEnv)
	for name, value := range globalValueMap(storeGlobals) {
		result[name] = value
	}

	if req != nil {
		for _, v := range req.Variables {
			result[v.Name] = v.Value
		}
	}
	return result
}

func (m *Model) collectGlobalValues(
	doc *restfile.Document,
	envName string,
) map[string]scripts.GlobalValue {
	return effectiveGlobalValues(doc, m.collectStoredGlobalValues(envName))
}

func collectDocumentGlobalValues(doc *restfile.Document) map[string]scripts.GlobalValue {
	globals := make(map[string]scripts.GlobalValue)
	if doc != nil {
		for _, v := range doc.Globals {
			name := strings.TrimSpace(v.Name)
			if name == "" {
				continue
			}
			globals[name] = scripts.GlobalValue{Name: name, Value: v.Value, Secret: v.Secret}
		}
	}
	if len(globals) == 0 {
		return nil
	}
	return globals
}

func (m *Model) collectStoredGlobalValues(envName string) map[string]scripts.GlobalValue {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	globals := make(map[string]scripts.GlobalValue)
	if gs := m.globalsStore(); gs != nil {
		if snapshot := gs.Snapshot(resolvedEnv); len(snapshot) > 0 {
			for key, entry := range snapshot {
				name := strings.TrimSpace(entry.Name)
				if name == "" {
					name = key
				}
				globals[name] = scripts.GlobalValue{
					Name:   name,
					Value:  entry.Value,
					Secret: entry.Secret,
				}
			}
		}
	}
	if len(globals) == 0 {
		return nil
	}
	return globals
}

func effectiveGlobalValues(
	doc *restfile.Document,
	storeGlobals map[string]scripts.GlobalValue,
) map[string]scripts.GlobalValue {
	return mergeGlobalValues(collectDocumentGlobalValues(doc), storeGlobals)
}

func cloneGlobalValues(src map[string]scripts.GlobalValue) map[string]scripts.GlobalValue {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]scripts.GlobalValue, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func mergeGlobalValues(
	base map[string]scripts.GlobalValue,
	changes map[string]scripts.GlobalValue,
) map[string]scripts.GlobalValue {
	if len(base) == 0 && len(changes) == 0 {
		return nil
	}
	out := cloneGlobalValues(base)
	if out == nil {
		out = make(map[string]scripts.GlobalValue, len(changes))
	}
	for key, change := range changes {
		name := strings.TrimSpace(change.Name)
		if name == "" {
			name = strings.TrimSpace(key)
		}
		if name == "" {
			continue
		}
		for existing := range out {
			if strings.EqualFold(strings.TrimSpace(existing), name) {
				delete(out, existing)
			}
		}
		if change.Delete {
			continue
		}
		change.Name = name
		out[name] = change
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func globalValueMap(globals map[string]scripts.GlobalValue) map[string]string {
	if len(globals) == 0 {
		return nil
	}
	values := make(map[string]string, len(globals))
	for key, entry := range globals {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			name = strings.TrimSpace(key)
		}
		if name == "" || entry.Delete {
			continue
		}
		values[name] = entry.Value
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func (m *Model) applyGlobalMutations(changes map[string]scripts.GlobalValue, envName string) {
	gs := m.globalsStore()
	if len(changes) == 0 || gs == nil {
		return
	}

	env := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	for _, change := range changes {
		name := strings.TrimSpace(change.Name)
		if name == "" {
			continue
		}
		if change.Delete {
			gs.Delete(env, name)
			continue
		}
		gs.Set(env, name, change.Value, change.Secret)
	}
}

func (m *Model) showGlobalSummary() tea.Cmd {
	text := m.buildGlobalSummary()
	if strings.TrimSpace(text) == "" {
		text = "Globals: (empty)"
	}
	m.setStatusMessage(statusMsg{level: statusInfo, text: text})
	return nil
}

func (m *Model) buildGlobalSummary() string {
	var segments []string

	if snapshot := m.globalsSnapshot(); len(snapshot) > 0 {
		entries := make([]summaryEntry, 0, len(snapshot))
		for key, value := range snapshot {
			name := strings.TrimSpace(value.Name)
			if name == "" {
				name = key
			}
			entries = append(
				entries,
				summaryEntry{name: name, value: value.Value, secret: value.Secret},
			)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			parts = append(
				parts,
				fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)),
			)
		}
		segments = append(segments, "Globals: "+strings.Join(parts, ", "))
	}

	if doc := m.doc; doc != nil {
		entries := make([]summaryEntry, 0, len(doc.Globals))
		for _, global := range doc.Globals {
			name := strings.TrimSpace(global.Name)
			if name == "" {
				continue
			}
			entries = append(
				entries,
				summaryEntry{name: name, value: global.Value, secret: global.Secret},
			)
		}
		if len(entries) > 0 {
			sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
			parts := make([]string, 0, len(entries))
			for _, entry := range entries {
				parts = append(
					parts,
					fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)),
				)
			}
			segments = append(segments, "Doc: "+strings.Join(parts, ", "))
		}
	}

	return strings.Join(segments, " | ")
}

func (m *Model) globalsSnapshot() map[string]globalValue {
	gs := m.globalsStore()
	if gs == nil {
		return nil
	}
	return gs.Snapshot(m.cfg.EnvironmentName)
}

func (m *Model) clearGlobalValues() tea.Cmd {
	gs := m.globalsStore()
	if gs == nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No global store available"})
		return nil
	}

	env := m.cfg.EnvironmentName
	gs.Clear(env)
	if cs := m.cookieStore(); cs != nil {
		cs.Clear(env)
	}

	label := env
	if strings.TrimSpace(label) == "" {
		label = "default"
	}

	m.setStatusMessage(
		statusMsg{level: statusInfo, text: fmt.Sprintf("Cleared globals and cookies for %s", label)},
	)
	return nil
}

type summaryEntry struct {
	name   string
	value  string
	secret bool
}

func maskSecret(value string, secret bool) string {
	if secret {
		return "•••"
	}
	return value
}

func mergeVariableMaps(base map[string]string, additions map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(additions))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range additions {
		merged[k] = v
	}
	return merged
}

func (m *Model) resolveHTTPOptions(opts httpclient.Options) httpclient.Options {
	if opts.BaseDir == "" && m.currentFile != "" {
		opts.BaseDir = filepath.Dir(m.currentFile)
	}

	if fallbackEnabled() {
		fallbacks := make([]string, 0, len(opts.FallbackBaseDirs)+3)
		fallbacks = append(fallbacks, opts.FallbackBaseDirs...)
		fallbacks = append(fallbacks, opts.BaseDir)
		if m.workspaceRoot != "" {
			fallbacks = append(fallbacks, m.workspaceRoot)
		}
		if cwd, err := os.Getwd(); err == nil {
			fallbacks = append(fallbacks, cwd)
		}
		opts.FallbackBaseDirs = util.DedupeNonEmptyStrings(fallbacks)
		opts.NoFallback = false
	} else {
		opts.FallbackBaseDirs = nil
		opts.NoFallback = true
	}
	return opts
}

func fallbackEnabled() bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv("RESTERM_ENABLE_FALLBACK")))
	return val == "1" || val == "true" || val == "yes"
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	clone := make(map[string]string, len(input))
	for k, v := range input {
		clone[k] = v
	}
	return clone
}
