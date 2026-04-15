package request

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func CloneRequest(req *restfile.Request) *restfile.Request { return cloneRequest(req) }

func RenderRequestText(req *restfile.Request) string { return renderRequestText(req) }

func cloneRequest(req *restfile.Request) *restfile.Request {
	if req == nil {
		return nil
	}

	cp := *req
	cp.Headers = cloneHeader(req.Headers)
	if req.Settings != nil {
		cp.Settings = make(map[string]string, len(req.Settings))
		for k, v := range req.Settings {
			cp.Settings[k] = v
		}
	}

	cp.Variables = append([]restfile.Variable(nil), req.Variables...)
	cp.Metadata.Tags = append([]string(nil), req.Metadata.Tags...)
	cp.Metadata.Auth = restfile.CloneAuthSpec(req.Metadata.Auth)
	cp.Metadata.Scripts = append([]restfile.ScriptBlock(nil), req.Metadata.Scripts...)
	cp.Metadata.Uses = append([]restfile.UseSpec(nil), req.Metadata.Uses...)
	cp.Metadata.Asserts = append([]restfile.AssertSpec(nil), req.Metadata.Asserts...)
	cp.Metadata.Captures = append([]restfile.CaptureSpec(nil), req.Metadata.Captures...)
	if len(req.Metadata.Applies) > 0 {
		cp.Metadata.Applies = make([]restfile.ApplySpec, len(req.Metadata.Applies))
		copy(cp.Metadata.Applies, req.Metadata.Applies)
		for i := range cp.Metadata.Applies {
			cp.Metadata.Applies[i].Uses = append([]string(nil), req.Metadata.Applies[i].Uses...)
		}
	}
	if req.Metadata.When != nil {
		v := *req.Metadata.When
		cp.Metadata.When = &v
	}
	if req.Metadata.ForEach != nil {
		v := *req.Metadata.ForEach
		cp.Metadata.ForEach = &v
	}
	if req.Metadata.Compare != nil {
		v := *req.Metadata.Compare
		if len(v.Environments) > 0 {
			v.Environments = append([]string(nil), v.Environments...)
		}
		cp.Metadata.Compare = &v
	}
	if req.Metadata.Profile != nil {
		v := *req.Metadata.Profile
		cp.Metadata.Profile = &v
	}
	if req.Metadata.Trace != nil {
		v := *req.Metadata.Trace
		cp.Metadata.Trace = &v
	}
	if req.Body.GraphQL != nil {
		v := *req.Body.GraphQL
		cp.Body.GraphQL = &v
	}
	if req.GRPC != nil {
		v := *req.GRPC
		if len(v.Metadata) > 0 {
			v.Metadata = append([]restfile.MetadataPair(nil), v.Metadata...)
		}
		cp.GRPC = &v
	}
	if req.SSE != nil {
		v := *req.SSE
		cp.SSE = &v
	}
	if req.WebSocket != nil {
		v := *req.WebSocket
		if len(v.Options.Subprotocols) > 0 {
			v.Options.Subprotocols = append([]string(nil), v.Options.Subprotocols...)
		}
		if len(v.Steps) > 0 {
			v.Steps = append([]restfile.WebSocketStep(nil), v.Steps...)
		}
		cp.WebSocket = &v
	}
	return engine.NormReq(&cp)
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	cp := make(http.Header, len(h))
	for k, v := range h {
		cp[k] = append([]string(nil), v...)
	}
	return cp
}

