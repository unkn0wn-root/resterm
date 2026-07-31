package directive

// ArgKind tells the editor how much text after a directive belongs to its value.
type ArgKind uint8

const (
	ArgNone ArgKind = iota
	ArgToken
	ArgText
	ArgSetting
	ArgOptions
)

// Completion, syntax highlighting, and embedded help all use this metadata.
// Topic names the helpdoc topic that documents the directive.
type Spec struct {
	Name    Name
	Aliases []Name
	Summary string
	Args    ArgKind
	Topic   string
}

// Keep this list in completion order. Alias lookup and syntax highlighting also
// come from this table, so a directive added elsewhere would be only partly known.
var specs = []Spec{
	{
		Name:    Mock,
		Summary: "Define an interpolated mock response or response sequence",
		Args:    ArgOptions,
		Topic:   "mocks",
	},
	{
		Name:    Match,
		Summary: "Match mock requests by query, header rules, or JSON body",
		Args:    ArgOptions,
		Topic:   "mocks",
	},
	{Name: Expect, Summary: "Declare the expected number of matching mock calls", Args: ArgOptions, Topic: "mocks"},
	{Name: RequestName, Summary: "Assign a display name to the request", Args: ArgToken, Topic: "requests"},
	{
		Name:    Description,
		Aliases: []Name{Desc},
		Summary: "Add a multi-line description",
		Args:    ArgText,
		Topic:   "requests",
	},
	{Name: Tag, Aliases: []Name{Tags}, Summary: "Categorize the request with tags", Args: ArgText, Topic: "requests"},
	{Name: NoLog, Aliases: []Name{Nolog}, Summary: "Disable logging of response bodies", Topic: "requests"},
	{
		Name:    LogSensitiveHeaders,
		Aliases: []Name{LogSecretHeaders},
		Summary: "Permit logging sensitive headers",
		Args:    ArgToken,
		Topic:   "requests",
	},
	{Name: Auth, Summary: "Configure authentication (basic, bearer, etc.)", Args: ArgToken, Topic: "authentication"},
	{Name: Setting, Summary: "Set options (transport/TLS/etc.)", Args: ArgSetting, Topic: "transport"},
	{Name: Settings, Summary: "Set multiple options on one line", Args: ArgOptions, Topic: "transport"},
	{Name: Timeout, Summary: "Override the request timeout", Args: ArgToken, Topic: "transport"},
	{Name: Body, Summary: "Control body parsing and template expansion", Args: ArgText, Topic: "requests"},
	{Name: Var, Summary: "Declare a request-scoped variable", Args: ArgText, Topic: "variables"},
	{Name: Request, Summary: "Define a request-scoped variable", Args: ArgText, Topic: "variables"},
	{Name: RequestSecret, Summary: "Define a secret request variable", Args: ArgText, Topic: "variables"},
	{Name: File, Summary: "Define a file-scoped variable", Args: ArgText, Topic: "variables"},
	{Name: FileSecret, Summary: "Define a secret file variable", Args: ArgText, Topic: "variables"},
	{Name: Global, Summary: "Define or override a global variable", Args: ArgText, Topic: "variables"},
	{Name: GlobalSecret, Summary: "Define a secret global variable", Args: ArgText, Topic: "variables"},
	{Name: Const, Summary: "Define a reusable constant", Args: ArgText, Topic: "variables"},
	{Name: Use, Summary: "Import a RestermScript module", Args: ArgText, Topic: "rts"},
	{Name: Script, Summary: "Start a pre-request or test script block", Args: ArgToken, Topic: "scripting"},
	{Name: RTS, Summary: "Start a RestermScript pre-request block", Args: ArgToken, Topic: "rts"},
	{Name: Patch, Summary: "Define a reusable apply profile at file/global scope", Args: ArgText, Topic: "rts"},
	{
		Name:    Apply,
		Summary: "Apply an inline patch or reuse profiles (use=...) before pre-request scripts",
		Args:    ArgText,
		Topic:   "rts",
	},
	{
		Name:    When,
		Aliases: []Name{SkipIf},
		Summary: "Conditionally run or skip a request/step",
		Args:    ArgText,
		Topic:   "rts",
	},
	{Name: Capture, Summary: "Capture data from the response", Args: ArgText, Topic: "rts"},
	{Name: Assert, Summary: "Evaluate a RestermScript assertion", Args: ArgText, Topic: "rts"},
	{Name: Trace, Summary: "Enable HTTP tracing and latency budgets", Args: ArgOptions, Topic: "tracing"},
	{Name: Profile, Summary: "Run the request repeatedly with profiling", Args: ArgOptions, Topic: "profiling"},
	{Name: Compare, Summary: "Run the request across multiple environments", Args: ArgOptions, Topic: "comparison"},
	{Name: SSH, Summary: "Send request via SSH jump host", Args: ArgOptions, Topic: "ssh"},
	{Name: K8s, Summary: "Send request via Kubernetes port-forward", Args: ArgOptions, Topic: "kubernetes"},
	{Name: Workflow, Summary: "Begin a workflow definition", Args: ArgText, Topic: "workflows"},
	{Name: Step, Summary: "Add a workflow step", Args: ArgText, Topic: "workflows"},
	{Name: If, Summary: "Conditionally run a workflow step", Args: ArgText, Topic: "workflows"},
	{Name: Elif, Summary: "Additional workflow condition", Args: ArgText, Topic: "workflows"},
	{Name: Else, Summary: "Fallback workflow branch", Args: ArgOptions, Topic: "workflows"},
	{Name: Switch, Summary: "Branch workflow steps based on a value", Args: ArgText, Topic: "workflows"},
	{Name: Case, Summary: "Match a switch case", Args: ArgText, Topic: "workflows"},
	{Name: Default, Summary: "Fallback switch case", Args: ArgOptions, Topic: "workflows"},
	{Name: ForEach, Summary: "Run a request once per list item", Args: ArgText, Topic: "workflows"},
	{Name: GraphQL, Summary: "Enable GraphQL request handling", Args: ArgToken, Topic: "graphql"},
	{
		Name:    GraphQLOperation,
		Aliases: []Name{Operation},
		Summary: "Set the GraphQL operation name",
		Args:    ArgToken,
		Topic:   "graphql",
	},
	{
		Name:    Variables,
		Aliases: []Name{GraphQLVariables},
		Summary: "Provide GraphQL variables (JSON)",
		Args:    ArgText,
		Topic:   "graphql",
	},
	{
		Name:    Query,
		Aliases: []Name{GraphQLQuery},
		Summary: "Inline a GraphQL query",
		Args:    ArgText,
		Topic:   "graphql",
	},
	{Name: GRPC, Summary: "Configure the gRPC method (supports streaming)", Args: ArgText, Topic: "grpc"},
	{Name: GRPCDescriptor, Summary: "Load a gRPC descriptor set", Args: ArgText, Topic: "grpc"},
	{Name: GRPCReflection, Summary: "Toggle gRPC reflection", Args: ArgToken, Topic: "grpc"},
	{Name: GRPCPlaintext, Summary: "Force plaintext gRPC transport", Args: ArgToken, Topic: "grpc"},
	{Name: GRPCAuthority, Summary: "Set gRPC authority override", Args: ArgText, Topic: "grpc"},
	{
		Name:    GRPCMetadata,
		Summary: "Attach gRPC metadata (Repeatable. Reserved keys rejected - use @timeout)",
		Args:    ArgText,
		Topic:   "grpc",
	},
	{Name: SSE, Summary: "Enable Server-Sent Events streaming", Args: ArgOptions, Topic: "streaming"},
	{Name: WebSocket, Summary: "Enable WebSocket streaming", Args: ArgOptions, Topic: "streaming"},
	{Name: WS, Summary: "Add a WebSocket scripted step (send/ping/wait/close)", Args: ArgText, Topic: "streaming"},
}

var index = func() map[Name]*Spec {
	ix := make(map[Name]*Spec, len(specs)*2)
	for i := range specs {
		spec := &specs[i]
		ix[spec.Name] = spec
		for _, alias := range spec.Aliases {
			ix[alias] = spec
		}
	}
	return ix
}()

// Lookup resolves canonical names and aliases alike. The lookup expects lowercase input.
func Lookup(name Name) (Spec, bool) {
	spec, ok := index[name]
	if !ok {
		return Spec{}, false
	}
	return *spec, true
}

// The returned data is shared. Callers must treat the slice and its aliases as read-only.
func Specs() []Spec {
	return specs
}
