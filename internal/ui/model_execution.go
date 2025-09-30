package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/parser/curl"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) sendActiveRequest() tea.Cmd {
	content := m.editor.Value()
	doc := parser.Parse(m.currentFile, []byte(content))
	cursorLine := currentCursorLine(m.editor)
	req := findRequestAtLine(doc, cursorLine)
	if req != nil && (cursorLine < req.LineRange.Start || cursorLine > req.LineRange.End) {
		req = nil
	}
	if req == nil {
		if inline := buildInlineRequest(content, cursorLine); inline != nil {
			req = inline
		} else {
			return func() tea.Msg {
				return statusMsg{text: "No request at cursor", level: statusWarn}
			}
		}
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(req)

	cloned := cloneRequest(req)
	m.currentRequest = cloned
	m.testResults = nil
	m.scriptError = nil

	options := m.cfg.HTTPOptions
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}

	m.sending = true
	base := fmt.Sprintf("Sending %s", cloned.URL)
	m.statusPulseBase = base
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: base, level: statusInfo})

	execCmd := m.executeRequest(doc, cloned, options)
	if tick := m.scheduleStatusPulse(); tick != nil {
		return tea.Batch(execCmd, tick)
	}
	return execCmd
}

func (m *Model) executeRequest(doc *restfile.Document, req *restfile.Request, options httpclient.Options) tea.Cmd {
	client := m.client
	runner := m.scriptRunner
	envName := m.cfg.EnvironmentName
	baseVars := m.collectVariables(doc, req)

	return func() tea.Msg {
		preVars := cloneStringMap(baseVars)
		preResult, err := runner.RunPreRequest(req.Metadata.Scripts, scripts.PreRequestInput{
			Request:   req,
			Variables: preVars,
		})
		if err != nil {
			return responseMsg{err: errdef.Wrap(errdef.CodeScript, err, "pre-request script")}
		}

		if err := applyPreRequestOutput(req, preResult); err != nil {
			return responseMsg{err: err}
		}

		scriptVars := cloneStringMap(preResult.Variables)
		resolver := m.buildResolver(doc, req, scriptVars)

		if req.GRPC != nil {
			if err := m.prepareGRPCRequest(req, resolver); err != nil {
				return responseMsg{err: err}
			}
		}

		timeout := defaultTimeout(options.Timeout)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if req.GRPC != nil {
			grpcOpts := m.grpcOptions
			if grpcOpts.BaseDir == "" {
				grpcOpts.BaseDir = options.BaseDir
				if grpcOpts.BaseDir == "" && m.currentFile != "" {
					grpcOpts.BaseDir = filepath.Dir(m.currentFile)
				}
			}

			if grpcOpts.DialTimeout == 0 {
				grpcOpts.DialTimeout = timeout
			}

			grpcResp, grpcErr := m.grpcClient.Execute(ctx, req, req.GRPC, grpcOpts)
			return responseMsg{
				grpc:        grpcResp,
				err:         grpcErr,
				executed:    req,
				requestText: renderRequestText(req),
				environment: envName,
			}
		}

		response, err := client.Execute(ctx, req, resolver, options)
		if err != nil {
			return responseMsg{err: err}
		}

		testVars := mergeVariableMaps(baseVars, scriptVars)
		tests, testErr := runner.RunTests(req.Metadata.Scripts, scripts.TestInput{
			Response:  response,
			Variables: testVars,
		})

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

const statusPulseInterval = 300 * time.Millisecond

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

func (m *Model) buildResolver(doc *restfile.Document, req *restfile.Request, extras ...map[string]string) *vars.Resolver {
	providers := make([]vars.Provider, 0, 6)

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

	fileVars := make(map[string]string)
	for _, v := range doc.Variables {
		fileVars[v.Name] = v.Value
	}
	if len(fileVars) > 0 {
		providers = append(providers, vars.NewMapProvider("file", fileVars))
	}

	if envValues := m.environmentValues(); len(envValues) > 0 {
		providers = append(providers, vars.NewMapProvider("environment", envValues))
	}

	providers = append(providers, vars.EnvProvider{})
	return vars.NewResolver(providers...)
}

func (m *Model) environmentValues() map[string]string {
	if m.cfg.EnvironmentSet == nil || m.cfg.EnvironmentName == "" {
		return nil
	}
	if env, ok := m.cfg.EnvironmentSet[m.cfg.EnvironmentName]; ok {
		return env
	}
	return nil
}

func (m *Model) collectVariables(doc *restfile.Document, req *restfile.Request) map[string]string {
	vars := make(map[string]string)
	if env := m.environmentValues(); env != nil {
		for k, v := range env {
			vars[k] = v
		}
	}
	if doc != nil {
		for _, v := range doc.Variables {
			vars[v.Name] = v.Value
		}
	}
	if req != nil {
		for _, v := range req.Variables {
			vars[v.Name] = v.Value
		}
	}
	return vars
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
	if grpcReq.Target == "" {
		return errdef.New(errdef.CodeHTTP, "grpc target not specified")
	}
	req.URL = grpcReq.Target
	return nil
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
	return &clone
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
	var builder strings.Builder
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
		if !continuesCurl(trimmed) {
			return -1, -1, ""
		}
	}
	if start == -1 {
		return -1, -1, ""
	}

	builder.Reset()
	end = start
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" && i > start {
			break
		}

		end = i
		stripped := strings.TrimLeft(line, " \t")
		tail := strings.TrimRight(stripped, " \t")
		continued := false
		if strings.HasSuffix(tail, "\\") && !strings.HasSuffix(tail, "\\\\") {
			continued = true
			stripped = strings.TrimSuffix(tail, "\\")
		}

		builder.WriteString(stripped)
		if continued {
			builder.WriteByte(' ')
			continue
		}
		break
	}
	return start, end, strings.TrimSpace(builder.String())
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

func continuesCurl(line string) bool {
	if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "--") {
		return true
	}
	if strings.HasSuffix(line, "\\") {
		return true
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
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
