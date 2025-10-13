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
}

type ProfileSpec struct {
	Count  int
	Warmup int
	Delay  time.Duration
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
}

const (
	HistoryMethodWorkflow = "WORKFLOW"
)

type Document struct {
	Path      string
	Variables []Variable
	Globals   []Variable
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
