package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/parser/curl"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/traceutil"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) sendActiveRequest() tea.Cmd {
	content := m.editor.Value()
	doc := parser.Parse(m.currentFile, []byte(content))
	cursorLine := currentCursorLine(m.editor)
	req, _ := m.requestAtCursor(doc, content, cursorLine)
	if req == nil {
		return func() tea.Msg {
			return statusMsg{text: "No request at cursor", level: statusWarn}
		}
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(req)
	m.syncSSHGlobals(doc)

	cloned := cloneRequest(req)
	m.currentRequest = cloned
	m.testResults = nil
	m.scriptError = nil
	options := m.cfg.HTTPOptions
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}

	if spec := m.compareSpecForRequest(cloned); spec != nil {
		if cloned.Metadata.Profile != nil {
			m.setStatusMessage(statusMsg{level: statusWarn, text: "@compare cannot run alongside @profile"})
			return nil
		}
		return m.startCompareRun(doc, cloned, spec, options)
	}

	if cloned.Metadata.Trace != nil && cloned.Metadata.Trace.Enabled {
		options.Trace = true
		if budget, ok := traceutil.BudgetFromSpec(cloned.Metadata.Trace); ok {
			options.TraceBudget = &budget
		}
	}

	if cloned.Metadata.Profile != nil {
		return m.startProfileRun(doc, cloned, options)
	}

	m.sending = true
	target := m.statusRequestTarget(doc, cloned, "")
	base := "Sending"
	if trimmed := strings.TrimSpace(target); trimmed != "" {
		base = fmt.Sprintf("Sending %s", trimmed)
	}
	m.statusPulseBase = base
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: base, level: statusInfo})

	execCmd := m.executeRequest(doc, cloned, options, "")
	if tick := m.scheduleStatusPulse(); tick != nil {
		return tea.Batch(execCmd, tick)
	}
	return execCmd
}

// Allow CLI-level compare flags to kick off a sweep even when the request lacks
// @compare metadata so users can reuse the same editor workflow while honoring
// --compare selections.
func (m *Model) startConfigCompareFromEditor() tea.Cmd {
	content := m.editor.Value()
	doc := parser.Parse(m.currentFile, []byte(content))
	cursorLine := currentCursorLine(m.editor)
	req, _ := m.requestAtCursor(doc, content, cursorLine)
	if req == nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No request at cursor"})
		return nil
	}

	if req.Metadata.Profile != nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "@profile cannot run during compare"})
		return nil
	}

	spec := buildConfigCompareSpec(m.cfg.CompareTargets, m.cfg.CompareBase)
	if spec == nil && req.Metadata.Compare != nil {
		spec = cloneCompareSpec(req.Metadata.Compare)
	}
	if spec == nil {
		m.setStatusMessage(statusMsg{
			level: statusWarn,
			text:  "No compare targets configured. Use --compare or add @compare.",
		})
		return nil
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(req)
	m.syncSSHGlobals(doc)

	cloned := cloneRequest(req)
	m.currentRequest = cloned
	m.testResults = nil
	m.scriptError = nil

	options := m.cfg.HTTPOptions
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}
	if cloned.Metadata.Trace != nil && cloned.Metadata.Trace.Enabled {
		options.Trace = true
		if budget, ok := traceutil.BudgetFromSpec(cloned.Metadata.Trace); ok {
			options.TraceBudget = &budget
		}
	}

	return m.startCompareRun(doc, cloned, spec, options)
}

