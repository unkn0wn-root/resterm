package request

import (
	"net/textproto"
	"sort"
	"strings"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

const explainMask = "•••"

func (e *Engine) redactExplainReport(
	rep *xplain.Report,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	globs map[string]scripts.GlobalValue,
	extra ...string,
) *xplain.Report {
	if rep == nil {
		return nil
	}
	secs := e.explainSecrets(doc, req, env, globs, extra)

	rep.Name = redactExplainText(rep.Name, secs)
	rep.Method = redactExplainText(rep.Method, secs)
	rep.URL = redactExplainText(rep.URL, secs)
	rep.Decision = redactExplainText(rep.Decision, secs)
	rep.Failure = redactExplainText(rep.Failure, secs)

	for i := range rep.Vars {
		rep.Vars[i].Value = redactExplainText(rep.Vars[i].Value, secs)
	}
	for i := range rep.Stages {
		rep.Stages[i].Summary = redactExplainText(rep.Stages[i].Summary, secs)
		for j := range rep.Stages[i].Changes {
			ch := &rep.Stages[i].Changes[j]
			ch.Before = redactExplainChangeValue(ch.Field, ch.Before, secs)
			ch.After = redactExplainChangeValue(ch.Field, ch.After, secs)
		}
		for j := range rep.Stages[i].Notes {
			rep.Stages[i].Notes[j] = redactExplainText(rep.Stages[i].Notes[j], secs)
		}
	}
	if rep.Final != nil {
		rep.Final.Method = redactExplainText(rep.Final.Method, secs)
		rep.Final.URL = redactExplainText(rep.Final.URL, secs)
		rep.Final.Body = redactExplainText(rep.Final.Body, secs)
		rep.Final.BodyNote = redactExplainText(rep.Final.BodyNote, secs)
		for i := range rep.Final.Headers {
			h := &rep.Final.Headers[i]
			if shouldMaskExplainHeader(h.Name) {
				if strings.TrimSpace(h.Value) != "" {
					h.Value = explainMask
				}
				continue
			}
			h.Value = redactExplainText(h.Value, secs)
		}
		for i := range rep.Final.Settings {
			rep.Final.Settings[i].Value = redactExplainText(rep.Final.Settings[i].Value, secs)
		}
		for i := range rep.Final.Details {
			d := &rep.Final.Details[i]
			key := d.Key
			val := d.Value
			d.Key = redactExplainText(key, secs)
			if shouldMaskExplainPair(key, val) {
				if strings.TrimSpace(val) != "" {
					d.Value = explainMask
				}
				continue
			}
			d.Value = redactExplainText(val, secs)
		}
		for i := range rep.Final.Steps {
			rep.Final.Steps[i] = redactExplainText(rep.Final.Steps[i], secs)
		}
		if rep.Final.Route != nil {
			rep.Final.Route.Summary = redactExplainText(rep.Final.Route.Summary, secs)
			for i := range rep.Final.Route.Notes {
				rep.Final.Route.Notes[i] = redactExplainText(rep.Final.Route.Notes[i], secs)
			}
		}
	}
	for i := range rep.Warnings {
		rep.Warnings[i] = redactExplainText(rep.Warnings[i], secs)
	}
	return rep
}

func (e *Engine) explainSecrets(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	globs map[string]scripts.GlobalValue,
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
	for _, gv := range globs {
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
	if hdr, ok := explainHeaderField(field); ok && shouldMaskExplainHeader(hdr) {
		if strings.TrimSpace(val) == "" {
			return val
		}
		return explainMask
	}
	return redactExplainText(val, secs)
}

func redactExplainText(s string, secs []string) string {
	if strings.TrimSpace(s) == "" || len(secs) == 0 {
		return s
	}
	out := s
	for _, sec := range secs {
		if sec == "" || !strings.Contains(out, sec) {
			continue
		}
		out = strings.ReplaceAll(out, sec, explainMask)
	}
	return out
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

func shouldMaskExplainHeader(name string) bool {
	if name == "" {
		return false
	}
	_, ok := sensHdr[strings.ToLower(name)]
	return ok
}

func shouldMaskExplainPair(key, val string) bool {
	if !strings.EqualFold(strings.TrimSpace(key), "Metadata") {
		return false
	}
	name, _, ok := strings.Cut(val, ":")
	if !ok {
		return false
	}
	return shouldMaskExplainHeader(strings.TrimSpace(name))
}
