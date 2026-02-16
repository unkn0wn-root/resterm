package collection

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type ref struct {
	Path string
	Role FileRole
}

func collectRefs(doc *restfile.Document) []ref {
	if doc == nil {
		return nil
	}
	rs := make([]ref, 0, 16)
	for _, u := range doc.Uses {
		rs = addRef(rs, u.Path, RoleScript)
	}
	for _, req := range doc.Requests {
		if req == nil {
			continue
		}
		for _, u := range req.Metadata.Uses {
			rs = addRef(rs, u.Path, RoleScript)
		}
		for _, s := range req.Metadata.Scripts {
			rs = addRef(rs, s.FilePath, RoleScript)
		}
		rs = addRef(rs, req.Body.FilePath, RoleAsset)
		for _, p := range bodyIncludes(req.Body.Text) {
			rs = addRef(rs, p, RoleAsset)
		}
		if gql := req.Body.GraphQL; gql != nil {
			rs = addRef(rs, gql.QueryFile, RoleAsset)
			rs = addRef(rs, gql.VariablesFile, RoleAsset)
		}
		if grpc := req.GRPC; grpc != nil {
			rs = addRef(rs, grpc.DescriptorSet, RoleAsset)
			rs = addRef(rs, grpc.MessageFile, RoleAsset)
		}
		if ws := req.WebSocket; ws != nil {
			for _, step := range ws.Steps {
				if step.Type != restfile.WebSocketStepSendFile {
					continue
				}
				rs = addRef(rs, step.File, RoleAsset)
			}
		}
	}
	return rs
}

func bodyIncludes(body string) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	parts := strings.Split(body, "\n")
	out := make([]string, 0, len(parts))
	for _, ln := range parts {
		// TrimSpace keeps CRLF and LF source files equivalent for include parsing.
		t := strings.TrimSpace(ln)
		if len(t) <= 1 {
			continue
		}
		if !strings.HasPrefix(t, "@") || strings.HasPrefix(t, "@{") {
			continue
		}
		p := strings.TrimSpace(t[1:])
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func addRef(rs []ref, p string, role FileRole) []ref {
	p = strings.TrimSpace(p)
	if p == "" {
		return rs
	}
	return append(rs, ref{Path: p, Role: role})
}