// Accept an environment override so compare sweeps can force a per-iteration
// scope without mutating the global environment selection.
func (m *Model) executeRequest(doc *restfile.Document, req *restfile.Request, options httpclient.Options, envOverride string, extras ...map[string]string) tea.Cmd {
	if req != nil && req.Metadata.Trace != nil && req.Metadata.Trace.Enabled {
		options.Trace = true
		if budget, ok := traceutil.BudgetFromSpec(req.Metadata.Trace); ok {
			options.TraceBudget = &budget
		}
	}
	client := m.client
	runner := m.scriptRunner

	// selecting env this way lets compare overrides win without persisting the change.
	envName := vars.SelectEnv(m.cfg.EnvironmentSet, envOverride, m.cfg.EnvironmentName)
	baseVars := m.collectVariables(doc, req, envName)
	if len(extras) > 0 {
		for _, extra := range extras {
			for key, value := range extra {
				if key == "" {
					continue
				}
				baseVars[key] = value
			}
		}
	}

	return func() tea.Msg {
		preVars := cloneStringMap(baseVars)
		preGlobals := m.collectGlobalValues(doc, envName)
		preResult, err := runner.RunPreRequest(req.Metadata.Scripts, scripts.PreRequestInput{
			Request:   req,
			Variables: preVars,
			Globals:   preGlobals,
			BaseDir:   options.BaseDir,
		})
		if err != nil {
			return responseMsg{err: errdef.Wrap(errdef.CodeScript, err, "pre-request script"), executed: req}
		}

		if err := applyPreRequestOutput(req, preResult); err != nil {
			return responseMsg{err: err, executed: req}
		}

		m.applyGlobalMutations(preResult.Globals, envName)

		scriptVars := cloneStringMap(preResult.Variables)
		resolverExtras := make([]map[string]string, 0, len(extras)+1)
		if len(scriptVars) > 0 {
			resolverExtras = append(resolverExtras, scriptVars)
		}
		for _, extra := range extras {
			if len(extra) > 0 {
				resolverExtras = append(resolverExtras, extra)
			}
		}

		resolver := m.buildResolver(doc, req, envName, resolverExtras...)
		sshPlan, err := m.resolveSSH(doc, req, resolver, envName)
		if err != nil {
			return responseMsg{err: errdef.Wrap(errdef.CodeHTTP, err, "resolve ssh"), executed: req}
		}
		if sshPlan != nil {
			options.SSH = sshPlan
		}

		effectiveTimeout := defaultTimeout(resolveRequestTimeout(req, options.Timeout))
		if err := m.ensureOAuth(req, resolver, options, envName, effectiveTimeout); err != nil {
			return responseMsg{err: err, executed: req}
		}

		var (
			ctx          context.Context
			cancel       context.CancelFunc
			cancelActive = true
		)

		if req.WebSocket != nil && len(req.WebSocket.Steps) == 0 {
			ctx, cancel = context.WithCancel(context.Background())
		} else {
			ctx, cancel = context.WithTimeout(context.Background(), effectiveTimeout)
		}
		defer func() {
			if cancelActive {
				cancel()
			}
		}()

		if req.GRPC != nil {
			if err := m.prepareGRPCRequest(req, resolver); err != nil {
				return responseMsg{err: err, executed: req}
			}
		}

		if req.GRPC != nil {
			grpcOpts := m.grpcOptions
			if grpcOpts.BaseDir == "" {
				grpcOpts.BaseDir = options.BaseDir
				if grpcOpts.BaseDir == "" && m.currentFile != "" {
					grpcOpts.BaseDir = filepath.Dir(m.currentFile)
				}
			}

			if grpcOpts.DialTimeout == 0 {
				grpcOpts.DialTimeout = effectiveTimeout
			}

			if sshPlan != nil {
				grpcOpts.SSH = sshPlan
			}

			grpcResp, grpcErr := m.grpcClient.Execute(ctx, req, req.GRPC, grpcOpts)
			if grpcErr != nil {
				return responseMsg{
					grpc:        grpcResp,
					err:         grpcErr,
					executed:    req,
					requestText: renderRequestText(req),
					environment: envName,
				}
			}

			respForScripts := grpcScriptResponse(req, grpcResp)
			var captures captureResult
			if err := m.applyCaptures(doc, req, resolver, respForScripts, nil, &captures, envName); err != nil {
				return responseMsg{err: err, executed: req}
			}

			updatedVars := m.collectVariables(doc, req, envName)
			testVars := mergeVariableMaps(updatedVars, scriptVars)
			testGlobals := m.collectGlobalValues(doc, envName)
			tests, globalChanges, testErr := runner.RunTests(req.Metadata.Scripts, scripts.TestInput{
				Response:  respForScripts,
				Variables: testVars,
				Globals:   testGlobals,
				BaseDir:   options.BaseDir,
			})
			m.applyGlobalMutations(globalChanges, envName)

			return responseMsg{
				grpc:        grpcResp,
				tests:       tests,
				scriptErr:   testErr,
				executed:    req,
				requestText: renderRequestText(req),
				environment: envName,
			}
		}

		var response *httpclient.Response
		switch {
		case req.WebSocket != nil:
			if err := m.expandWebSocketSteps(req, resolver); err != nil {
				return responseMsg{err: err, executed: req}
			}
			handle, fallback, startErr := client.StartWebSocket(ctx, req, resolver, options)
			if startErr != nil {
				return responseMsg{err: startErr, executed: req}
			}
			if fallback != nil {
				response = fallback
			} else {
				m.attachWebSocketHandle(handle, req)
				if len(req.WebSocket.Steps) == 0 {
					if handle != nil && handle.Session != nil {
						sessionDone := handle.Session.Done()
						go func() {
							<-sessionDone
							cancel()
						}()
						cancelActive = false
					}
					response = streamingPlaceholderResponse(handle.Meta)
				} else {
					response, err = client.CompleteWebSocket(ctx, handle, req, options)
				}
			}
		case req.SSE != nil:
			handle, fallback, startErr := client.StartSSE(ctx, req, resolver, options)
			if startErr != nil {
				return responseMsg{err: startErr, executed: req}
			}
			if fallback != nil {
				response = fallback
			} else {
				m.attachSSEHandle(handle, req)
				response, err = httpclient.CompleteSSE(handle)
			}
		default:
			response, err = client.Execute(ctx, req, resolver, options)
		}
		if err != nil {
			return responseMsg{response: response, err: err, executed: req}
		}

		streamInfo, streamErr := streamInfoFromResponse(req, response)
		if streamErr != nil {
			return responseMsg{err: errdef.Wrap(errdef.CodeHTTP, streamErr, "decode stream transcript"), executed: req}
		}

		respForScripts := httpScriptResponse(response)
		var captures captureResult
		if err := m.applyCaptures(doc, req, resolver, respForScripts, streamInfo, &captures, envName); err != nil {
			return responseMsg{err: err, executed: req}
		}

		updatedVars := m.collectVariables(doc, req, envName)
		testVars := mergeVariableMaps(updatedVars, scriptVars)
		testGlobals := m.collectGlobalValues(doc, envName)
		traceInput := scripts.NewTraceInput(response.Timeline, req.Metadata.Trace)
		tests, globalChanges, testErr := runner.RunTests(req.Metadata.Scripts, scripts.TestInput{
			Response:  respForScripts,
			Variables: testVars,
			Globals:   testGlobals,
			BaseDir:   options.BaseDir,
			Stream:    streamInfo,
			Trace:     traceInput,
		})
		m.applyGlobalMutations(globalChanges, envName)

		return responseMsg{
			response:    response,
			tests:       tests,
			scriptErr:   testErr,
			executed:    req,
			requestText: renderRequestText(req),
			environment: envName,
		}
	}
}

func streamingPlaceholderResponse(meta httpclient.StreamMeta) *httpclient.Response {
	headers := meta.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}

	headers.Set(streamHeaderType, "websocket")
	headers.Set(streamHeaderSummary, "streaming")
	status := meta.Status
	if strings.TrimSpace(status) == "" {
		status = "101 Switching Protocols"
	}

	statusCode := meta.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusSwitchingProtocols
	}

	return &httpclient.Response{
		Status:         status,
		StatusCode:     statusCode,
		Proto:          meta.Proto,
		Headers:        headers,
		ReqMethod:      meta.RequestMethod,
		RequestHeaders: cloneHeader(meta.RequestHeaders),
		ReqHost:        meta.RequestHost,
		ReqLen:         meta.RequestLength,
		ReqTE:          append([]string(nil), meta.RequestTE...),
		EffectiveURL:   meta.EffectiveURL,
		Request:        meta.Request,
	}
}

func (m *Model) expandWebSocketSteps(req *restfile.Request, resolver *vars.Resolver) error {
	if req == nil || req.WebSocket == nil || resolver == nil {
		return nil
	}

	steps := req.WebSocket.Steps
	if len(steps) == 0 {
		return nil
	}

	for i := range steps {
		step := &steps[i]
		if trimmed := strings.TrimSpace(step.Value); trimmed != "" {
			expanded, err := resolver.ExpandTemplates(trimmed)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket step value")
			}
			step.Value = expanded
		}
		if trimmed := strings.TrimSpace(step.File); trimmed != "" {
			expanded, err := resolver.ExpandTemplates(trimmed)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket file path")
			}
			step.File = expanded
		}
		if trimmed := strings.TrimSpace(step.Reason); trimmed != "" {
			expanded, err := resolver.ExpandTemplates(trimmed)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket close reason")
			}
			step.Reason = expanded
		}
	}

	req.WebSocket.Steps = steps
	return nil
}

func httpScriptResponse(resp *httpclient.Response) *scripts.Response {
	if resp == nil {
		return nil
	}
	return &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: resp.Status,
		Code:   resp.StatusCode,
		URL:    resp.EffectiveURL,
		Time:   resp.Duration,
		Header: cloneHeader(resp.Headers),
		Body:   append([]byte(nil), resp.Body...),
	}
}

