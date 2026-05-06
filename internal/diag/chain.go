package diag

import (
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"reflect"
	"strings"
)

const maxChainDepth = 32

type chainState struct {
	seen map[error]struct{}
	skip map[string]struct{}
}

func (s *chainState) entries(err error, depth int) []ChainEntry {
	if err == nil {
		return nil
	}
	if depth >= maxChainDepth {
		return []ChainEntry{{Kind: ChainCause, Message: "error chain truncated"}}
	}
	if s.markSeen(err) {
		return nil
	}

	if e, ok := err.(*diagnosticError); ok {
		children := s.entries(e.err, depth+1)
		msg := e.message
		if msg == "" || s.shouldSkip(msg) {
			return children
		}

		entry := operationEntry(e)
		entry.Message = msg
		if e.err == nil {
			entry.Kind = ChainCause
			return []ChainEntry{entry}
		}
		entry.Children = children
		return []ChainEntry{entry}
	}

	if urlErr, ok := err.(*url.Error); ok {
		return s.typedWrapper(
			ChainOperation,
			classify(urlErr),
			typedURLMessage(urlErr),
			urlErr.Err,
			depth,
		)
	}
	if opErr, ok := err.(*net.OpError); ok {
		return s.typedWrapper(
			ChainOperation,
			classify(opErr),
			typedNetOpMessage(opErr),
			opErr.Err,
			depth,
		)
	}
	if pathErr, ok := err.(*fs.PathError); ok {
		return s.typedWrapper(
			ChainOperation,
			classify(pathErr),
			typedPathMessage(pathErr),
			pathErr.Err,
			depth,
		)
	}

	if wrapped, ok := err.(errsUnwrapper); ok {
		var out []ChainEntry
		for _, child := range wrapped.Unwrap() {
			childChain := s.entries(child, depth+1)
			if len(childChain) == 0 {
				childChain = leafChain(child, s)
			}
			out = append(out, childChain...)
		}
		return out
	}

	if rep, ok := err.(reporter); ok {
		msg := reportMessage(rep.Diagnostic())
		if msg != "" && !s.shouldSkip(msg) {
			rep := rep.Diagnostic()
			return []ChainEntry{{
				Class:     rep.Class(),
				Component: firstComponent(rep),
				Kind:      ChainCause,
				Message:   msg,
			}}
		}
	}

	if wrapped, ok := err.(errUnwrapper); ok {
		if child := wrapped.Unwrap(); child != nil {
			return s.entries(child, depth+1)
		}
	}
	return leafChain(err, s)
}

func (s *chainState) typedWrapper(
	kind ChainKind,
	class Class,
	msg string,
	child error,
	depth int,
) []ChainEntry {
	children := s.entries(child, depth+1)
	if msg == "" || s.shouldSkip(msg) {
		return children
	}
	return []ChainEntry{{
		Class:    class,
		Kind:     kind,
		Message:  msg,
		Children: children,
	}}
}

func (s *chainState) markSeen(err error) bool {
	if err == nil {
		return true
	}
	t := reflect.TypeOf(err)
	if t == nil || !t.Comparable() {
		return false
	}
	if _, ok := s.seen[err]; ok {
		return true
	}
	s.seen[err] = struct{}{}
	return false
}

func (s *chainState) shouldSkip(msg string) bool {
	if msg == "" {
		return true
	}
	_, ok := s.skip[msg]
	return ok
}

func operationEntry(e *diagnosticError) ChainEntry {
	if e == nil || e.message == "" {
		return ChainEntry{}
	}
	class := classFromError(e)
	component := e.component
	if component == "" {
		component = e.meta.component
	}
	return ChainEntry{
		Class:     class,
		Component: component,
		Kind:      ChainOperation,
		Message:   e.message,
	}
}

func chainWithOp(op ChainEntry, base, existing []ChainEntry) []ChainEntry {
	if len(base) == 0 {
		base = existing
	}
	if op.Message == "" {
		return prepareChain(base)
	}
	op.Children = prepareChain(base)
	return prepareChain([]ChainEntry{op})
}

func chainOfError(err error, skip ...string) []ChainEntry {
	if err == nil {
		return nil
	}
	st := chainState{
		seen: make(map[error]struct{}),
		skip: make(map[string]struct{}, len(skip)),
	}
	for _, msg := range skip {
		if msg != "" {
			st.skip[msg] = struct{}{}
		}
	}
	return prepareChain(st.entries(err, 0))
}

func leafChain(err error, st *chainState) []ChainEntry {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if msg == "" || st.shouldSkip(msg) {
		return nil
	}
	return []ChainEntry{{
		Class:   classify(err),
		Kind:    ChainCause,
		Message: msg,
	}}
}

func typedURLMessage(err *url.Error) string {
	if err == nil {
		return ""
	}
	switch {
	case err.Op != "" && err.URL != "":
		return fmt.Sprintf("%s %q", err.Op, err.URL)
	case err.Op != "":
		return err.Op
	default:
		return err.URL
	}
}

func typedNetOpMessage(err *net.OpError) string {
	if err == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if err.Op != "" {
		parts = append(parts, err.Op)
	}
	if err.Net != "" {
		parts = append(parts, err.Net)
	}
	if err.Addr != nil {
		parts = append(parts, err.Addr.String())
	}
	return strings.Join(parts, " ")
}

func typedPathMessage(err *fs.PathError) string {
	if err == nil {
		return ""
	}
	switch {
	case err.Op != "" && err.Path != "":
		return err.Op + " " + err.Path
	case err.Op != "":
		return err.Op
	default:
		return err.Path
	}
}

func reportMessage(rep Report) string {
	rep = prepareReport(rep)
	if len(rep.Items) == 0 {
		return ""
	}
	return rep.Summary()
}

func firstComponent(rep Report) Component {
	rep = prepareReport(rep)
	if len(rep.Items) == 0 {
		return ""
	}
	return rep.Items[0].Component
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
