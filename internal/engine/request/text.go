package request

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func RenderRequestText(req *restfile.Request) string { return renderRequestText(req) }

func renderRequestText(req *restfile.Request) string {
	if req == nil {
		return ""
	}

	var w requestTextWriter
	w.writeRequestLine(req.Method, req.URL)
	w.writeHeaders(req.Headers)
	w.blankLine()
	w.writeWebSocketSection(req.WebSocket)
	w.writeSSESection(req.SSE)

	switch {
	case req.GRPC != nil:
		w.writeGRPCSection(req.GRPC)
	case req.Body.GraphQL != nil:
		w.writeGraphQLSection(req.Body.GraphQL)
	case req.Body.FilePath != "":
		w.writeFileReference(req.Body.FilePath)
	case strings.TrimSpace(req.Body.Text) != "":
		w.writeTextBlock(req.Body.Text)
	}

	return w.String()
}

type requestTextWriter struct {
	strings.Builder
}

func (w *requestTextWriter) writeRequestLine(method, url string) {
	fmt.Fprintf(&w.Builder, "%s %s\n", method, url)
}

func (w *requestTextWriter) writeHeaders(headers http.Header) {
	headerNames := make([]string, 0, len(headers))
	for name := range headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)

	for _, name := range headerNames {
		for _, value := range headers[name] {
			w.writeLine(fmt.Sprintf("%s: %s", name, value))
		}
	}
}

func (w *requestTextWriter) writeWebSocketSection(ws *restfile.WebSocketRequest) {
	if ws == nil {
		return
	}

	w.writeLine(renderWebSocketDirectiveLine(ws.Options))
	for _, step := range ws.Steps {
		if line := renderWebSocketStepLine(step); line != "" {
			w.writeLine(line)
		}
	}
	w.blankLine()
}

func (w *requestTextWriter) writeSSESection(sse *restfile.SSERequest) {
	if sse == nil {
		return
	}

	w.writeLine(renderSSEDirectiveLine(sse.Options))
	w.blankLine()
}

func (w *requestTextWriter) writeGRPCSection(grpc *restfile.GRPCRequest) {
	if grpc == nil {
		return
	}

	if grpc.FullMethod != "" {
		w.writeLine("# @grpc " + strings.TrimPrefix(grpc.FullMethod, "/"))
	}
	if grpc.DescriptorSet != "" {
		w.writeLine("# @grpc-descriptor " + grpc.DescriptorSet)
	}
	if !grpc.UseReflection {
		w.writeLine("# @grpc-reflection false")
	}
	if grpc.PlaintextSet {
		w.writeLine(fmt.Sprintf("# @grpc-plaintext %t", grpc.Plaintext))
	}
	if grpc.Authority != "" {
		w.writeLine("# @grpc-authority " + grpc.Authority)
	}
	for _, pair := range grpc.Metadata {
		w.writeLine(fmt.Sprintf("# @grpc-metadata %s: %s", pair.Key, pair.Value))
	}

	w.blankLine()
	switch {
	case strings.TrimSpace(grpc.Message) != "":
		w.writeTextBlock(grpc.Message)
	case strings.TrimSpace(grpc.MessageFile) != "":
		w.writeFileReference(strings.TrimSpace(grpc.MessageFile))
	}
}

func (w *requestTextWriter) writeGraphQLSection(gql *restfile.GraphQLBody) {
	if gql == nil {
		return
	}

	w.writeLine("# @graphql")
	if strings.TrimSpace(gql.OperationName) != "" {
		w.writeLine("# @operation " + strings.TrimSpace(gql.OperationName))
	}

	switch {
	case strings.TrimSpace(gql.Query) != "":
		w.writeTextBlock(gql.Query)
	case strings.TrimSpace(gql.QueryFile) != "":
		w.writeFileReference(strings.TrimSpace(gql.QueryFile))
	}

	if strings.TrimSpace(gql.Variables) == "" && strings.TrimSpace(gql.VariablesFile) == "" {
		return
	}

	w.blankLine()
	w.writeLine("# @variables")
	switch {
	case strings.TrimSpace(gql.Variables) != "":
		w.writeTextBlock(gql.Variables)
	case strings.TrimSpace(gql.VariablesFile) != "":
		w.writeFileReference(strings.TrimSpace(gql.VariablesFile))
	}
}

func (w *requestTextWriter) writeTextBlock(text string) {
	w.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		w.WriteByte('\n')
	}
}

func (w *requestTextWriter) writeFileReference(path string) {
	w.writeLine("< " + path)
}

func (w *requestTextWriter) writeLine(line string) {
	w.WriteString(line)
	w.WriteByte('\n')
}

func (w *requestTextWriter) blankLine() {
	w.WriteByte('\n')
}

func renderSSEDirectiveLine(opts restfile.SSEOptions) string {
	return renderDirectiveLine(
		"# @sse",
		durationDirectivePart("duration", opts.TotalTimeout),
		durationDirectivePart("idle", opts.IdleTimeout),
		intDirectivePart("max-events", opts.MaxEvents),
		int64DirectivePart("max-bytes", opts.MaxBytes),
	)
}

func renderWebSocketDirectiveLine(opts restfile.WebSocketOptions) string {
	return renderDirectiveLine(
		"# @websocket",
		durationDirectivePart("timeout", opts.HandshakeTimeout),
		durationDirectivePart("idle", opts.IdleTimeout),
		int64DirectivePart("max-message-bytes", opts.MaxMessageBytes),
		csvDirectivePart("subprotocols", opts.Subprotocols),
		boolDirectivePart("compression", opts.Compression, opts.CompressionSet),
	)
}

func renderDirectiveLine(name string, parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return name
	}
	return name + " " + strings.Join(filtered, " ")
}

func durationDirectivePart(name string, value time.Duration) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%s=%s", name, value)
}

func intDirectivePart(name string, value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%s=%d", name, value)
}

func int64DirectivePart(name string, value int64) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%s=%d", name, value)
}

func csvDirectivePart(name string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	return fmt.Sprintf("%s=%s", name, strings.Join(values, ","))
}

func boolDirectivePart(name string, value, set bool) string {
	if !set {
		return ""
	}
	return fmt.Sprintf("%s=%t", name, value)
}

func renderWebSocketStepLine(st restfile.WebSocketStep) string {
	const prefix = "# @ws "

	switch st.Type {
	case restfile.WebSocketStepSendText:
		return prefix + "send " + st.Value
	case restfile.WebSocketStepSendJSON:
		return prefix + "send-json " + st.Value
	case restfile.WebSocketStepSendBase64:
		return prefix + "send-base64 " + st.Value
	case restfile.WebSocketStepSendFile:
		if st.File == "" {
			return ""
		}
		return prefix + "send-file " + st.File
	case restfile.WebSocketStepPing:
		if strings.TrimSpace(st.Value) == "" {
			return prefix + "ping"
		}
		return prefix + "ping " + st.Value
	case restfile.WebSocketStepPong:
		if strings.TrimSpace(st.Value) == "" {
			return prefix + "pong"
		}
		return prefix + "pong " + st.Value
	case restfile.WebSocketStepWait:
		return prefix + "wait " + st.Duration.String()
	case restfile.WebSocketStepClose:
		if st.Code == 0 {
			if strings.TrimSpace(st.Reason) == "" {
				return prefix + "close"
			}
			return prefix + "close " + st.Reason
		}

		reason := strings.TrimSpace(st.Reason)
		if reason == "" {
			return fmt.Sprintf("%sclose %d", prefix, st.Code)
		}
		return fmt.Sprintf("%sclose %d %s", prefix, st.Code, reason)
	default:
		return ""
	}
}
