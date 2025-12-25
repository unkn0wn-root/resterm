package restfile

import (
	"net/http"
	"time"
)

type VariableScope int

const (
	ScopeFile VariableScope = iota
	ScopeRequest
	ScopeGlobal
)

type LineRange struct {
	Start int
	End   int
}

type Variable struct {
	Name   string
	Value  string
	Scope  VariableScope
	Line   int
	Secret bool
}

type Constant struct {
	Name  string
	Value string
	Line  int
}

type AuthSpec struct {
	Type   string
	Params map[string]string
}

type ScriptBlock struct {
	Kind     string
	Lang     string
	Body     string
	FilePath string
}

type UseSpec struct {
	Path  string
	Alias string
	Line  int
}

type ConditionSpec struct {
	Expression string
	Line       int
	Col        int
	Negate     bool
}

type ForEachSpec struct {
	Expression string
	Var        string
	Line       int
	Col        int
}

type BodySource struct {
	Text     string
	FilePath string
	MimeType string
	GraphQL  *GraphQLBody
	Options  BodyOptions
}

type BodyOptions struct {
	ExpandTemplates bool
}

type GraphQLBody struct {
	Query         string
	QueryFile     string
	Variables     string
	VariablesFile string
	OperationName string
}

type SSHScope int

const (
	SSHScopeRequest SSHScope = iota
	SSHScopeFile
	SSHScopeGlobal
)

type SSHOpt[T any] struct {
	Val T
	Set bool
}

type SSHProfile struct {
	Scope        SSHScope
	Name         string
	Host         string
	Port         int
	PortStr      string
	User         string
	Pass         string
	Key          string
	KeyPass      string
	Agent        SSHOpt[bool]
	KnownHosts   string
	Strict       SSHOpt[bool]
	Persist      SSHOpt[bool]
	Timeout      SSHOpt[time.Duration]
	TimeoutStr   string
	KeepAlive    SSHOpt[time.Duration]
	KeepAliveStr string
	Retries      SSHOpt[int]
	RetriesStr   string
}

type SSHSpec struct {
	Use    string
	Inline *SSHProfile
}

type GRPCRequest struct {
	Target        string
	Package       string
	Service       string
	Method        string
	FullMethod    string
	DescriptorSet string
	UseReflection bool
	Plaintext     bool
	PlaintextSet  bool
	Authority     string
	Message       string
	MessageFile   string
	Metadata      map[string]string
}

type RequestMetadata struct {
	Name                  string
	Description           string
	Tags                  []string
	NoLog                 bool
	AllowSensitiveHeaders bool
	Auth                  *AuthSpec
	Scripts               []ScriptBlock
	Uses                  []UseSpec
	Applies               []ApplySpec
	When                  *ConditionSpec
	ForEach               *ForEachSpec
	Asserts               []AssertSpec
	Captures              []CaptureSpec
	Profile               *ProfileSpec
	Trace                 *TraceSpec
	Compare               *CompareSpec
}

type ProfileSpec struct {
	Count  int
	Warmup int
	Delay  time.Duration
}

type TraceSpec struct {
	Enabled bool
	Budgets TraceBudget
}

type TraceBudget struct {
	Total     time.Duration
	Tolerance time.Duration
	Phases    map[string]time.Duration
}

type CompareSpec struct {
	Environments []string
	Baseline     string
}

type CaptureScope int

const (
	CaptureScopeRequest CaptureScope = iota
	CaptureScopeFile
	CaptureScopeGlobal
)

type CaptureSpec struct {
	Scope      CaptureScope
	Name       string
	Expression string
	Secret     bool
}

type AssertSpec struct {
	Expression string
	Message    string
	Line       int
}

type ApplySpec struct {
	Expression string
	Line       int
	Col        int
}

