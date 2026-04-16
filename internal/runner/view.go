package runner

import (
	"bytes"
	"sort"
	"strings"
)

func (r Result) Transcript() []byte {
	return bytes.Clone(r.transcript)
}

func (r Result) RequestText() string {
	return r.requestText
}

func (r *Result) SetRequestText(text string) {
	if r == nil {
		return
	}
	r.requestText = strings.TrimSpace(text)
}

func (r Result) UnresolvedTemplateVars() ([]string, bool) {
	if !r.unresolvedTemplateVarsSet {
		return nil, false
	}
	return append([]string(nil), r.unresolvedTemplateVars...), true
}

func (r *Result) SetUnresolvedTemplateVars(items []string) {
	if r == nil {
		return
	}
	r.unresolvedTemplateVarsSet = true
	if len(items) == 0 {
		r.unresolvedTemplateVars = nil
		return
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	r.unresolvedTemplateVars = out
}

func (s StepResult) Transcript() []byte {
	return bytes.Clone(s.transcript)
}

func (s StepResult) RequestText() string {
	return s.requestText
}

func (s *StepResult) SetRequestText(text string) {
	if s == nil {
		return
	}
	s.requestText = strings.TrimSpace(text)
}