func cloneValueMap(src map[string]rts.Value) map[string]rts.Value {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]rts.Value, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func renderRequestText(req *restfile.Request) string {
	if req == nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", req.Method, req.URL)
	ns := make([]string, 0, len(req.Headers))
	for n := range req.Headers {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	for _, n := range ns {
		for _, v := range req.Headers[n] {
			fmt.Fprintf(&b, "%s: %s\n", n, v)
		}
	}

	b.WriteString("\n")
	if req.WebSocket != nil {
		b.WriteString(renderWebSocketSection(req.WebSocket))
	}
	if req.SSE != nil {
		b.WriteString(renderSSESection(req.SSE))
	}
	if req.GRPC != nil {
		grpc := req.GRPC
		if grpc.FullMethod != "" {
			b.WriteString("# @grpc ")
			b.WriteString(strings.TrimPrefix(grpc.FullMethod, "/"))
			b.WriteString("\n")
		}
		if grpc.DescriptorSet != "" {
			b.WriteString("# @grpc-descriptor " + grpc.DescriptorSet + "\n")
		}
		if !grpc.UseReflection {
			b.WriteString("# @grpc-reflection false\n")
		}
		if grpc.PlaintextSet {
			fmt.Fprintf(&b, "# @grpc-plaintext %t\n", grpc.Plaintext)
		}
		if grpc.Authority != "" {
			b.WriteString("# @grpc-authority " + grpc.Authority + "\n")
		}
		if len(grpc.Metadata) > 0 {
			for _, p := range grpc.Metadata {
				fmt.Fprintf(&b, "# @grpc-metadata %s: %s\n", p.Key, p.Value)
			}
		}
		b.WriteString("\n")
		switch {
		case strings.TrimSpace(grpc.Message) != "":
			b.WriteString(grpc.Message)
			if !strings.HasSuffix(grpc.Message, "\n") {
				b.WriteString("\n")
			}
		case strings.TrimSpace(grpc.MessageFile) != "":
			b.WriteString("< " + strings.TrimSpace(grpc.MessageFile) + "\n")
		}
		return b.String()
	}
	if req.Body.GraphQL != nil {
		gql := req.Body.GraphQL
		b.WriteString("# @graphql\n")
		if strings.TrimSpace(gql.OperationName) != "" {
			b.WriteString("# @operation " + strings.TrimSpace(gql.OperationName) + "\n")
		}
		switch {
		case strings.TrimSpace(gql.Query) != "":
			b.WriteString(gql.Query)
			if !strings.HasSuffix(gql.Query, "\n") {
				b.WriteString("\n")
			}
		case strings.TrimSpace(gql.QueryFile) != "":
			b.WriteString("< " + strings.TrimSpace(gql.QueryFile) + "\n")
		}
		if strings.TrimSpace(gql.Variables) != "" || strings.TrimSpace(gql.VariablesFile) != "" {
			b.WriteString("\n# @variables\n")
			switch {
			case strings.TrimSpace(gql.Variables) != "":
				b.WriteString(gql.Variables)
				if !strings.HasSuffix(gql.Variables, "\n") {
					b.WriteString("\n")
				}
			case strings.TrimSpace(gql.VariablesFile) != "":
				b.WriteString("< " + strings.TrimSpace(gql.VariablesFile) + "\n")
			}
		}
		return b.String()
	}
	if req.Body.FilePath != "" {
		b.WriteString("< " + req.Body.FilePath + "\n")
		return b.String()
	}
	if strings.TrimSpace(req.Body.Text) != "" {
		b.WriteString(req.Body.Text)
		if !strings.HasSuffix(req.Body.Text, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderSSESection(sse *restfile.SSERequest) string {
	if sse == nil {
		return ""
	}
	ps := make([]string, 0, 4)
	if sse.Options.TotalTimeout > 0 {
		ps = append(ps, fmt.Sprintf("duration=%s", sse.Options.TotalTimeout))
	}
	if sse.Options.IdleTimeout > 0 {
		ps = append(ps, fmt.Sprintf("idle=%s", sse.Options.IdleTimeout))
	}
	if sse.Options.MaxEvents > 0 {
		ps = append(ps, fmt.Sprintf("max-events=%d", sse.Options.MaxEvents))
	}
	if sse.Options.MaxBytes > 0 {
		ps = append(ps, fmt.Sprintf("max-bytes=%d", sse.Options.MaxBytes))
	}
	ln := "# @sse"
	if len(ps) > 0 {
		ln += " " + strings.Join(ps, " ")
	}
	return ln + "\n\n"
}

func renderWebSocketSection(ws *restfile.WebSocketRequest) string {
	if ws == nil {
		return ""
	}
	ls := []string{renderWebSocketDirectiveLine(ws.Options)}
	for _, st := range ws.Steps {
		if ln := renderWebSocketStepLine(st); ln != "" {
			ls = append(ls, ln)
		}
	}
	return strings.Join(ls, "\n") + "\n\n"
}

func renderWebSocketDirectiveLine(opts restfile.WebSocketOptions) string {
	ps := make([]string, 0, 5)
	if opts.HandshakeTimeout > 0 {
		ps = append(ps, fmt.Sprintf("timeout=%s", opts.HandshakeTimeout))
	}
	if opts.IdleTimeout > 0 {
		ps = append(ps, fmt.Sprintf("idle=%s", opts.IdleTimeout))
	}
	if opts.MaxMessageBytes > 0 {
		ps = append(ps, fmt.Sprintf("max-message-bytes=%d", opts.MaxMessageBytes))
	}
	if len(opts.Subprotocols) > 0 {
		ps = append(ps, fmt.Sprintf("subprotocols=%s", strings.Join(opts.Subprotocols, ",")))
	}
	if opts.CompressionSet {
		ps = append(ps, fmt.Sprintf("compression=%t", opts.Compression))
	}
	ln := "# @websocket"
	if len(ps) > 0 {
		ln += " " + strings.Join(ps, " ")
	}
	return ln
}

func renderWebSocketStepLine(st restfile.WebSocketStep) string {
	pfx := "# @ws "
	switch st.Type {
	case restfile.WebSocketStepSendText:
		return pfx + "send " + st.Value
	case restfile.WebSocketStepSendJSON:
		return pfx + "send-json " + st.Value
	case restfile.WebSocketStepSendBase64:
		return pfx + "send-base64 " + st.Value
	case restfile.WebSocketStepSendFile:
		if st.File == "" {
			return ""
		}
		return pfx + "send-file " + st.File
	case restfile.WebSocketStepPing:
		if strings.TrimSpace(st.Value) == "" {
			return pfx + "ping"
		}
		return pfx + "ping " + st.Value
	case restfile.WebSocketStepPong:
		if strings.TrimSpace(st.Value) == "" {
			return pfx + "pong"
		}
		return pfx + "pong " + st.Value
	case restfile.WebSocketStepWait:
		return pfx + "wait " + st.Duration.String()
	case restfile.WebSocketStepClose:
		code := st.Code
		if code == 0 {
			if strings.TrimSpace(st.Reason) == "" {
				return pfx + "close"
			}
			return pfx + "close " + st.Reason
		}
		reason := strings.TrimSpace(st.Reason)
		if reason == "" {
			return fmt.Sprintf("%sclose %d", pfx, code)
		}
		return fmt.Sprintf("%sclose %d %s", pfx, code, reason)
	default:
		return ""
	}
}

func copyBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	return append([]byte(nil), src...)
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeStringMaps(xs ...map[string]string) map[string]string {
	n := 0
	for _, x := range xs {
		n += len(x)
	}
	if n == 0 {
		return nil
	}
	out := make(map[string]string, n)
	for _, x := range xs {
		for k, v := range x {
			out[k] = v
		}
	}
	return out
}