func grpcScriptResponse(req *restfile.Request, resp *grpcclient.Response) *scripts.Response {
	if resp == nil {
		return nil
	}

	headers := make(http.Header)
	for name, values := range resp.Headers {
		for _, value := range values {
			headers.Add(name, value)
		}
	}
	for name, values := range resp.Trailers {
		key := "Grpc-Trailer-" + name
		for _, value := range values {
			headers.Add(key, value)
		}
	}

	status := resp.StatusCode.String()
	if msg := strings.TrimSpace(resp.StatusMessage); msg != "" && !strings.EqualFold(msg, status) {
		status = fmt.Sprintf("%s (%s)", status, msg)
	}

	target := ""
	if req != nil && req.GRPC != nil {
		target = strings.TrimSpace(req.GRPC.Target)
	}

	return &scripts.Response{
		Kind:   scripts.ResponseKindGRPC,
		Status: status,
		Code:   int(resp.StatusCode),
		URL:    target,
		Time:   resp.Duration,
		Header: headers,
		Body:   []byte(resp.Message),
	}
}

const statusPulseInterval = 1 * time.Second
const (
	streamHeaderType    = "X-Resterm-Stream-Type"
	streamHeaderSummary = "X-Resterm-Stream-Summary"
)

func (m *Model) scheduleStatusPulse() tea.Cmd {
	if !m.sending {
		return nil
	}
	return tea.Tick(statusPulseInterval, func(time.Time) tea.Msg {
		return statusPulseMsg{}
	})
}

func (m *Model) handleStatusPulse() tea.Cmd {
	if !m.sending {
		return nil
	}

	m.statusPulseFrame++
	if m.statusPulseFrame >= 3 {
		m.statusPulseFrame = 0
	}

	base := strings.TrimSpace(m.statusPulseBase)
	if base == "" {
		base = "Sending"
	}

	dots := strings.Repeat(".", m.statusPulseFrame+1)
	m.setStatusMessage(statusMsg{text: base + dots, level: statusInfo})
	return m.scheduleStatusPulse()
}

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

