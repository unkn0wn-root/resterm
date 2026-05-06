package diag

import (
	"slices"
)

type chainEntryKey struct {
	Kind    ChainKind
	Class   Class
	Message string
}

func chainKey(entry ChainEntry) chainEntryKey {
	return chainEntryKey{
		Kind:    entry.Kind,
		Class:   entry.Class,
		Message: entry.Message,
	}
}

func severityOrError(sev Severity) Severity {
	return knownOr(sev, SeverityError, SeverityWarning, SeverityNote)
}

func noteKindOrInfo(kind NoteKind) NoteKind {
	return knownOr(kind, NoteInfo, NoteHelp, NoteWarning, NoteSuggestion)
}

func chainKindOrCause(kind ChainKind) ChainKind {
	return knownOr(kind, ChainCause, ChainOperation)
}

func classOrUnknown(class Class) Class {
	if class == "" {
		return ClassUnknown
	}
	return class
}

func knownOr[T comparable](value, fallback T, allowed ...T) T {
	if value == fallback || slices.Contains(allowed, value) {
		return value
	}
	return fallback
}

func prepareChainEntry(entry ChainEntry) ChainEntry {
	entry.Class = classOrUnknown(entry.Class)
	entry.Kind = chainKindOrCause(entry.Kind)
	entry.Children = prepareChain(entry.Children)
	return entry
}

func prepareChain(src []ChainEntry) []ChainEntry {
	chain := make([]ChainEntry, 0, len(src))
	for _, entry := range src {
		entry = prepareChainEntry(entry)
		if entry.Message == "" {
			chain = appendChainEntries(chain, entry.Children)
			continue
		}
		chain = appendChain(chain, entry)
	}
	return chain
}

func appendChainEntries(chain []ChainEntry, entries []ChainEntry) []ChainEntry {
	for _, entry := range entries {
		chain = appendChain(chain, entry)
	}
	return chain
}

func appendChain(chain []ChainEntry, entry ChainEntry) []ChainEntry {
	key := chainKey(entry)
	if slices.ContainsFunc(chain, func(got ChainEntry) bool {
		return chainKey(got) == key
	}) {
		return chain
	}
	return append(chain, entry)
}

func prepareDiagnostic(d Diagnostic, path string) Diagnostic {
	d.Class = classOrUnknown(d.Class)
	d.Severity = severityOrError(d.Severity)
	d.Span = prepareSpan(d.Span, path)
	d.Source = append([]byte(nil), d.Source...)
	d.Labels = prepareLabels(d.Labels, path)
	d.Notes = prepareNotes(d.Notes)
	d.Chain = prepareChain(d.Chain)
	d.Frames = slices.Clone(d.Frames)
	return d
}

func prepareReport(r Report) Report {
	if len(r.Items) == 0 {
		return Report{}
	}
	out := Report{
		Path:   r.Path,
		Source: append([]byte(nil), r.Source...),
		Items:  make([]Diagnostic, len(r.Items)),
	}
	for i, it := range r.Items {
		out.Items[i] = prepareDiagnostic(it, out.Path)
	}
	return out
}

func prepareSpan(span Span, path string) Span {
	if span.Start.Path == "" {
		span.Start.Path = path
	}
	if span.Start.Col <= 0 && span.Start.Line > 0 {
		span.Start.Col = 1
	}
	if span.End.Path == "" {
		span.End.Path = span.Start.Path
	}
	return span
}

func prepareLabels(src []Label, path string) []Label {
	out := make([]Label, 0, len(src))
	for _, label := range src {
		if label.Message == "" && label.Span.Start == (Pos{}) && label.Span.End == (Pos{}) {
			continue
		}
		if label.Span.Start.Path == "" {
			label.Span.Start.Path = path
		}
		if label.Span.End.Path == "" {
			label.Span.End.Path = label.Span.Start.Path
		}
		out = append(out, label)
	}
	return out
}

func prepareNotes(src []Note) []Note {
	out := make([]Note, 0, len(src))
	for _, note := range src {
		note.Kind = noteKindOrInfo(note.Kind)
		if note.Message == "" {
			continue
		}
		dup := slices.ContainsFunc(out, func(got Note) bool {
			return got.Kind == note.Kind &&
				got.Message == note.Message &&
				got.Span.Start == note.Span.Start
		})
		if !dup {
			out = append(out, note)
		}
	}
	return out
}
