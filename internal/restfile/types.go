package restfile

import (
	"maps"
	"net/http"
	"strings"
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
	Kind       string
	Lang       string
	Body       string
	FilePath   string
	SourcePath string       `json:"-"`
	Lines      []ScriptLine `json:"-"`
}

// ScriptLine maps one stored script body line back to its source file position.
type ScriptLine struct {
	Line int
	Col  int
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

// Scenarios that share a method and path merge into one route when compiled.
type Mock struct {
	Title       string
	Name        string
	Sequence    string
	SequenceKey MockSequenceKey
	Method      string
	Path        string
	Latency     time.Duration
	Default     bool
	Match       MockMatch
	Expectation *MockExpectation
	Responses   []MockResponse
	// DisableInterpolation preserves response templates as literal text.
	DisableInterpolation bool
	LineRange            LineRange
}

type MockSequenceKeySource uint8

const (
	MockSequenceKeySourceUnknown MockSequenceKeySource = iota
	MockSequenceKeySourcePath
	MockSequenceKeySourceQuery
	MockSequenceKeySourceHeader
	MockSequenceKeySourceCookie
)

type MockSequenceKey struct {
	Source MockSequenceKeySource
	Name   string
}

func (k MockSequenceKey) IsZero() bool {
	return k.Source == MockSequenceKeySourceUnknown && k.Name == ""
}

type MockExpectation struct {
	Calls uint64
	Line  int
}

type MockHeaderOp uint8

const (
	MockHeaderOpUnknown MockHeaderOp = iota
	MockHeaderOpExact
	MockHeaderOpPrefix
	MockHeaderOpPresent
	MockHeaderOpAbsent
)

type MockHeaderRule struct {
	Op     MockHeaderOp
	Values []string
}

// StringList decodes a JSON string or string array, the value shape shared by
// @match query, @match headers, and mock request patterns.
type StringList []string

type MockMatch struct {
	Query   map[string]StringList
	Headers map[string]MockHeaderRule
	JSON    []byte
}

type MockResponse struct {
	Status  int
	Headers http.Header
	Body    BodySource
}

type BodyOptions struct {
	ExpandTemplates bool
	ForceInline     bool
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
	Line         int
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
	Invalid      bool
	Error        string
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
	Col        int
}

type AssertSpec struct {
	Expression string
	Message    string
	Line       int
	Col        int
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
	Mocks     []*Mock
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
	Expect    WorkflowExpect
	Vars      map[string]string
	Options   map[string]string
	Line      int
	When      *ConditionSpec
	If        *WorkflowIf
	Switch    *WorkflowSwitch
	ForEach   *WorkflowForEach
}

// WorkflowExpect holds the parsed expect assertions of a workflow step.
// StatusCode comes out of the parser already validated. Extra keeps
// unrecognized expect keys so reporting can still show them.
type WorkflowExpect struct {
	Status     string
	StatusCode *int
	Extra      map[string]string
}

func (e WorkflowExpect) HasStatus() bool {
	return e.Status != "" || e.StatusCode != nil
}

func (e WorkflowExpect) Empty() bool {
	return e.Status == "" && e.StatusCode == nil && len(e.Extra) == 0
}

func (e WorkflowExpect) Clone() WorkflowExpect {
	if e.StatusCode != nil {
		n := *e.StatusCode
		e.StatusCode = &n
	}
	e.Extra = maps.Clone(e.Extra)
	return e
}

// WorkflowVarKeys returns the request and workflow scoped variable keys for a
// loop variable name. wfKey is empty unless wf is true.
func WorkflowVarKeys(name string, wf bool) (reqKey, wfKey string) {
	if name == "" {
		return "", ""
	}
	reqKey = "vars.request." + name
	if wf {
		wfKey = "vars.workflow." + name
	}
	return reqKey, wfKey
}

// IsWorkflowScopedVar reports whether key names a workflow scoped variable.
func IsWorkflowScopedVar(key string) bool {
	return strings.HasPrefix(key, "vars.workflow.")
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
	// Mock marks @mock/@match errors so the mock compiler can reject a
	// document without matching on message text.
	Mock bool
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
