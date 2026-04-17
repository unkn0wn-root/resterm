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
	// SourcePath tracks the file that defined this auth so relative command auth
	// execution stays anchored to the auth definition, not the consuming request.
	SourcePath string
}

type AuthScope int

const (
	AuthScopeRequest AuthScope = iota
	AuthScopeFile
	AuthScopeGlobal
)

type AuthProfile struct {
	Scope      AuthScope
	Name       string
	Spec       AuthSpec
	Line       int
	SourcePath string
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

type K8sScope int

const (
	K8sScopeRequest K8sScope = iota
	K8sScopeFile
	K8sScopeGlobal
)

type PatchScope int

const (
	PatchScopeFile PatchScope = iota
	PatchScopeGlobal
)

type Opt[T any] struct {
	Val T
	Set bool
}

type SSHOpt[T any] = Opt[T]

type K8sOpt[T any] = Opt[T]

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
	Agent        Opt[bool]
	KnownHosts   string
	Strict       Opt[bool]
	Persist      Opt[bool]
	Timeout      Opt[time.Duration]
	TimeoutStr   string
	KeepAlive    Opt[time.Duration]
	KeepAliveStr string
	Retries      Opt[int]
	RetriesStr   string
}

type SSHSpec struct {
	Use    string
	Inline *SSHProfile
}

type K8sProfile struct {
	Scope        K8sScope
	Name         string
	Namespace    string
	Target       string
	Pod          string
	Port         int
	PortStr      string
	Context      string
	Kubeconfig   string
	Container    string
	Address      string
	LocalPort    int
	LocalPortStr string
	Persist      Opt[bool]
	PodWait      Opt[time.Duration]
	PodWaitStr   string
	Retries      Opt[int]
	RetriesStr   string
}

type K8sSpec struct {
	Use    string
	Inline *K8sProfile
}

type MetadataPair struct {
	Key   string
	Value string
}

type GRPCRequest struct {
	Target             string
	Package            string
	Service            string
	Method             string
	FullMethod         string
	DescriptorSet      string
	UseReflection      bool
	Plaintext          bool
	PlaintextSet       bool
	Authority          string
	Message            string
	MessageFile        string
	MessageExpanded    string
	MessageExpandedSet bool
	Metadata           []MetadataPair
}

type RequestMetadata struct {
	Name                  string
	Description           string
	Tags                  []string
	NoLog                 bool
	AllowSensitiveHeaders bool
	Auth                  *AuthSpec
	AuthDisabled          bool
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

type CaptureExprMode uint8

const (
	CaptureExprModeAuto CaptureExprMode = iota
	CaptureExprModeTemplate
	CaptureExprModeRTS
)

type CaptureSpec struct {
	Scope      CaptureScope
	Name       string
	Expression string
	Mode       CaptureExprMode
	Secret     bool
	Line       int
}

type AssertSpec struct {
	Expression string
	Message    string
	Line       int
}

type ApplySpec struct {
	Uses       []string
	Expression string
	Line       int
	Col        int
}

type PatchProfile struct {
	Scope      PatchScope
	Name       string
	Expression string
	Line       int
	Col        int
	SourcePath string
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
	K8s          *K8sSpec
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
	Auth      []AuthProfile
	SSH       []SSHProfile
	K8s       []K8sProfile
	Patches   []PatchProfile
	Settings  map[string]string
	Uses      []UseSpec
	Requests  []*Request
	Workflows []Workflow
	Errors    []ParseError
	Warnings  []ParseDiagnostic
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

type ParseDiagnostic = ParseError

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
