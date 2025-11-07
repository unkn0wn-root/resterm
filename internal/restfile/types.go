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
	Body     string
	FilePath string
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
	ReceiveTimeout   time.Duration
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

type WorkflowStep struct {
	Name      string
	Using     string
	OnFailure WorkflowFailureMode
	Expect    map[string]string
	Vars      map[string]string
	Options   map[string]string
	Line      int
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