func (m *Model) buildResolver(doc *restfile.Document, req *restfile.Request, envName string, extras ...map[string]string) *vars.Resolver {
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

	if m.globals != nil {
		if snapshot := m.globals.snapshot(resolvedEnv); len(snapshot) > 0 {
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
	return vars.NewResolver(providers...)
}

// buildDisplayResolver is a best-effort resolver for UI/status rendering that
// avoids expanding secret values.
func (m *Model) buildDisplayResolver(doc *restfile.Document, req *restfile.Request, envName string, extras ...map[string]string) *vars.Resolver {
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

	if m.globals != nil {
		if snapshot := m.globals.snapshot(resolvedEnv); len(snapshot) > 0 {
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
	return vars.NewResolver(providers...)
}

func (m *Model) resolveSSH(doc *restfile.Document, req *restfile.Request, resolver *vars.Resolver, envName string) (*ssh.Plan, error) {
	if req == nil || req.SSH == nil {
		return nil, nil
	}
	if m.sshGlobals != nil {
		path := m.documentRuntimePath(doc)
		m.sshGlobals.set(path, docSSHProfiles(doc))
	}

	manager := m.sshMgr
	if manager == nil {
		manager = ssh.NewManager()
		m.sshMgr = manager
	}
	fileProfiles := docSSHProfiles(doc)
	globalProfiles := []restfile.SSHProfile(nil)
	if m.sshGlobals != nil {
		globalProfiles = m.sshGlobals.all()
	}
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

func (m *Model) documentRuntimePath(doc *restfile.Document) string {
	if doc != nil && strings.TrimSpace(doc.Path) != "" {
		return doc.Path
	}
	return m.currentFile
}

func (m *Model) syncSSHGlobals(doc *restfile.Document) {
	if m.sshGlobals == nil {
		return
	}
	path := m.documentRuntimePath(doc)
	m.sshGlobals.set(path, docSSHProfiles(doc))
}

func docSSHProfiles(doc *restfile.Document) []restfile.SSHProfile {
	if doc == nil {
		return nil
	}
	return doc.SSH
}

func (m *Model) mergeFileRuntimeVars(target map[string]string, doc *restfile.Document, envName string) {
	if target == nil || m.fileVars == nil {
		return
	}
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	path := m.documentRuntimePath(doc)
	if snapshot := m.fileVars.snapshot(resolvedEnv, path); len(snapshot) > 0 {
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
func (m *Model) mergeFileRuntimeVarsSafe(target map[string]string, doc *restfile.Document, envName string) {
	if target == nil || m.fileVars == nil {
		return
	}
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	path := m.documentRuntimePath(doc)
	if snapshot := m.fileVars.snapshot(resolvedEnv, path); len(snapshot) > 0 {
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

func (m *Model) collectVariables(doc *restfile.Document, req *restfile.Request, envName string) map[string]string {
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
	if m.globals != nil {
		if snapshot := m.globals.snapshot(resolvedEnv); len(snapshot) > 0 {
			for key, entry := range snapshot {
				name := entry.Name
				if strings.TrimSpace(name) == "" {
					name = key
				}
				result[name] = entry.Value
			}
		}
	}

	if req != nil {
		for _, v := range req.Variables {
			result[v.Name] = v.Value
		}
	}
	return result
}

func (m *Model) collectGlobalValues(doc *restfile.Document, envName string) map[string]scripts.GlobalValue {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
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

	if m.globals != nil {
		if snapshot := m.globals.snapshot(resolvedEnv); len(snapshot) > 0 {
			for key, entry := range snapshot {
				name := strings.TrimSpace(entry.Name)
				if name == "" {
					name = key
				}
				globals[name] = scripts.GlobalValue{Name: name, Value: entry.Value, Secret: entry.Secret}
			}
		}
	}

	if len(globals) == 0 {
		return nil
	}

	return globals
}

func (m *Model) applyGlobalMutations(changes map[string]scripts.GlobalValue, envName string) {
	if len(changes) == 0 || m.globals == nil {
		return
	}

	env := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	for _, change := range changes {
		name := strings.TrimSpace(change.Name)
		if name == "" {
			continue
		}
		if change.Delete {
			m.globals.delete(env, name)
			continue
		}
		m.globals.set(env, name, change.Value, change.Secret)
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
			entries = append(entries, summaryEntry{name: name, value: value.Value, secret: value.Secret})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			parts = append(parts, fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)))
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
			entries = append(entries, summaryEntry{name: name, value: global.Value, secret: global.Secret})
		}
		if len(entries) > 0 {
			sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
			parts := make([]string, 0, len(entries))
			for _, entry := range entries {
				parts = append(parts, fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)))
			}
			segments = append(segments, "Doc: "+strings.Join(parts, ", "))
		}
	}

	return strings.Join(segments, " | ")
}

func (m *Model) globalsSnapshot() map[string]globalValue {
	if m.globals == nil {
		return nil
	}
	return m.globals.snapshot(m.cfg.EnvironmentName)
}

func (m *Model) clearGlobalValues() tea.Cmd {
	if m.globals == nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No global store available"})
		return nil
	}

	env := m.cfg.EnvironmentName
	m.globals.clear(env)
	label := env
	if strings.TrimSpace(label) == "" {
		label = "default"
	}

	m.setStatusMessage(statusMsg{level: statusInfo, text: fmt.Sprintf("Cleared globals for %s", label)})
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

type captureResult struct {
	requestVars map[string]restfile.Variable
	fileVars    map[string]restfile.Variable
}

func (r *captureResult) addRequest(name, value string, secret bool) {
	if r == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if r.requestVars == nil {
		r.requestVars = make(map[string]restfile.Variable)
	}
	key := strings.ToLower(name)
	r.requestVars[key] = restfile.Variable{Name: name, Value: value, Secret: secret, Scope: restfile.ScopeRequest}
}

func (r *captureResult) addFile(name, value string, secret bool) {
	if r == nil {
		return
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return
	}

	if r.fileVars == nil {
		r.fileVars = make(map[string]restfile.Variable)
	}

	key := strings.ToLower(name)
	r.fileVars[key] = restfile.Variable{Name: name, Value: value, Secret: secret, Scope: restfile.ScopeFile}
}

func (m *Model) applyCaptures(
	doc *restfile.Document,
	req *restfile.Request,
	resolver *vars.Resolver,
	resp *scripts.Response,
	stream *scripts.StreamInfo,
	result *captureResult,
	envName string,
) error {
	if req == nil || resp == nil {
		return nil
	}
	if len(req.Metadata.Captures) == 0 {
		return nil
	}

	envKey := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	ctx := newCaptureContext(resp, stream)
	for _, capture := range req.Metadata.Captures {
		value, err := ctx.evaluate(capture.Expression, resolver)
		if err != nil {
			return errdef.Wrap(errdef.CodeScript, err, "evaluate capture %s", capture.Name)
		}
		switch capture.Scope {
		case restfile.CaptureScopeRequest:
			upsertVariable(&req.Variables, restfile.ScopeRequest, capture.Name, value, capture.Secret)
			if result != nil {
				result.addRequest(capture.Name, value, capture.Secret)
			}
		case restfile.CaptureScopeFile:
			if doc != nil {
				upsertVariable(&doc.Variables, restfile.ScopeFile, capture.Name, value, capture.Secret)
			}
			if result != nil {
				result.addFile(capture.Name, value, capture.Secret)
			}
		case restfile.CaptureScopeGlobal:
			if m.globals != nil {
				m.globals.set(envKey, capture.Name, value, capture.Secret)
			}
		}
	}

	if result != nil && len(result.fileVars) > 0 && m.fileVars != nil {
		path := m.documentRuntimePath(doc)
		for _, entry := range result.fileVars {
			m.fileVars.set(envKey, path, entry.Name, entry.Value, entry.Secret)
		}
	}

	return nil
}

type captureContext struct {
	response  *scripts.Response
	body      string
	headers   http.Header
	stream    *scripts.StreamInfo
	jsonOnce  sync.Once
	jsonValue interface{}
	jsonErr   error
}

var captureTemplatePattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func newCaptureContext(resp *scripts.Response, stream *scripts.StreamInfo) *captureContext {
	body := ""
	if resp != nil {
		body = string(resp.Body)
	}

	var headers http.Header
	if resp != nil {
		headers = cloneHeader(resp.Header)
	}
	return &captureContext{response: resp, body: body, headers: headers, stream: stream}
}

func (c *captureContext) evaluate(expr string, resolver *vars.Resolver) (string, error) {
	var firstErr error
	expanded := captureTemplatePattern.ReplaceAllStringFunc(expr, func(match string) string {
		name := strings.TrimSpace(captureTemplatePattern.FindStringSubmatch(match)[1])
		if name == "" {
			return match
		}

		if strings.HasPrefix(strings.ToLower(name), "response.") {
			value, err := c.lookupResponse(strings.TrimSpace(name[len("response."):]))
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return match
			}
			return value
		}

		if strings.HasPrefix(strings.ToLower(name), "stream.") {
			value, err := c.lookupStream(strings.TrimSpace(name[len("stream."):]))
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return match
			}
			return value
		}

		if resolver != nil {
			res, err := resolver.ExpandTemplates(match)
			if err == nil {
				return res
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		return match
	})

	if firstErr != nil {
		return "", firstErr
	}
	return expanded, nil
}

func (c *captureContext) lookupResponse(path string) (string, error) {
	switch strings.ToLower(path) {
	case "body":
		return c.body, nil
	case "status":
		if c.response != nil {
			return c.response.Status, nil
		}
		return "", nil
	case "statuscode":
		if c.response != nil {
			return strconv.Itoa(c.response.Code), nil
		}
		return "", nil
	}
	if strings.HasPrefix(strings.ToLower(path), "headers.") {
		key := path[len("headers."):]
		if c.headers == nil {
			return "", fmt.Errorf("header %s not available", key)
		}
		values := c.headers.Values(key)
		if len(values) == 0 {
			values = c.headers.Values(http.CanonicalHeaderKey(key))
		}
		if len(values) == 0 {
			return "", fmt.Errorf("header %s not found", key)
		}
		return strings.Join(values, ", "), nil
	}
	if strings.HasPrefix(strings.ToLower(path), "json") {
		return c.lookupJSON(path), nil
	}
	return "", fmt.Errorf("unsupported response reference %q", path)
}

func (c *captureContext) lookupStream(path string) (string, error) {
	if c.stream == nil {
		return "", fmt.Errorf("stream data not available")
	}

	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("stream reference empty")
	}

	lower := strings.ToLower(trimmed)
	if lower == "kind" {
		return c.stream.Kind, nil
	}

	if strings.HasPrefix(lower, "summary.") {
		key := strings.TrimSpace(trimmed[len("summary."):])
		value, ok := caseLookup(c.stream.Summary, key)
		if !ok {
			return "", fmt.Errorf("stream summary field %s not found", key)
		}
		return formatCaptureValue(value)
	}

	if strings.HasPrefix(lower, "events[") {
		inner := trimmed[len("events["):]
		closeIdx := strings.Index(inner, "]")
		if closeIdx <= 0 {
			return "", fmt.Errorf("invalid stream events reference")
		}
		count := len(c.stream.Events)
		if count == 0 {
			return "", fmt.Errorf("stream events empty")
		}
		indexText := strings.TrimSpace(inner[:closeIdx])
		idx, err := strconv.Atoi(indexText)
		if err != nil {
			return "", fmt.Errorf("stream event index %s invalid", indexText)
		}
		if idx < 0 {
			idx = count + idx
		}
		if idx < 0 || idx >= count {
			return "", fmt.Errorf("stream event index %s out of range", indexText)
		}
		event := c.stream.Events[idx]
		remainder := strings.TrimSpace(inner[closeIdx+1:])
		remainder = strings.TrimPrefix(remainder, ".")
		remainder = strings.TrimSpace(remainder)
		if remainder == "" {
			return formatCaptureValue(event)
		}
		value, ok := caseLookup(event, remainder)
		if !ok {
			return "", fmt.Errorf("stream event field %s not found", remainder)
		}
		return formatCaptureValue(value)
	}

	return "", fmt.Errorf("unsupported stream reference %q", path)
}

func (c *captureContext) lookupJSON(path string) string {
	c.jsonOnce.Do(func() {
		if strings.TrimSpace(c.body) == "" {
			c.jsonErr = fmt.Errorf("response body empty")
			return
		}
		var data interface{}
		if err := json.Unmarshal([]byte(c.body), &data); err != nil {
			c.jsonErr = err
			return
		}
		c.jsonValue = data
	})
	if c.jsonErr != nil {
		return ""
	}

	trimmed := strings.TrimSpace(path[len("json"):])
	if trimmed == "" {
		return c.body
	}

	trimmed = strings.TrimPrefix(trimmed, ".")
	current := c.jsonValue
	for _, segment := range splitJSONPath(trimmed) {
		switch typed := current.(type) {
		case map[string]interface{}:
			val, ok := typed[segment.name]
			if !ok {
				return ""
			}
			current = val
		case []interface{}:
			if segment.index == nil {
				return ""
			}
			idx := *segment.index
			if idx < 0 || idx >= len(typed) {
				return ""
			}
			current = typed[idx]
		default:
			return ""
		}
	}
	return stringifyJSONValue(current)
}

type jsonPathSegment struct {
	name  string
	index *int
}

func splitJSONPath(path string) []jsonPathSegment {
	parts := strings.Split(path, ".")
	segments := make([]jsonPathSegment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		segment := jsonPathSegment{}
		if bracket := strings.Index(part, "["); bracket != -1 {
			segment.name = part[:bracket]
			end := strings.Index(part[bracket:], "]")
			if end > 1 {
				idxStr := part[bracket+1 : bracket+end]
				if n, err := strconv.Atoi(idxStr); err == nil {
					segment.index = &n
				}
			}
		} else {
			segment.name = part
		}
		segments = append(segments, segment)
	}
	return segments
}

func stringifyJSONValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if float64(int64(v)) == v {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}

func caseLookup(m map[string]interface{}, key string) (interface{}, bool) {
	if m == nil {
		return nil, false
	}
	if value, ok := m[key]; ok {
		return value, true
	}
	lower := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return nil, false
}

func formatCaptureValue(value interface{}) (string, error) {
	if value == nil {
		return "", nil
	}
	switch v := value.(type) {
	case string:
		return v, nil
	case fmt.Stringer:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case float32, float64:
		return fmt.Sprintf("%v", v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v), nil
		}
		return string(data), nil
	}
}

func upsertVariable(list *[]restfile.Variable, scope restfile.VariableScope, name, value string, secret bool) {
	lower := strings.ToLower(name)
	vars := *list
	for i := range vars {
		if strings.ToLower(vars[i].Name) == lower {
			vars[i].Value = value
			vars[i].Scope = scope
			vars[i].Secret = secret
			return
		}
	}
	*list = append(vars, restfile.Variable{Name: name, Value: value, Scope: scope, Secret: secret})
}

func (m *Model) ensureOAuth(req *restfile.Request, resolver *vars.Resolver, opts httpclient.Options, envName string, timeout time.Duration) error {
	if req == nil || req.Metadata.Auth == nil {
		return nil
	}
	if !strings.EqualFold(req.Metadata.Auth.Type, "oauth2") {
		return nil
	}
	if m.oauth == nil {
		return errdef.New(errdef.CodeHTTP, "oauth support is not initialised")
	}

	cfg, err := m.buildOAuthConfig(req.Metadata.Auth, resolver)
	if err != nil {
		return err
	}

	envKey := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	cfg = m.oauth.MergeCachedConfig(envKey, cfg)
	if cfg.TokenURL == "" {
		return errdef.New(errdef.CodeHTTP, "@auth oauth2 requires token_url (include it once per cache_key to seed the cache)")
	}

	grant := strings.ToLower(strings.TrimSpace(cfg.GrantType))
	header := cfg.Header
	if strings.TrimSpace(header) == "" {
		header = "Authorization"
	}
	if req.Headers != nil && req.Headers.Get(header) != "" {
		return nil
	}

	tokenTimeout := timeout
	if grant == "authorization_code" && tokenTimeout < 2*time.Minute {
		tokenTimeout = 2 * time.Minute
		m.setStatusMessage(statusMsg{level: statusInfo, text: "Open browser to complete OAuth (auth code/PKCE)"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), tokenTimeout)

	defer cancel()

	token, err := m.oauth.Token(ctx, envKey, cfg, opts)
	if err != nil {
		return errdef.Wrap(errdef.CodeHTTP, err, "fetch oauth token")
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	if req.Headers.Get(header) != "" {
		return nil
	}

	value := token.AccessToken
	if strings.EqualFold(header, "authorization") {
		typeValue := strings.TrimSpace(token.TokenType)
		if typeValue == "" {
			typeValue = "Bearer"
		}
		value = strings.TrimSpace(typeValue) + " " + token.AccessToken
	}

	req.Headers.Set(header, value)
	return nil
}

func (m *Model) buildOAuthConfig(auth *restfile.AuthSpec, resolver *vars.Resolver) (oauth.Config, error) {
	cfg := oauth.Config{Extra: make(map[string]string)}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, "missing oauth spec")
	}

	expand := func(key string) (string, error) {
		value := auth.Params[key]
		if strings.TrimSpace(value) == "" {
			return "", nil
		}
		if resolver == nil {
			return strings.TrimSpace(value), nil
		}
		expanded, err := resolver.ExpandTemplates(value)
		if err != nil {
			return "", errdef.Wrap(errdef.CodeHTTP, err, "expand oauth param %s", key)
		}
		return strings.TrimSpace(expanded), nil
	}

	var err error
	if cfg.TokenURL, err = expand("token_url"); err != nil {
		return cfg, err
	}
	if cfg.AuthURL, err = expand("auth_url"); err != nil {
		return cfg, err
	}
	if cfg.RedirectURL, err = expand("redirect_uri"); err != nil {
		return cfg, err
	}
	if cfg.ClientID, err = expand("client_id"); err != nil {
		return cfg, err
	}
	if cfg.ClientSecret, err = expand("client_secret"); err != nil {
		return cfg, err
	}
	if cfg.Scope, err = expand("scope"); err != nil {
		return cfg, err
	}
	if cfg.Audience, err = expand("audience"); err != nil {
		return cfg, err
	}
	if cfg.Resource, err = expand("resource"); err != nil {
		return cfg, err
	}
	if cfg.Username, err = expand("username"); err != nil {
		return cfg, err
	}
	if cfg.Password, err = expand("password"); err != nil {
		return cfg, err
	}
	if cfg.ClientAuth, err = expand("client_auth"); err != nil {
		return cfg, err
	}
	if cfg.GrantType, err = expand("grant"); err != nil {
		return cfg, err
	}
	if cfg.Header, err = expand("header"); err != nil {
		return cfg, err
	}
	if cfg.CacheKey, err = expand("cache_key"); err != nil {
		return cfg, err
	}
	if cfg.CodeVerifier, err = expand("code_verifier"); err != nil {
		return cfg, err
	}
	if cfg.CodeMethod, err = expand("code_challenge_method"); err != nil {
		return cfg, err
	}
	if cfg.State, err = expand("state"); err != nil {
		return cfg, err
	}

	known := map[string]struct{}{
		"token_url":             {},
		"auth_url":              {},
		"redirect_uri":          {},
		"client_id":             {},
		"client_secret":         {},
		"scope":                 {},
		"audience":              {},
		"resource":              {},
		"username":              {},
		"password":              {},
		"client_auth":           {},
		"grant":                 {},
		"header":                {},
		"cache_key":             {},
		"code_verifier":         {},
		"code_challenge_method": {},
		"state":                 {},
	}
	for key, raw := range auth.Params {
		if _, ok := known[strings.ToLower(key)]; ok {
			continue
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value, err := expand(key)
		if err != nil {
			return cfg, err
		}
		if value != "" {
			cfg.Extra[strings.ToLower(strings.ReplaceAll(key, "-", "_"))] = value
		}
	}
	if len(cfg.Extra) == 0 {
		cfg.Extra = nil
	}
	return cfg, nil
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

func (m *Model) prepareGRPCRequest(req *restfile.Request, resolver *vars.Resolver) error {
	grpcReq := req.GRPC
	if grpcReq == nil {
		return nil
	}

	if strings.TrimSpace(grpcReq.FullMethod) == "" {
		service := strings.TrimSpace(grpcReq.Service)
		method := strings.TrimSpace(grpcReq.Method)
		if service != "" && method != "" {
			if grpcReq.Package != "" {
				grpcReq.FullMethod = "/" + grpcReq.Package + "." + service + "/" + method
			} else {
				grpcReq.FullMethod = "/" + service + "/" + method
			}
		} else {
			return errdef.New(errdef.CodeHTTP, "grpc method metadata is incomplete")
		}
	}

	if text := strings.TrimSpace(req.Body.Text); text != "" {
		grpcReq.Message = req.Body.Text
		grpcReq.MessageFile = ""
	} else if file := strings.TrimSpace(req.Body.FilePath); file != "" {
		grpcReq.MessageFile = req.Body.FilePath
		grpcReq.Message = ""
	}

	if resolver != nil {
		target, err := resolver.ExpandTemplates(grpcReq.Target)
		if err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc target")
		}

		grpcReq.Target = strings.TrimSpace(target)
		if strings.TrimSpace(grpcReq.Message) != "" {
			expanded, err := resolver.ExpandTemplates(grpcReq.Message)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc message")
			}
			grpcReq.Message = expanded
		}
		if len(grpcReq.Metadata) > 0 {
			for key, value := range grpcReq.Metadata {
				expanded, err := resolver.ExpandTemplates(value)
				if err != nil {
					return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc metadata %s", key)
				}
				grpcReq.Metadata[key] = expanded
			}
		}
		if authority := strings.TrimSpace(grpcReq.Authority); authority != "" {
			expanded, err := resolver.ExpandTemplates(authority)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc authority")
			}
			grpcReq.Authority = strings.TrimSpace(expanded)
		}
		if descriptor := strings.TrimSpace(grpcReq.DescriptorSet); descriptor != "" {
			expanded, err := resolver.ExpandTemplates(descriptor)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc descriptor set")
			}
			grpcReq.DescriptorSet = strings.TrimSpace(expanded)
		}

		if req.Headers != nil {
			for key, values := range req.Headers {
				for i, value := range values {
					expanded, err := resolver.ExpandTemplates(value)
					if err != nil {
						return errdef.Wrap(errdef.CodeHTTP, err, "expand header %s", key)
					}
					req.Headers[key][i] = expanded
				}
			}
		}
	}

	grpcReq.Target = strings.TrimSpace(grpcReq.Target)
	grpcReq.Target = normalizeGRPCTarget(grpcReq.Target, grpcReq)
	if grpcReq.Target == "" {
		return errdef.New(errdef.CodeHTTP, "grpc target not specified")
	}
	req.URL = grpcReq.Target
	return nil
}

