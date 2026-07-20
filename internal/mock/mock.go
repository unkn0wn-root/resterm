// Package mock serves HTTP responses compiled from @mock definitions in .http files.
package mock

import (
	"fmt"
	"time"
)

const (
	DefaultAddr             = "127.0.0.1:8080"
	DefaultLogs             = 200
	DefaultSequenceKeyLimit = 10_000
	DefaultJournalEntries   = 2000
	DefaultJournalBytes     = 16 << 20
	DefaultJournalBodyLimit = 64 << 10
)

type Event struct {
	Time          time.Time
	Method        string
	Target        string
	Route         string
	Scenario      string
	SequenceStep  int
	SequenceTotal int
	Source        string
	Status        int
	Duration      time.Duration
	Matched       bool
	Error         string
	Reload        bool
}

// ScenarioLabel includes sequence progress when the event came from a response sequence.
func (e Event) ScenarioLabel() string {
	if e.Scenario == "" || e.SequenceStep <= 0 || e.SequenceTotal <= 1 {
		return e.Scenario
	}
	return fmt.Sprintf("%s %d/%d", e.Scenario, e.SequenceStep, e.SequenceTotal)
}

type CORS struct {
	Enabled  bool
	Wildcard bool
	Origins  []string
}

func WildcardCORS() CORS {
	return CORS{Enabled: true, Wildcard: true}
}

type Options struct {
	CORS             CORS
	EnableControl    bool
	Logs             int
	SequenceKeyLimit int
	JournalEntries   int
	JournalBytes     int64
	JournalBodyLimit int64
	OnEvent          func(Event)
	// TLSCert and TLSKey are PEM file paths. When set, the server speaks HTTPS.
	TLSCert string
	TLSKey  string
}

type Stats struct {
	Addr      string
	Routes    int
	Scenarios int
	Calls     uint64
}
