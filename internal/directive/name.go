package directive

// Name is the lowercase spelling without the leading @.
type Name string

const (
	Mock                Name = "mock"
	Match               Name = "match"
	Expect              Name = "expect"
	RequestName         Name = "name"
	Description         Name = "description"
	Desc                Name = "desc"
	Tag                 Name = "tag"
	Tags                Name = "tags"
	NoLog               Name = "no-log"
	Nolog               Name = "nolog"
	LogSensitiveHeaders Name = "log-sensitive-headers"
	LogSecretHeaders    Name = "log-secret-headers"
	Auth                Name = "auth"
	Setting             Name = "setting"
	Settings            Name = "settings"
	Timeout             Name = "timeout"
	Body                Name = "body"
	Var                 Name = "var"
	Request             Name = "request"
	RequestSecret       Name = "request-secret"
	File                Name = "file"
	FileSecret          Name = "file-secret"
	Global              Name = "global"
	GlobalSecret        Name = "global-secret"
	Const               Name = "const"
	Use                 Name = "use"
	Script              Name = "script"
	RTS                 Name = "rts"
	Patch               Name = "patch"
	Apply               Name = "apply"
	When                Name = "when"
	SkipIf              Name = "skip-if"
	Capture             Name = "capture"
	Assert              Name = "assert"
	Poll                Name = "poll"
	Retry               Name = "retry"
	RetryWhen           Name = "retry-when"
	RetryBackoff        Name = "retry-backoff"
	Trace               Name = "trace"
	Profile             Name = "profile"
	Compare             Name = "compare"
	SSH                 Name = "ssh"
	K8s                 Name = "k8s"
	Workflow            Name = "workflow"
	Step                Name = "step"
	If                  Name = "if"
	Elif                Name = "elif"
	Else                Name = "else"
	Switch              Name = "switch"
	Case                Name = "case"
	Default             Name = "default"
	ForEach             Name = "for-each"
	GraphQL             Name = "graphql"
	GraphQLOperation    Name = "graphql-operation"
	Operation           Name = "operation"
	Variables           Name = "variables"
	GraphQLVariables    Name = "graphql-variables"
	Query               Name = "query"
	GraphQLQuery        Name = "graphql-query"
	GRPC                Name = "grpc"
	GRPCDescriptor      Name = "grpc-descriptor"
	GRPCReflection      Name = "grpc-reflection"
	GRPCPlaintext       Name = "grpc-plaintext"
	GRPCAuthority       Name = "grpc-authority"
	GRPCMetadata        Name = "grpc-metadata"
	SSE                 Name = "sse"
	WebSocket           Name = "websocket"
	WS                  Name = "ws"
)

func (n Name) String() string {
	return string(n)
}

func (n Name) Tag() string {
	return "@" + string(n)
}

func (n Name) Comment() string {
	return "# " + n.Tag()
}

// Unknown names pass through so each parser can decide whether they are valid.
// The lookup expects lowercase input.
func (n Name) Canonical() Name {
	if spec, ok := index[n]; ok {
		return spec.Name
	}
	return n
}