func normalizeGRPCTarget(target string, grpcReq *restfile.GRPCRequest) string {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return ""
	}

	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "grpcs://"):
		if grpcReq != nil && !grpcReq.PlaintextSet {
			grpcReq.Plaintext = false
			grpcReq.PlaintextSet = true
		}
		return trimmed[len("grpcs://"):]
	case strings.HasPrefix(lower, "https://"):
		if grpcReq != nil && !grpcReq.PlaintextSet {
			grpcReq.Plaintext = false
			grpcReq.PlaintextSet = true
		}
		return trimmed[len("https://"):]
	case strings.HasPrefix(lower, "grpc://"):
		return trimmed[len("grpc://"):]
	case strings.HasPrefix(lower, "http://"):
		return trimmed[len("http://"):]
	default:
		return trimmed
	}
}

func applyPreRequestOutput(req *restfile.Request, out scripts.PreRequestOutput) error {
	if out.Method != nil {
		req.Method = strings.ToUpper(strings.TrimSpace(*out.Method))
	}

	if out.URL != nil {
		req.URL = strings.TrimSpace(*out.URL)
	}

	if len(out.Query) > 0 {
		parsed, err := url.Parse(req.URL)
		if err != nil {
			return errdef.Wrap(errdef.CodeScript, err, "invalid url after script")
		}

		query := parsed.Query()
		for key, value := range out.Query {
			query.Set(key, value)
		}
		parsed.RawQuery = query.Encode()
		req.URL = parsed.String()
	}
	if out.Headers != nil {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		for name, values := range out.Headers {
			req.Headers.Del(name)
			for _, value := range values {
				req.Headers.Add(name, value)
			}
		}
	}
	if out.Body != nil {
		req.Body.FilePath = ""
		req.Body.Text = *out.Body
		req.Body.GraphQL = nil
	}
	if len(out.Variables) > 0 {
		existing := make(map[string]int)
		for i, v := range req.Variables {
			existing[strings.ToLower(v.Name)] = i
		}

		for name, value := range out.Variables {
			key := strings.ToLower(name)
			if idx, ok := existing[key]; ok {
				req.Variables[idx].Value = value
			} else {
				req.Variables = append(req.Variables, restfile.Variable{
					Name:  name,
					Value: value,
					Scope: restfile.ScopeRequest,
				})
			}
		}
	}
	return nil
}

