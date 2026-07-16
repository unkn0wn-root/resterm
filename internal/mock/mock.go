// Package mock serves HTTP responses compiled from @mock definitions in .http files.
package mock

import "time"

const (
	DefaultAddr = "127.0.0.1:8080"
	DefaultLogs = 200
)

type Event struct {
	Time     time.Time
	Method   string
	Target   string
	Route    string
	Scenario string
	Source   string
	Status   int
	Duration time.Duration
	Matched  bool
	Error    string
	Reload   bool
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
	CORS    CORS
	Logs    int
	OnEvent func(Event)
}

type Stats struct {
	Addr      string
	Routes    int
	Scenarios int
	Calls     uint64
}
