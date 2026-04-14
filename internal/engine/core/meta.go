package core

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Mode uint8

const (
	ModeUnknown Mode = iota
	ModeWorkflow
	ModeForEach
	ModeCompare
	ModeProfile
)

func (m Mode) String() string {
	switch m {
	case ModeWorkflow:
		return "workflow"
	case ModeForEach:
		return "for-each"
	case ModeCompare:
		return "compare"
	case ModeProfile:
		return "profile"
	default:
		return "unknown"
	}
}

type RunMeta struct {
	ID   string
	Mode Mode
	Name string
	Env  string
}

type EvtMeta struct {
	Run RunMeta
	At  time.Time
}

func NewMeta(run RunMeta, at time.Time) EvtMeta {
	if at.IsZero() {
		at = time.Now()
	}
	return EvtMeta{Run: run, At: at}
}

type ReqMeta struct {
	Index int
	Label string
	Env   string
}

type StepMeta struct {
	Index  int
	Name   string
	Kind   restfile.WorkflowStepKind
	Target string
	Branch string
	Iter   int
	Total  int
}

type RowMeta struct {
	Index int
	Env   string
	Base  bool
	Total int
}

type IterMeta struct {
	Index       int
	Total       int
	Warmup      bool
	WarmupIndex int
	WarmupTotal int
	RunIndex    int
	RunTotal    int
	Delay       time.Duration
}