type Request struct {
	Metadata     RequestMetadata
	Method       string
	URL          string
	Headers      http.Header
	Body         BodySource
	Variables    []Variable
	Settings     map[string]string
	LineRange    LineRange
	OriginalText string
	GRPC         *GRPCRequest
	SSE          *SSERequest
	WebSocket    *WebSocketRequest
	SSH          *SSHSpec
}

type SSERequest struct {
	Options SSEOptions
}

type SSEOptions struct {
	TotalTimeout time.Duration
	IdleTimeout  time.Duration
	MaxEvents    int
	MaxBytes     int64
}

type WebSocketRequest struct {
	Options WebSocketOptions
	Steps   []WebSocketStep
}

type WebSocketOptions struct {
	HandshakeTimeout time.Duration
	IdleTimeout      time.Duration
	MaxMessageBytes  int64
	Subprotocols     []string
	Compression      bool
	CompressionSet   bool
}

type WebSocketStepType string

const (
	WebSocketStepSendText   WebSocketStepType = "send_text"
	WebSocketStepSendJSON   WebSocketStepType = "send_json"
	WebSocketStepSendBase64 WebSocketStepType = "send_base64"
	WebSocketStepSendFile   WebSocketStepType = "send_file"
	WebSocketStepPing       WebSocketStepType = "ping"
	WebSocketStepPong       WebSocketStepType = "pong"
	WebSocketStepWait       WebSocketStepType = "wait"
	WebSocketStepClose      WebSocketStepType = "close"
)

type WebSocketStep struct {
	Type     WebSocketStepType
	Value    string
	File     string
	Duration time.Duration
	Code     int
	Reason   string
}

const (
	HistoryMethodWorkflow = "WORKFLOW"
	HistoryMethodCompare  = "COMPARE"
)

type Document struct {
	Path      string
	Variables []Variable
	Globals   []Variable
	Constants []Constant
	SSH       []SSHProfile
	Settings  map[string]string
	Uses      []UseSpec
	Requests  []*Request
	Workflows []Workflow
	Errors    []ParseError
	Raw       []byte
}

type WorkflowFailureMode string

const (
	WorkflowOnFailureStop     WorkflowFailureMode = "stop"
	WorkflowOnFailureContinue WorkflowFailureMode = "continue"
)

type Workflow struct {
	Name             string
	Description      string
	Tags             []string
	DefaultOnFailure WorkflowFailureMode
	Options          map[string]string
	Steps            []WorkflowStep
	LineRange        LineRange
}

type WorkflowStepKind string

const (
	WorkflowStepKindRequest WorkflowStepKind = "step"
	WorkflowStepKindIf      WorkflowStepKind = "if"
	WorkflowStepKindSwitch  WorkflowStepKind = "switch"
	WorkflowStepKindForEach WorkflowStepKind = "for-each"
)

type WorkflowStep struct {
	Kind      WorkflowStepKind
	Name      string
	Using     string
	OnFailure WorkflowFailureMode
	Expect    map[string]string
	Vars      map[string]string
	Options   map[string]string
	Line      int
	When      *ConditionSpec
	If        *WorkflowIf
	Switch    *WorkflowSwitch
	ForEach   *WorkflowForEach
}

type WorkflowIf struct {
	Cond  string
	Then  WorkflowIfBranch
	Elifs []WorkflowIfBranch
	Else  *WorkflowIfBranch
	Line  int
}

type WorkflowIfBranch struct {
	Cond string
	Run  string
	Fail string
	Line int
}

type WorkflowSwitch struct {
	Expr    string
	Cases   []WorkflowSwitchCase
	Default *WorkflowSwitchCase
	Line    int
}

type WorkflowSwitchCase struct {
	Expr string
	Run  string
	Fail string
	Line int
}

type WorkflowForEach struct {
	Expr string
	Var  string
	Line int
}

type ParseError struct {
	Line    int
	Column  int
	Message string
}

func (e ParseError) Error() string {
	return e.Message
}

type DocumentIndex struct {
	Requests []*IndexedRequest
}

type IndexedRequest struct {
	Request *Request
	Range   LineRange
	Index   int
}
