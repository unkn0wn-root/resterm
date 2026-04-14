package engine

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/bindings"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/registry"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Config struct {
	FilePath        string
	Client          *httpclient.Client
	EnvironmentSet  vars.EnvironmentSet
	EnvironmentName string
	EnvironmentFile string
	HTTPOptions     httpclient.Options
	GRPCOptions     grpcclient.Options
	SSHManager      *ssh.Manager
	K8sManager      *k8s.Manager
	History         history.Store
	WorkspaceRoot   string
	Recursive       bool
	CompareTargets  []string
	CompareBase     string
	Registry        *registry.Index
	Bindings        *bindings.Map
}

type Executor interface {
	ExecuteRequest(
		doc *restfile.Document,
		req *restfile.Request,
		envOverride string,
	) (RequestResult, error)
	ExecuteWorkflow(
		doc *restfile.Document,
		wf *restfile.Workflow,
		envOverride string,
	) (*WorkflowResult, error)
	ExecuteCompare(
		doc *restfile.Document,
		req *restfile.Request,
		spec *restfile.CompareSpec,
		envOverride string,
	) (*CompareResult, error)
	ExecuteProfile(
		doc *restfile.Document,
		req *restfile.Request,
		envOverride string,
	) (*ProfileResult, error)
	RuntimeState() RuntimeState
	LoadRuntimeState(RuntimeState)
	AuthState() AuthState
	LoadAuthState(AuthState)
	Close() error
}

type RequestResult struct {
	Response       *httpclient.Response
	GRPC           *grpcclient.Response
	Stream         *scripts.StreamInfo
	Transcript     []byte
	Err            error
	Tests          []scripts.TestResult
	ScriptErr      error
	Executed       *restfile.Request
	RequestText    string
	RuntimeSecrets []string
	Environment    string
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

type CompareResult struct {
	Baseline    string
	Environment string
	Summary     string
	Report      string
	Success     bool
	Skipped     bool
	Canceled    bool
	Rows        []CompareRow
}

type CompareRow struct {
	Environment string
	Summary     string
	Response    *httpclient.Response
	GRPC        *grpcclient.Response
	Stream      *scripts.StreamInfo
	Transcript  []byte
	Err         error
	Tests       []scripts.TestResult
	ScriptErr   error
	Skipped     bool
	SkipReason  string
	Canceled    bool
	Success     bool
	Duration    time.Duration
}

type ProfileResult struct {
	Environment string
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
}

type WorkflowResult struct {
	Kind        string
	Name        string
	Environment string
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
	Method     string
	Target     string
	Branch     string
	Iteration  int
	Total      int
	Summary    string
	Response   *httpclient.Response
	GRPC       *grpcclient.Response
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
