package ui

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/curl"
	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

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
	clone.Metadata.Auth = restfile.CloneAuthSpec(req.Metadata.Auth)
	clone.Metadata.Scripts = restfile.CloneScriptBlocks(req.Metadata.Scripts)
	clone.Metadata.Uses = append([]restfile.UseSpec(nil), req.Metadata.Uses...)
	if len(req.Metadata.Applies) > 0 {
		clone.Metadata.Applies = make([]restfile.ApplySpec, len(req.Metadata.Applies))
		copy(clone.Metadata.Applies, req.Metadata.Applies)
		for i := range clone.Metadata.Applies {
			clone.Metadata.Applies[i].Uses = append(
				[]string(nil),
				req.Metadata.Applies[i].Uses...,
			)
		}
	}
	clone.Metadata.Asserts = append([]restfile.AssertSpec(nil), req.Metadata.Asserts...)
	clone.Metadata.Captures = append([]restfile.CaptureSpec(nil), req.Metadata.Captures...)
	if req.Metadata.When != nil {
		when := *req.Metadata.When
		clone.Metadata.When = &when
	}
	if req.Metadata.ForEach != nil {
		forEach := *req.Metadata.ForEach
		clone.Metadata.ForEach = &forEach
	}
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
			meta := make([]restfile.MetadataPair, len(grpcCopy.Metadata))
			copy(meta, grpcCopy.Metadata)
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

func cloneRequestIf(req *restfile.Request, enabled bool) *restfile.Request {
	if !enabled {
		return nil
	}
	return cloneRequest(req)
}

func (m *Model) requestAtCursor(
	doc *restfile.Document,
	content string,
	cursorLine int,
) (*restfile.Request, bool) {
	if req, _ := requestAtLine(doc, cursorLine); req != nil {
		return req, false
	}
	if inline := buildInlineRequest(content, cursorLine); inline != nil {
		return inline, true
	}
	if doc != nil && len(doc.Requests) > 0 {
		last := doc.Requests[len(doc.Requests)-1]
		if last != nil && cursorLine > last.LineRange.End {
			return last, false
		}
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
	fmt.Fprintf(&builder, "%s %s\n", req.Method, req.URL)
	headerNames := make([]string, 0, len(req.Headers))
	for name := range req.Headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		for _, value := range req.Headers[name] {
			fmt.Fprintf(&builder, "%s: %s\n", name, value)
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
			fmt.Fprintf(&builder, "# @grpc-plaintext %t\n", grpc.Plaintext)
		}
		if grpc.Authority != "" {
			builder.WriteString("# @grpc-authority " + grpc.Authority + "\n")
		}
		if len(grpc.Metadata) > 0 {
			for _, pair := range grpc.Metadata {
				fmt.Fprintf(&builder, "# @grpc-metadata %s: %s\n", pair.Key, pair.Value)
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
	if opts.IdleTimeout > 0 {
		parts = append(parts, fmt.Sprintf("idle=%s", opts.IdleTimeout))
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
	return curl.ExtractCommand(lines, cursorIdx)
}

func inlineRequestFromLine(raw string, lineNumber int) *restfile.Request {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	method := "GET"
	url := ""

	fields := strings.Fields(trimmed)
	fields, ver := httpver.SplitToken(fields)
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
		url = strings.Join(fields, " ")
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
		Settings:     httpver.SetIfMissing(nil, ver),
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
