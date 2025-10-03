package restfile

import "net/http"

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
	Name        string
	Description string
	Tags        []string
	NoLog       bool
	Auth        *AuthSpec
	Scripts     []ScriptBlock
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

type Document struct {
	Path      string
	Variables []Variable
	Requests  []*Request
	Errors    []ParseError
	Raw       []byte
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
