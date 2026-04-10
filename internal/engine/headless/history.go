package headless

import (
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

var sensHdr = map[string]struct{}{
	"api-key":                 {},
	"apikey":                  {},
	"authorization":           {},
	"proxy-authorization":     {},
	"x-access-token":          {},
	"x-amz-security-token":    {},
	"x-api-key":               {},
	"x-apikey":                {},
	"x-auth-email":            {},
	"x-auth-key":              {},
	"x-auth-token":            {},
	"x-aws-access-token":      {},
	"x-aws-secret-access-key": {},
	"x-client-secret":         {},
	"x-csrf-token":            {},
	"x-goog-api-key":          {},
	"x-refresh-token":         {},
	"x-secret-key":            {},
	"x-token":                 {},
	"x-xsrf-token":            {},
}

func (e *Engine) secretValues(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extra ...string,
) []string {
	vals := make(map[string]struct{})
	add := func(v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		vals[v] = struct{}{}
	}
	if req != nil {
		for _, v := range req.Variables {
			if v.Secret {
				add(v.Value)
			}
		}
	}
	if doc != nil {
		for _, v := range doc.Variables {
			if v.Secret {
				add(v.Value)
			}
		}
		for _, v := range doc.Globals {
			if v.Secret {
				add(v.Value)
			}
		}
	}
	if e != nil && e.rt != nil {
		if fs := e.rt.Files(); fs != nil {
			if snap := fs.Snapshot(e.env(env), e.filePath(doc)); len(snap) > 0 {
				for _, v := range snap {
					if v.Secret {
						add(v.Value)
					}
				}
			}
		}
		if gs := e.rt.Globals(); gs != nil {
			if snap := gs.Snapshot(e.env(env)); len(snap) > 0 {
				for _, v := range snap {
					if v.Secret {
						add(v.Value)
					}
				}
			}
		}
	}
	for _, v := range extra {
		add(v)
	}
	if len(vals) == 0 {
		return nil
	}
	out := make([]string, 0, len(vals))
	for v := range vals {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func redactText(text string, secs []string, maskHdr bool) string {
	out := text
	for _, sec := range secs {
		if sec == "" || !strings.Contains(out, sec) {
			continue
		}
		out = strings.ReplaceAll(out, sec, "***")
	}
	if !maskHdr {
		return out
	}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(line[:colon]))
		if _, ok := sensHdr[name]; !ok {
			continue
		}
		rest := line[colon+1:]
		padN := len(rest) - len(strings.TrimLeft(rest, " \t"))
		pad := ""
		if padN > 0 {
			pad = rest[:padN]
		}
		lines[i] = line[:colon+1] + pad + "***"
	}
	return strings.Join(lines, "\n")
}

func snippetHTTP(resp *httpclient.Response, req *restfile.Request, secs []string) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	return redactText(string(resp.Body), secs, false)
}

func snippetGRPC(resp *grpcclient.Response, req *restfile.Request, secs []string) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	return strings.TrimSpace(redactText(resp.Message, secs, false))
}