func cloneRequest(req *restfile.Request) *restfile.Request {
	if req == nil {
		return nil
	}

	clone := *req
	clone.Headers = cloneHeader(req.Headers)
	if req.Settings != nil {
		clone.Settings = make(map[string]string, len(req.Settings))
		for k, v := range req.Settings {
			clone.Settings[k] = v
		}
	}

	clone.Variables = append([]restfile.Variable(nil), req.Variables...)
	clone.Metadata.Tags = append([]string(nil), req.Metadata.Tags...)
	clone.Metadata.Scripts = append([]restfile.ScriptBlock(nil), req.Metadata.Scripts...)
	clone.Metadata.Captures = append([]restfile.CaptureSpec(nil), req.Metadata.Captures...)
	if req.Metadata.Compare != nil {
		spec := *req.Metadata.Compare
		if len(spec.Environments) > 0 {
			spec.Environments = append([]string(nil), spec.Environments...)
		}
		clone.Metadata.Compare = &spec
	}
	if req.Body.GraphQL != nil {
		gql := *req.Body.GraphQL
		clone.Body.GraphQL = &gql
	}
	if req.GRPC != nil {
		grpcCopy := *req.GRPC
		if len(grpcCopy.Metadata) > 0 {
			meta := make(map[string]string, len(grpcCopy.Metadata))
			for k, v := range grpcCopy.Metadata {
				meta[k] = v
			}
			grpcCopy.Metadata = meta
		}
		clone.GRPC = &grpcCopy
	}
	if req.SSE != nil {
		sseCopy := *req.SSE
		clone.SSE = &sseCopy
	}
	if req.WebSocket != nil {
		wsCopy := *req.WebSocket
		if len(wsCopy.Options.Subprotocols) > 0 {
			protocols := make([]string, len(wsCopy.Options.Subprotocols))
			copy(protocols, wsCopy.Options.Subprotocols)
			wsCopy.Options.Subprotocols = protocols
		}
		if len(wsCopy.Steps) > 0 {
			steps := make([]restfile.WebSocketStep, len(wsCopy.Steps))
			copy(steps, wsCopy.Steps)
			wsCopy.Steps = steps
		}
		clone.WebSocket = &wsCopy
	}
	return &clone
}

