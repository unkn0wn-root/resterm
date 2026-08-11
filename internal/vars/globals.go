package vars

import (
	"slices"
	"strings"
)

// GlobalMutation describes a script/runtime global variable set or delete.
type GlobalMutation struct {
	Name   string
	Value  string
	Secret bool
	Delete bool
}

type Globals = NameMap[GlobalMutation]

// Secrets records values that must remain masked for the lifetime of a run.
// Values are added when exposed rather than when a change commits because
// failed, deleted, and overwritten mutations may still appear in output.
type Secrets struct {
	vals []string
}

func (s *Secrets) Add(vals ...string) {
	if s == nil {
		return
	}
	for _, v := range vals {
		if strings.TrimSpace(v) == "" || slices.Contains(s.vals, v) {
			continue
		}
		s.vals = append(s.vals, v)
	}
}

func (s *Secrets) Values() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.vals...)
}
