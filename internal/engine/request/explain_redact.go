package request

import (
	"net/textproto"
	"strings"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/redact"
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
	sec := redact.New(e.explainSecrets(doc, req, env, globs, extra)...)

	rep.Name = sec.Text(rep.Name)
	rep.Method = sec.Text(rep.Method)
	rep.URL = sec.Text(rep.URL)
	rep.Decision = sec.Text(rep.Decision)
	rep.Failure = sec.Text(rep.Failure)

	for i := range rep.Vars {
		rep.Vars[i].Value = sec.Text(rep.Vars[i].Value)
	}
	for i := range rep.Stages {
		rep.Stages[i].Summary = sec.Text(rep.Stages[i].Summary)
		for j := range rep.Stages[i].Changes {
			ch := &rep.Stages[i].Changes[j]
			ch.Before = redactExplainChangeValue(ch.Field, ch.Before, sec)
			ch.After = redactExplainChangeValue(ch.Field, ch.After, sec)
		}
		for j := range rep.Stages[i].Notes {
			rep.Stages[i].Notes[j] = sec.Text(rep.Stages[i].Notes[j])
		}
	}
	if rep.Final != nil {
		rep.Final.Method = sec.Text(rep.Final.Method)
		rep.Final.URL = sec.Text(rep.Final.URL)
		rep.Final.Body = string(sec.JSON([]byte(rep.Final.Body)))
		rep.Final.BodyNote = sec.Text(rep.Final.BodyNote)
		for i := range rep.Final.Headers {
			h := &rep.Final.Headers[i]
			if header.Sensitive(h.Name) {
				if strings.TrimSpace(h.Value) != "" {
					h.Value = redact.Mask
				}
				continue
			}
			h.Value = sec.Text(h.Value)
		}
		for i := range rep.Final.Settings {
			rep.Final.Settings[i].Value = sec.Text(rep.Final.Settings[i].Value)
		}
		for i := range rep.Final.Details {
			d := &rep.Final.Details[i]
			key := d.Key
			val := d.Value
			d.Key = sec.Text(key)
			if shouldMaskExplainPair(key, val) {
				if strings.TrimSpace(val) != "" {
					d.Value = redact.Mask
				}
				continue
			}
			d.Value = sec.Text(val)
		}
		for i := range rep.Final.Steps {
			rep.Final.Steps[i] = sec.Text(rep.Final.Steps[i])
		}
		if rep.Final.Route != nil {
			rep.Final.Route.Summary = sec.Text(rep.Final.Route.Summary)
			for i := range rep.Final.Route.Notes {
				rep.Final.Route.Notes[i] = sec.Text(rep.Final.Route.Notes[i])
			}
		}
	}
	for i := range rep.Warnings {
		rep.Warnings[i] = sec.Text(rep.Warnings[i])
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
	return out
}

func redactExplainChangeValue(field, val string, sec redact.Set) string {
	if hdr, ok := explainHeaderField(field); ok && header.Sensitive(hdr) {
		if strings.TrimSpace(val) == "" {
			return val
		}
		return redact.Mask
	}
	return sec.Text(val)
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