func (m *Model) requestAtCursor(doc *restfile.Document, content string, cursorLine int) (*restfile.Request, bool) {
	var req *restfile.Request
	if doc != nil {
		req = findRequestAtLine(doc, cursorLine)
		if req != nil && (cursorLine < req.LineRange.Start || cursorLine > req.LineRange.End) {
			req = nil
		}
		if req != nil {
			return req, false
		}
	}
	if inline := buildInlineRequest(content, cursorLine); inline != nil {
		return inline, true
	}
	return nil, false
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}

	cloned := make(http.Header, len(h))
	for k, values := range h {
		cloned[k] = append([]string(nil), values...)
	}
	return cloned
}

func renderRequestText(req *restfile.Request) string {
	if req == nil {
		return ""
	}

	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("%s %s\n", req.Method, req.URL))
	headerNames := make([]string, 0, len(req.Headers))
	for name := range req.Headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		for _, value := range req.Headers[name] {
			builder.WriteString(fmt.Sprintf("%s: %s\n", name, value))
		}
	}

	builder.WriteString("\n")
	if req.WebSocket != nil {
		builder.WriteString(renderWebSocketSection(req.WebSocket))
	}
	if req.SSE != nil {
		builder.WriteString(renderSSESection(req.SSE))
	}
	if req.GRPC != nil {
		grpc := req.GRPC
		if grpc.FullMethod != "" {
			builder.WriteString("# @grpc ")
			builder.WriteString(strings.TrimPrefix(grpc.FullMethod, "/"))
			builder.WriteString("\n")
		}
		if grpc.DescriptorSet != "" {
			builder.WriteString("# @grpc-descriptor " + grpc.DescriptorSet + "\n")
		}
		if !grpc.UseReflection {
			builder.WriteString("# @grpc-reflection false\n")
		}
		if grpc.PlaintextSet {
			builder.WriteString(fmt.Sprintf("# @grpc-plaintext %t\n", grpc.Plaintext))
		}
		if grpc.Authority != "" {
			builder.WriteString("# @grpc-authority " + grpc.Authority + "\n")
		}
		if len(grpc.Metadata) > 0 {
			keys := make([]string, 0, len(grpc.Metadata))
			for k := range grpc.Metadata {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				builder.WriteString(fmt.Sprintf("# @grpc-metadata %s: %s\n", k, grpc.Metadata[k]))
			}
		}
		builder.WriteString("\n")
		if strings.TrimSpace(grpc.Message) != "" {
			builder.WriteString(grpc.Message)
			if !strings.HasSuffix(grpc.Message, "\n") {
				builder.WriteString("\n")
			}
		} else if strings.TrimSpace(grpc.MessageFile) != "" {
			builder.WriteString("< " + strings.TrimSpace(grpc.MessageFile) + "\n")
		}
	} else if req.Body.GraphQL != nil {
		gql := req.Body.GraphQL
		builder.WriteString("# @graphql\n")
		if strings.TrimSpace(gql.OperationName) != "" {
			builder.WriteString("# @operation " + strings.TrimSpace(gql.OperationName) + "\n")
		}

		if strings.TrimSpace(gql.Query) != "" {
			builder.WriteString(gql.Query)
			if !strings.HasSuffix(gql.Query, "\n") {
				builder.WriteString("\n")
			}
		} else if strings.TrimSpace(gql.QueryFile) != "" {
			builder.WriteString("< " + strings.TrimSpace(gql.QueryFile) + "\n")
		}

		if strings.TrimSpace(gql.Variables) != "" || strings.TrimSpace(gql.VariablesFile) != "" {
			builder.WriteString("\n# @variables\n")
			if strings.TrimSpace(gql.Variables) != "" {
				builder.WriteString(gql.Variables)
				if !strings.HasSuffix(gql.Variables, "\n") {
					builder.WriteString("\n")
				}
			} else if strings.TrimSpace(gql.VariablesFile) != "" {
				builder.WriteString("< " + strings.TrimSpace(gql.VariablesFile) + "\n")
			}
		}
	} else if req.Body.FilePath != "" {
		builder.WriteString("< " + req.Body.FilePath + "\n")
	} else if strings.TrimSpace(req.Body.Text) != "" {
		builder.WriteString(req.Body.Text)
		if !strings.HasSuffix(req.Body.Text, "\n") {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func renderSSESection(sse *restfile.SSERequest) string {
	if sse == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if sse.Options.TotalTimeout > 0 {
		parts = append(parts, fmt.Sprintf("duration=%s", sse.Options.TotalTimeout))
	}
	if sse.Options.IdleTimeout > 0 {
		parts = append(parts, fmt.Sprintf("idle=%s", sse.Options.IdleTimeout))
	}
	if sse.Options.MaxEvents > 0 {
		parts = append(parts, fmt.Sprintf("max-events=%d", sse.Options.MaxEvents))
	}
	if sse.Options.MaxBytes > 0 {
		parts = append(parts, fmt.Sprintf("max-bytes=%d", sse.Options.MaxBytes))
	}
	line := "# @sse"
	if len(parts) > 0 {
		line += " " + strings.Join(parts, " ")
	}
	return line + "\n\n"
}

func renderWebSocketSection(ws *restfile.WebSocketRequest) string {
	if ws == nil {
		return ""
	}
	lines := []string{renderWebSocketDirectiveLine(ws.Options)}
	for _, step := range ws.Steps {
		if line := renderWebSocketStepLine(step); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func renderWebSocketDirectiveLine(opts restfile.WebSocketOptions) string {
	parts := make([]string, 0, 5)
	if opts.HandshakeTimeout > 0 {
		parts = append(parts, fmt.Sprintf("timeout=%s", opts.HandshakeTimeout))
	}
	if opts.ReceiveTimeout > 0 {
		parts = append(parts, fmt.Sprintf("receive=%s", opts.ReceiveTimeout))
	}
	if opts.MaxMessageBytes > 0 {
		parts = append(parts, fmt.Sprintf("max-message-bytes=%d", opts.MaxMessageBytes))
	}
	if len(opts.Subprotocols) > 0 {
		parts = append(parts, fmt.Sprintf("subprotocols=%s", strings.Join(opts.Subprotocols, ",")))
	}
	if opts.CompressionSet {
		parts = append(parts, fmt.Sprintf("compression=%t", opts.Compression))
	}
	line := "# @websocket"
	if len(parts) > 0 {
		line += " " + strings.Join(parts, " ")
	}
	return line
}

func renderWebSocketStepLine(step restfile.WebSocketStep) string {
	prefix := "# @ws "
	switch step.Type {
	case restfile.WebSocketStepSendText:
		return prefix + "send " + step.Value
	case restfile.WebSocketStepSendJSON:
		return prefix + "send-json " + step.Value
	case restfile.WebSocketStepSendBase64:
		return prefix + "send-base64 " + step.Value
	case restfile.WebSocketStepSendFile:
		if step.File == "" {
			return ""
		}
		return prefix + "send-file " + step.File
	case restfile.WebSocketStepPing:
		if strings.TrimSpace(step.Value) == "" {
			return prefix + "ping"
		}
		return prefix + "ping " + step.Value
	case restfile.WebSocketStepPong:
		if strings.TrimSpace(step.Value) == "" {
			return prefix + "pong"
		}
		return prefix + "pong " + step.Value
	case restfile.WebSocketStepWait:
		return prefix + "wait " + step.Duration.String()
	case restfile.WebSocketStepClose:
		code := step.Code
		if code == 0 {
			if strings.TrimSpace(step.Reason) == "" {
				return prefix + "close"
			}
			return prefix + "close " + step.Reason
		}
		reason := strings.TrimSpace(step.Reason)
		if reason == "" {
			return fmt.Sprintf("%sclose %d", prefix, code)
		}
		return fmt.Sprintf("%sclose %d %s", prefix, code, reason)
	default:
		return ""
	}
}

func buildInlineRequest(content string, lineNumber int) *restfile.Request {
	if lineNumber < 1 {
		return nil
	}

	lines := strings.Split(content, "\n")
	if req := inlineCurlRequest(lines, lineNumber); req != nil {
		return req
	}

	if lineNumber > len(lines) {
		return nil
	}
	return inlineRequestFromLine(lines[lineNumber-1], lineNumber)
}

func inlineCurlRequest(lines []string, lineNumber int) *restfile.Request {
	idx := lineNumber - 1
	if idx < 0 || idx >= len(lines) {
		return nil
	}

	start, end, command := extractCurlCommand(lines, idx)
	if command == "" {
		return nil
	}

	parsed, err := curl.ParseCommand(command)
	if err != nil {
		return nil
	}
	parsed.LineRange = restfile.LineRange{Start: start + 1, End: end + 1}
	parsed.OriginalText = strings.Join(lines[start:end+1], "\n")
	return parsed
}

func extractCurlCommand(lines []string, cursorIdx int) (start int, end int, command string) {
	start = -1
	for i := cursorIdx; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if i == cursorIdx {
				continue
			}
			break
		}
		if isCurlStartLine(trimmed) {
			start = i
			break
		}
	}
	if start == -1 {
		return -1, -1, ""
	}

	qs := &curlQuoteState{}
	var b strings.Builder
	end = start
	for i := start; i < len(lines); i++ {
		line := lines[i]
		openBefore := qs.open()
		if strings.TrimSpace(line) == "" && i > start && !openBefore {
			break
		}

		seg := line
		if !openBefore {
			seg = strings.TrimLeft(seg, " \t")
		}
		if !openBefore {
			seg = strings.TrimRight(seg, " \t")
		}
		if seg == "" {
			seg = ""
		}

		cont := curlLineContinues(seg)
		if cont {
			seg = seg[:len(seg)-1]
		}

		if b.Len() > 0 {
			if openBefore {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		}

		b.WriteString(seg)
		qs.consume(seg)
		end = i
		if cont {
			qs.resetEscape()
			continue
		}
		if qs.open() {
			continue
		}
		break
	}

	return start, end, strings.TrimSpace(b.String())
}

type curlQuoteState struct {
	ins bool
	ind bool
	esc bool
}

func (s *curlQuoteState) consume(v string) {
	for _, r := range v {
		if s.esc {
			s.esc = false
			continue
		}
		switch r {
		case '\\':
			if s.ins {
				continue
			}
			s.esc = true
		case '\'':
			if s.ind {
				continue
			}
			s.ins = !s.ins
		case '"':
			if s.ins {
				continue
			}
			s.ind = !s.ind
		}
	}
}

func (s *curlQuoteState) open() bool {
	return s.ins || s.ind
}

func (s *curlQuoteState) resetEscape() {
	s.esc = false
}

func curlLineContinues(v string) bool {
	if v == "" {
		return false
	}
	count := 0
	for i := len(v) - 1; i >= 0 && v[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

func isCurlStartLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "sudo ")
	trimmed = strings.TrimSpace(trimmed)
	if strings.HasPrefix(trimmed, "curl ") || trimmed == "curl" {
		return true
	}
	for _, prefix := range []string{"$", "%", ">", "!"} {
		prefixed := prefix + " "
		if strings.HasPrefix(trimmed, prefixed) {
			candidate := strings.TrimSpace(trimmed[len(prefixed):])
			if strings.HasPrefix(candidate, "curl ") || candidate == "curl" {
				return true
			}
		}
	}
	return false
}

func inlineRequestFromLine(raw string, lineNumber int) *restfile.Request {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	method := "GET"
	url := ""

	fields := strings.Fields(trimmed)
	if len(fields) == 1 {
		url = fields[0]
	} else if len(fields) >= 2 {
		candidate := strings.ToUpper(fields[0])
		if isInlineHTTPMethod(candidate) {
			method = candidate
			url = fields[1]
		}
	}

	if url == "" {
		url = trimmed
	}

	url = strings.Trim(url, "\"'")
	if !looksLikeHTTPRequestURL(url) {
		return nil
	}

	return &restfile.Request{
		Method: method,
		URL:    url,
		LineRange: restfile.LineRange{
			Start: lineNumber,
			End:   lineNumber,
		},
		OriginalText: raw,
	}
}

func isInlineHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func looksLikeHTTPRequestURL(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ws://") ||
		strings.HasPrefix(lower, "wss://")
}
