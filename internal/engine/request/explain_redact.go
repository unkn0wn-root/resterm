package request

import (
	"net/textproto"
	"sort"
	"strings"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (e *Engine) redactExplainReport(
	rep *xplain.Report,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	globs vars.Globals,
	extra ...string,
) *xplain.Report {
	if rep == nil {
		return nil
	}
	secs := e.explainSecrets(doc, req, env, globs, extra)

	rep.Name = RedactSecrets(rep.Name, secs)
	rep.Method = RedactSecrets(rep.Method, secs)
	rep.URL = RedactSecrets(rep.URL, secs)
	rep.Decision = RedactSecrets(rep.Decision, secs)
	rep.Failure = RedactSecrets(rep.Failure, secs)

	for i := range rep.Vars {
		rep.Vars[i].Value = RedactSecrets(rep.Vars[i].Value, secs)
	}
	for i := range rep.Stages {
		rep.Stages[i].Summary = RedactSecrets(rep.Stages[i].Summary, secs)
		for j := range rep.Stages[i].Changes {
			ch := &rep.Stages[i].Changes[j]
			ch.Before = redactExplainChangeValue(ch.Field, ch.Before, secs)
			ch.After = redactExplainChangeValue(ch.Field, ch.After, secs)
		}
		for j := range rep.Stages[i].Notes {
			rep.Stages[i].Notes[j] = RedactSecrets(rep.Stages[i].Notes[j], secs)
		}
	}
	if rep.Final != nil {
		rep.Final.Method = RedactSecrets(rep.Final.Method, secs)
		rep.Final.URL = RedactSecrets(rep.Final.URL, secs)
		rep.Final.Body = RedactSecrets(rep.Final.Body, secs)
		rep.Final.BodyNote = RedactSecrets(rep.Final.BodyNote, secs)
		for i := range rep.Final.Headers {
			h := &rep.Final.Headers[i]
			if header.Sensitive(h.Name) {
				if strings.TrimSpace(h.Value) != "" {
					h.Value = secretMask
				}
				continue
			}
			h.Value = RedactSecrets(h.Value, secs)
		}
		for i := range rep.Final.Settings {
			rep.Final.Settings[i].Value = RedactSecrets(rep.Final.Settings[i].Value, secs)
		}
		for i := range rep.Final.Details {
			d := &rep.Final.Details[i]
			key := d.Key
			val := d.Value
			d.Key = RedactSecrets(key, secs)
			if shouldMaskExplainPair(key, val) {
				if strings.TrimSpace(val) != "" {
					d.Value = secretMask
				}
				continue
			}
			d.Value = RedactSecrets(val, secs)
		}
		for i := range rep.Final.Steps {
			rep.Final.Steps[i] = RedactSecrets(rep.Final.Steps[i], secs)
		}
		if rep.Final.Route != nil {
			rep.Final.Route.Summary = RedactSecrets(rep.Final.Route.Summary, secs)
			for i := range rep.Final.Route.Notes {
				rep.Final.Route.Notes[i] = RedactSecrets(rep.Final.Route.Notes[i], secs)
			}
		}
	}
	for i := range rep.Warnings {
		rep.Warnings[i] = RedactSecrets(rep.Warnings[i], secs)
	}
	return rep
}

func (e *Engine) explainSecrets(
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	globs vars.Globals,
	extra []string,
) []string {
	vals := make(map[string]struct{})
	add := func(v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		vals[v] = struct{}{}
	}

	for _, v := range e.secretValues(doc, req, env, extra...) {
		add(v)
	}
	for _, gv := range globs.All() {
		if gv.Secret && !gv.Delete {
			add(gv.Value)
		}
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

func redactExplainChangeValue(field, val string, secs []string) string {
	if hdr, ok := explainHeaderField(field); ok && header.Sensitive(hdr) {
		if strings.TrimSpace(val) == "" {
			return val
		}
		return secretMask
	}
	return RedactSecrets(val, secs)
}

func explainHeaderField(field string) (string, bool) {
	field = strings.TrimSpace(field)
	if !strings.HasPrefix(strings.ToLower(field), "header.") {
		return "", false
	}
	name := strings.TrimSpace(field[len("header."):])
	if name == "" {
		return "", false
	}
	return textproto.CanonicalMIMEHeaderKey(name), true
}

func shouldMaskExplainPair(key, val string) bool {
	if !strings.EqualFold(strings.TrimSpace(key), "Metadata") {
		return false
	}
	name, _, ok := strings.Cut(val, ":")
	if !ok {
		return false
	}
	return header.Sensitive(name)
}
