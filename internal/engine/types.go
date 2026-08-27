package engine

import (
	"cmp"
	"slices"
	"time"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/bindings"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/registry"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	runfail "github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	str "github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Config struct {
	FilePath              string
	Client                *httpx.Client
	Catalog               vars.Catalog
	Selection             vars.Selection
	EnvironmentFile       string
	AllowInteractiveOAuth bool
	HTTPOptions           httpx.Options
	GRPCOptions           grpcx.Options
	SSHManager            *ssh.Manager
	K8sManager            *k8s.Manager
	History               history.Store
	WorkspaceRoot         string
	Recursive             bool
	Compare               CompareConfig
	Registry              *registry.Index
	Bindings              *bindings.Map
	SourceDiagnostics     bool
	MockInspector         mock.Inspector
}

// CompareConfig overrides a request's @compare directive for a whole run.
// Group is set only when Targets name profiles inside one environment group.
type CompareConfig struct {
	Targets []string
	Base    string
	Group   string
}

// Clone copies the target list and trims the baseline and group names.
func (c CompareConfig) Clone() CompareConfig {
	c.Targets = slices.Clone(c.Targets)
	c.Base = str.Trim(c.Base)
	c.Group = str.Trim(c.Group)
	return c
}

type Executor interface {
	ExecuteRequest(
		doc *restfile.Document,
		req *restfile.Request,
		sel vars.Selection,
	) (RequestResult, error)
	ExecuteWorkflow(
		doc *restfile.Document,
		wf *restfile.Workflow,
		sel vars.Selection,
	) (*WorkflowResult, error)
	ExecuteCompare(
		doc *restfile.Document,
		req *restfile.Request,
		spec *restfile.CompareSpec,
		sel vars.Selection,
	) (*CompareResult, error)
	ExecuteProfile(
		doc *restfile.Document,
		req *restfile.Request,
		sel vars.Selection,
	) (*ProfileResult, error)
	RuntimeState() RuntimeState
	LoadRuntimeState(RuntimeState)
	AuthState() AuthState
	LoadAuthState(AuthState)
	Close() error
}

type RequestResult struct {
	Response       *httpx.Response
	GRPC           *grpcx.Response
	Stream         *scripts.StreamInfo
	StreamID       string
	Transcript     []byte
	Err            error
	Tests          []scripts.TestResult
	ScriptErr      error
	Executed       *restfile.Request
	RequestText    string
	RuntimeSecrets []string
	Environment    string
	Selection      vars.Selection
	Skipped        bool
	SkipReason     string
	Preview        bool
	Explain        *xplain.Report
	Timing         Timing
	Compare        *CompareResult
	Profile        *ProfileResult
	Workflow       *WorkflowResult
}

type Timing struct {
	Start     time.Time
	End       time.Time
	Total     time.Duration
	Transport time.Duration
}

// Elapsed is the wall time the run took, including @poll and @retry waits. It
// falls back to transport time for results that were not timed.
func (t Timing) Elapsed() time.Duration {
	return cmp.Or(t.Total, t.Transport)
}

type CompareResult struct {
	Baseline    string
	Group       string
	Environment string
	Selection   vars.Selection
	Summary     string
	Report      string
	Success     bool
	Skipped     bool
	Canceled    bool
	Rows        []CompareRow
}

// Name is the label a compare row is matched by. Grouped compares use the
// profile, flat compares the environment.
func (r CompareRow) Name() string {
	if r.Profile != "" {
		return r.Profile
	}
	return r.Environment
}

type CompareRow struct {
	Environment string
	Profile     string
	Selection   vars.Selection
	Summary     string
	Response    *httpx.Response
	GRPC        *grpcx.Response
	Stream      *scripts.StreamInfo
	Transcript  []byte
	Err         error
	Tests       []scripts.TestResult
	ScriptErr   error
	// RuntimeSecrets contains the values exposed while this row ran.
	RuntimeSecrets []string
	Skipped        bool
	SkipReason     string
	Canceled       bool
	Success        bool
	Duration       time.Duration
}

type ProfileResult struct {
	Environment string
	Selection   vars.Selection
	Summary     string
	Report      string
	StartedAt   time.Time
	EndedAt     time.Time
	Duration    time.Duration
	Count       int
	Warmup      int
	Delay       time.Duration
	Success     bool
	Skipped     bool
	SkipReason  string
	Canceled    bool
	Results     *history.ProfileResults
	Failures    []ProfileFailure
}

type ProfileFailure struct {
	Iteration  int
	Warmup     bool
	Reason     string
	Status     string
	StatusCode int
	Duration   time.Duration
	Err        error
	// Failure preserves classification while typed request errors are still available.
	Failure runfail.Failure
}

type WorkflowResult struct {
	Kind        string
	Name        string
	Environment string
	Selection   vars.Selection
	Summary     string
	Report      string
	StartedAt   time.Time
	EndedAt     time.Time
	Duration    time.Duration
	Success     bool
	Skipped     bool
	Canceled    bool
	Steps       []WorkflowStep
}

type WorkflowStep struct {
	Name       string
	Selection  vars.Selection
	Method     string
	Target     string
	Branch     string
	Iteration  int
	Total      int
	Summary    string
	Response   *httpx.Response
	GRPC       *grpcx.Response
	Stream     *scripts.StreamInfo
	Transcript []byte
	Err        error
	Tests      []scripts.TestResult
	ScriptErr  error
	Skipped    bool
	Canceled   bool
	Success    bool
	Duration   time.Duration
}

type RuntimeState struct {
	Globals []RuntimeGlobal `json:"globals,omitempty"`
	Files   []RuntimeFile   `json:"files,omitempty"`
}

type RuntimeGlobal struct {
	Env       string    `json:"env,omitempty"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	Secret    bool      `json:"secret,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RuntimeFile struct {
	Env       string    `json:"env,omitempty"`
	Path      string    `json:"path,omitempty"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	Secret    bool      `json:"secret,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AuthState struct {
	OAuth   []oauth.SnapshotEntry   `json:"oauth,omitempty"`
	Command []authcmd.SnapshotEntry `json:"command,omitempty"`
}
