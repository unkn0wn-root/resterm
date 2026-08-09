package grpc

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
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
		b.request = &restfile.GRPCRequest{
			Metadata:      []restfile.MetadataPair{},
			UseReflection: true,
		}
	} else if b.request.Metadata == nil {
		b.request.Metadata = []restfile.MetadataPair{}
	}
	return b.request
}

func (b *Builder) SetTarget(target string) {
	req := b.EnsureRequest()
	req.Target = str.Trim(target)
}

// HandleDirective applies a gRPC directive. A directive with a missing value is
// left unclaimed because claiming it could make gRPC hide a body intended for
// another protocol. gRPC has no reset directive, so reset is always false.
func (b *Builder) HandleDirective(name directive.Name, rest string) (handled, reset bool, err error) {
	switch name {
	case directive.GRPC:
		if rest == "" {
			b.EnsureRequest()
			return true, false, nil
		}
		pkg, service, method := parseMethod(rest)
		if service == "" || method == "" {
			return true, false, fmt.Errorf("invalid @grpc method %q, use package.Service/Method", rest)
		}
		req := b.EnsureRequest()
		req.Package, req.Service, req.Method = pkg, service, method
		req.FullMethod = fullMethod(pkg, service, method)
		return true, false, nil
	case directive.GRPCDescriptor:
		if rest == "" {
			return true, false, nil
		}
		b.EnsureRequest().DescriptorSet = rest
		return true, false, nil
	case directive.GRPCReflection:
		on, err := parseSwitch(name, rest)
		if err != nil {
			return true, false, err
		}
		b.EnsureRequest().UseReflection = on
		return true, false, nil
	case directive.GRPCPlaintext:
		on, err := parseSwitch(name, rest)
		if err != nil {
			return true, false, err
		}
		b.EnsureRequest().Plaintext = restfile.OptOf(on)
		return true, false, nil
	case directive.GRPCAuthority:
		if rest == "" {
			return true, false, nil
		}
		b.EnsureRequest().Authority = rest
		return true, false, nil
	case directive.GRPCMetadata:
		if rest == "" {
			return true, false, nil
		}
		pair, err := parseMetadata(name, rest)
		if err != nil {
			return true, false, err
		}
		req := b.EnsureRequest()
		req.Metadata = append(req.Metadata, pair)
		return true, false, nil
	}
	return false, false, nil
}

func parseSwitch(name directive.Name, rest string) (bool, error) {
	on, ok := directive.ParseSwitch(rest)
	if !ok {
		return false, fmt.Errorf("invalid %s %q: expected true or false", name.Tag(), rest)
	}
	return on, nil
}

func parseMetadata(name directive.Name, rest string) (restfile.MetadataPair, error) {
	before, after, ok := strings.Cut(rest, ":")
	key := str.Trim(before)
	if !ok || key == "" {
		return restfile.MetadataPair{}, fmt.Errorf("invalid %s %q, use key: value", name.Tag(), rest)
	}
	return restfile.MetadataPair{Key: key, Value: str.Trim(after)}, nil
}

func fullMethod(pkg, service, method string) string {
	if pkg != "" {
		service = pkg + "." + service
	}
	return "/" + service + "/" + method
}

func (b *Builder) HandleBodyLine(line string, forceInline bool) bool {
	if b.request == nil {
		return false
	}

	if str.Trim(line) == "" {
		return false
	}

	if file, ok := bodyref.ParseBodyFile(line, forceInline); ok {
		b.messageFromFile = file
		b.messageLines = nil
		return true
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
		meta := make([]restfile.MetadataPair, len(grpcCopy.Metadata))
		copy(meta, grpcCopy.Metadata)
		grpcCopy.Metadata = meta
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
	} else if str.Trim(grpcCopy.Message) != "" {
		body.Text = grpcCopy.Message
	}
	return &grpcCopy, body, existingMime, true
}

func parseMethod(spec string) (pkg string, service string, method string) {
	working := str.Trim(spec)
	if working == "" {
		return "", "", ""
	}
	working = strings.TrimPrefix(working, "/")

	parts := strings.Split(working, "/")
	if len(parts) < 2 {
		return "", "", ""
	}

	serviceFQN := str.Trim(parts[0])
	method = str.Trim(parts[1])
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
