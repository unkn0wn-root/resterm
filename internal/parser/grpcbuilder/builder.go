package grpcbuilder

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Builder struct {
	request         *restfile.GRPCRequest
	messageLines    []string
	messageFromFile string
}

func New() *Builder {
	return &Builder{}
}

func IsMethodLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false
	}
	return strings.EqualFold(fields[0], "GRPC")
}

func (b *Builder) EnsureRequest() *restfile.GRPCRequest {
	if b.request == nil {
		b.request = &restfile.GRPCRequest{Metadata: map[string]string{}, UseReflection: true}
	} else if b.request.Metadata == nil {
		b.request.Metadata = map[string]string{}
	}
	return b.request
}

func (b *Builder) SetTarget(target string) {
	req := b.EnsureRequest()
	req.Target = strings.TrimSpace(target)
}

func (b *Builder) HandleDirective(key, rest string) bool {
	switch key {
	case "grpc":
		req := b.EnsureRequest()
		if rest == "" {
			return true
		}

		pkg, service, method := parseMethod(rest)
		if service != "" && method != "" {
			req.Package = pkg
			req.Service = service
			req.Method = method
			if pkg != "" {
				req.FullMethod = "/" + pkg + "." + service + "/" + method
			} else {
				req.FullMethod = "/" + service + "/" + method
			}
		}
		return true
	case "grpc-descriptor":
		b.EnsureRequest().DescriptorSet = rest
		return true
	case "grpc-reflection":
		req := b.EnsureRequest()
		if rest == "" {
			req.UseReflection = true
		} else if strings.EqualFold(rest, "false") || strings.EqualFold(rest, "0") {
			req.UseReflection = false
		} else {
			req.UseReflection = true
		}
		return true
	case "grpc-plaintext":
		req := b.EnsureRequest()
		req.PlaintextSet = true
		if rest == "" {
			req.Plaintext = true
		} else if strings.EqualFold(rest, "false") || strings.EqualFold(rest, "0") {
			req.Plaintext = false
		} else if strings.EqualFold(rest, "true") || strings.EqualFold(rest, "1") {
			req.Plaintext = true
		} else {
			req.Plaintext = true
		}
		return true
	case "grpc-authority":
		b.EnsureRequest().Authority = rest
		return true
	case "grpc-metadata":
		req := b.EnsureRequest()
		if rest != "" {
			if idx := strings.Index(rest, ":"); idx >= 0 {
				key := strings.TrimSpace(rest[:idx])
				value := strings.TrimSpace(rest[idx+1:])
				if key != "" {
					req.Metadata[key] = value
				}
			}
		}
		return true
	}
	return false
}

func (b *Builder) HandleBodyLine(line string) bool {
	if b.request == nil {
		return false
	}

	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	if strings.HasPrefix(trimmed, "<") {
		b.messageFromFile = strings.TrimSpace(strings.TrimPrefix(trimmed, "<"))
		b.messageLines = nil
		return true
	}

	if strings.HasPrefix(trimmed, "@") && strings.Contains(trimmed, "<") {
		parts := strings.SplitN(trimmed, "<", 2)
		if len(parts) == 2 {
			b.messageFromFile = strings.TrimSpace(parts[1])
			b.messageLines = nil
			return true
		}
	}
	b.messageLines = append(b.messageLines, line)
	return true
}

func (b *Builder) Finalize(
	existingMime string,
) (*restfile.GRPCRequest, restfile.BodySource, string, bool) {
	if b.request == nil {
		return nil, restfile.BodySource{}, existingMime, false
	}

	grpcCopy := *b.request
	if len(grpcCopy.Metadata) > 0 {
		metadataCopy := make(map[string]string, len(grpcCopy.Metadata))
		for k, v := range grpcCopy.Metadata {
			metadataCopy[k] = v
		}
		grpcCopy.Metadata = metadataCopy
	}
	if b.messageFromFile != "" {
		grpcCopy.MessageFile = b.messageFromFile
		grpcCopy.Message = ""
	} else if len(b.messageLines) > 0 {
		grpcCopy.Message = strings.Join(b.messageLines, "\n")
	}

	body := restfile.BodySource{}
	if grpcCopy.MessageFile != "" {
		body.FilePath = grpcCopy.MessageFile
	} else if strings.TrimSpace(grpcCopy.Message) != "" {
		body.Text = grpcCopy.Message
	}
	return &grpcCopy, body, existingMime, true
}

func parseMethod(spec string) (pkg string, service string, method string) {
	working := strings.TrimSpace(spec)
	if working == "" {
		return "", "", ""
	}
	working = strings.TrimPrefix(working, "/")

	parts := strings.Split(working, "/")
	if len(parts) < 2 {
		return "", "", ""
	}

	serviceFQN := strings.TrimSpace(parts[0])
	method = strings.TrimSpace(parts[1])
	if serviceFQN == "" || method == "" {
		return "", "", ""
	}

	lastDot := strings.LastIndex(serviceFQN, ".")
	if lastDot >= 0 {
		pkg = serviceFQN[:lastDot]
		service = serviceFQN[lastDot+1:]
	} else {
		service = serviceFQN
	}
	return pkg, service, method
}
