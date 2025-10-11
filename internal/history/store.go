package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

type Entry struct {
	ID             string          `json:"id"`
	ExecutedAt     time.Time       `json:"executedAt"`
	Environment    string          `json:"environment"`
	RequestName    string          `json:"requestName"`
	Method         string          `json:"method"`
	URL            string          `json:"url"`
	Status         string          `json:"status"`
	StatusCode     int             `json:"statusCode"`
	Duration       time.Duration   `json:"duration"`
	BodySnippet    string          `json:"bodySnippet"`
	RequestText    string          `json:"requestText"`
	Description    string          `json:"description,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	ProfileResults *ProfileResults `json:"profileResults,omitempty"`
}

type ProfileResults struct {
	TotalRuns      int                   `json:"totalRuns"`
	WarmupRuns     int                   `json:"warmupRuns"`
	SuccessfulRuns int                   `json:"successfulRuns"`
	FailedRuns     int                   `json:"failedRuns"`
	Latency        *ProfileLatency       `json:"latency,omitempty"`
	Percentiles    []ProfilePercentile   `json:"percentiles,omitempty"`
	Histogram      []ProfileHistogramBin `json:"histogram,omitempty"`
}

type ProfileLatency struct {
	Count  int           `json:"count"`
	Min    time.Duration `json:"min"`
	Max    time.Duration `json:"max"`
	Mean   time.Duration `json:"mean"`
	Median time.Duration `json:"median"`
	StdDev time.Duration `json:"stdDev"`
}

type ProfilePercentile struct {
	Percentile int           `json:"percentile"`
	Value      time.Duration `json:"value"`
}

type ProfileHistogramBin struct {
	From  time.Duration `json:"from"`
	To    time.Duration `json:"to"`
	Count int           `json:"count"`
}

type Store struct {
	path       string
	maxEntries int
	entries    []Entry
	mu         sync.RWMutex
	loaded     bool
}

func NewStore(path string, maxEntries int) *Store {
	if maxEntries <= 0 {
		maxEntries = 200
	}
	return &Store{path: path, maxEntries: maxEntries}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.entries = []Entry{}
			s.loaded = true
			return nil
		}
		return errdef.Wrap(errdef.CodeHistory, err, "read history")
	}

	if len(data) == 0 {
		s.entries = []Entry{}
		s.loaded = true
		return nil
	}

	if err := json.Unmarshal(data, &s.entries); err != nil {
		return errdef.Wrap(errdef.CodeHistory, err, "parse history")
	}

	s.sortEntriesLocked()
	s.loaded = true
	return nil
}

func (s *Store) Append(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.loaded {
		if err := s.Load(); err != nil {
			return err
		}
	}

	s.entries = append([]Entry{entry}, s.entries...)
	s.sortEntriesLocked()
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[:s.maxEntries]
	}

	if err := s.persist(); err != nil {
		return err
	}
	return nil
}

func (s *Store) Entries() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copies := make([]Entry, len(s.entries))
	copy(copies, s.entries)
	return copies
}

func (s *Store) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.loaded {
		if err := s.Load(); err != nil {
			return false, err
		}
	}

	idx := -1
	for i, entry := range s.entries {
		if entry.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false, nil
	}

	copy(s.entries[idx:], s.entries[idx+1:])
	s.entries = s.entries[:len(s.entries)-1]

	if err := s.persist(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ByRequest(identifier string) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if identifier == "" {
		return s.Entries()
	}

	var result []Entry
	for _, entry := range s.entries {
		if entry.RequestName == identifier || entry.URL == identifier {
			result = append(result, entry)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return newerFirst(result[i], result[j])
	})
	return result
}

func (s *Store) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return errdef.Wrap(errdef.CodeFilesystem, err, "create history dir")
	}

	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return errdef.Wrap(errdef.CodeHistory, err, "encode history")
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return errdef.Wrap(errdef.CodeFilesystem, err, "write history tmp")
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return errdef.Wrap(errdef.CodeFilesystem, err, "replace history file")
	}

	return nil
}

func (s *Store) sortEntriesLocked() {
	if len(s.entries) < 2 {
		return
	}

	sort.SliceStable(s.entries, func(i, j int) bool {
		return newerFirst(s.entries[i], s.entries[j])
	})
}

func newerFirst(a, b Entry) bool {
	ai := a.ExecutedAt
	bi := b.ExecutedAt
	switch {
	case ai.IsZero() && bi.IsZero():
		return compareIDsDesc(a.ID, b.ID)
	case ai.IsZero():
		return false
	case bi.IsZero():
		return true
	case ai.Equal(bi):
		return compareIDsDesc(a.ID, b.ID)
	default:
		return ai.After(bi)
	}
}

func compareIDsDesc(a, b string) bool {
	ai, errA := strconv.ParseInt(a, 10, 64)
	bi, errB := strconv.ParseInt(b, 10, 64)
	if errA == nil && errB == nil {
		return ai > bi
	}
	return a > b
}
