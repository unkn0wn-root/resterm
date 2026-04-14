package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	streamHeaderType    = "X-Resterm-Stream-Type"
	streamHeaderSummary = "X-Resterm-Stream-Summary"
)

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

func (m *Model) prepareGRPCRequest(
	req *restfile.Request,
	resolver *vars.Resolver,
	baseDir string,
) error {
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
	grpcReq.MessageExpanded = ""
	grpcReq.MessageExpandedSet = false

	if err := grpcclient.ValidateMetaPairs(grpcReq.Metadata); err != nil {
		return err
	}
	if err := grpcclient.ValidateHeaderPairs(req.Headers); err != nil {
		return err
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
		if req.Body.Options.ExpandTemplates && strings.TrimSpace(grpcReq.MessageFile) != "" {
			expanded, err := expandGRPCMessageFile(grpcReq.MessageFile, baseDir, resolver)
			if err != nil {
				return err
			}
			grpcReq.MessageExpanded = expanded
			grpcReq.MessageExpandedSet = true
		}
		if len(grpcReq.Metadata) > 0 {
			for i := range grpcReq.Metadata {
				value := grpcReq.Metadata[i].Value
				expanded, err := resolver.ExpandTemplates(value)
				if err != nil {
					return errdef.Wrap(
						errdef.CodeHTTP,
						err,
						"expand grpc metadata %s",
						grpcReq.Metadata[i].Key,
					)
				}
				grpcReq.Metadata[i].Value = expanded
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

func expandGRPCMessageFile(
	path string,
	baseDir string,
	resolver *vars.Resolver,
) (string, error) {
	if resolver == nil {
		return "", nil
	}
	full := path
	if !filepath.IsAbs(full) && baseDir != "" {
		full = filepath.Join(baseDir, full)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "read grpc message file %s", path)
	}
	expanded, err := resolver.ExpandTemplates(string(data))
	if err != nil {
		return "", errdef.Wrap(errdef.CodeHTTP, err, "expand grpc message file")
	}
	return expanded, nil
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
