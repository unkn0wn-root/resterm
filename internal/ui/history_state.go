package ui

import (
	"sort"
	"strconv"

	"github.com/unkn0wn-root/resterm/internal/history"
)

type historyScope int

const (
	historyScopeGlobal historyScope = iota
	historyScopeFile
	historyScopeRequest
	historyScopeWorkflow
)

func (s historyScope) next() historyScope {
	switch s {
	case historyScopeGlobal:
		return historyScopeFile
	case historyScopeFile:
		return historyScopeRequest
	case historyScopeRequest:
		return historyScopeWorkflow
	default:
		return historyScopeGlobal
	}
}

func historyScopeLabel(scope historyScope) string {
	switch scope {
	case historyScopeGlobal:
		return "Global"
	case historyScopeFile:
		return "File"
	case historyScopeRequest:
		return "Request"
	case historyScopeWorkflow:
		return "Workflow"
	default:
		return "Global"
	}
}

type historySort int

const (
	historySortNewest historySort = iota
	historySortOldest
)

func (s historySort) toggle() historySort {
	if s == historySortOldest {
		return historySortNewest
	}
	return historySortOldest
}

func historySortLabel(sort historySort) string {
	switch sort {
	case historySortOldest:
		return "Oldest"
	default:
		return "Newest"
	}
}

func sortHistoryEntries(entries []history.Entry, order historySort) []history.Entry {
	if len(entries) < 2 {
		return entries
	}
	out := append([]history.Entry(nil), entries...)
	if order == historySortOldest {
		sort.SliceStable(out, func(i, j int) bool {
			return historyEntryOlderFirst(out[i], out[j])
		})
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		return historyEntryNewerFirst(out[i], out[j])
	})
	return out
}

func historyEntryNewerFirst(a, b history.Entry) bool {
	ai := a.ExecutedAt
	bi := b.ExecutedAt
	switch {
	case ai.IsZero() && bi.IsZero():
		return compareHistoryIDsDesc(a.ID, b.ID)
	case ai.IsZero():
		return false
	case bi.IsZero():
		return true
	case ai.Equal(bi):
		return compareHistoryIDsDesc(a.ID, b.ID)
	default:
		return ai.After(bi)
	}
}

func historyEntryOlderFirst(a, b history.Entry) bool {
	ai := a.ExecutedAt
	bi := b.ExecutedAt
	switch {
	case ai.IsZero() && bi.IsZero():
		return compareHistoryIDsAsc(a.ID, b.ID)
	case ai.IsZero():
		return true
	case bi.IsZero():
		return false
	case ai.Equal(bi):
		return compareHistoryIDsAsc(a.ID, b.ID)
	default:
		return ai.Before(bi)
	}
}

func compareHistoryIDsDesc(a, b string) bool {
	ai, errA := strconv.ParseInt(a, 10, 64)
	bi, errB := strconv.ParseInt(b, 10, 64)
	if errA == nil && errB == nil {
		return ai > bi
	}
	return a > b
}

func compareHistoryIDsAsc(a, b string) bool {
	ai, errA := strconv.ParseInt(a, 10, 64)
	bi, errB := strconv.ParseInt(b, 10, 64)
	if errA == nil && errB == nil {
		return ai < bi
	}
	return a < b
}
