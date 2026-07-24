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

// Completion and syntax highlighting both use this metadata.
type Spec struct {
	Name    Name
	Aliases []Name
	Summary string
	Args    ArgKind
}

// Keep this list in completion order. Alias lookup and syntax highlighting also
// come from this table, so a directive added elsewhere would be only partly known.
var specs = []Spec{
	{Name: Mock, Summary: "Define an interpolated mock response or response sequence", Args: ArgOptions},
	{Name: Match, Summary: "Match mock requests by query, header rules, or JSON body", Args: ArgOptions},
	{Name: Expect, Summary: "Declare the expected number of matching mock calls", Args: ArgOptions},
	{Name: RequestName, Summary: "Assign a display name to the request", Args: ArgToken},
	{
		Name:    Description,
		Aliases: []Name{Desc},
		Summary: "Add a multi-line description",
		Args:    ArgText,
	},
	{Name: Tag, Aliases: []Name{Tags}, Summary: "Categorize the request with tags", Args: ArgText},
	{Name: NoLog, Aliases: []Name{Nolog}, Summary: "Disable logging of response bodies"},
	{
		Name:    LogSensitiveHeaders,
		Aliases: []Name{LogSecretHeaders},
		Summary: "Permit logging sensitive headers",
		Args:    ArgToken,
	},
	{Name: Auth, Summary: "Configure authentication (basic, bearer, etc.)", Args: ArgToken},
	{Name: Setting, Summary: "Set options (transport/TLS/etc.)", Args: ArgSetting},
	{Name: Settings, Summary: "Set multiple options on one line", Args: ArgOptions},
	{Name: Timeout, Summary: "Override the request timeout", Args: ArgToken},
	{Name: Body, Summary: "Control body parsing and template expansion", Args: ArgText},
	{Name: Var, Summary: "Declare a request-scoped variable", Args: ArgText},
	{Name: Request, Summary: "Define a request-scoped variable", Args: ArgText},
	{Name: RequestSecret, Summary: "Define a secret request variable", Args: ArgText},
	{Name: File, Summary: "Define a file-scoped variable", Args: ArgText},
	{Name: FileSecret, Summary: "Define a secret file variable", Args: ArgText},
	{Name: Global, Summary: "Define or override a global variable", Args: ArgText},
	{Name: GlobalSecret, Summary: "Define a secret global variable", Args: ArgText},
	{Name: Const, Summary: "Define a reusable constant", Args: ArgText},
	{Name: Use, Summary: "Import a RestermScript module", Args: ArgText},
	{Name: Script, Summary: "Start a pre-request or test script block", Args: ArgToken},
	{Name: RTS, Summary: "Start a RestermScript pre-request block", Args: ArgToken},
	{Name: Patch, Summary: "Define a reusable apply profile at file/global scope", Args: ArgText},
	{
		Name:    Apply,
		Summary: "Apply an inline patch or reuse profiles (use=...) before pre-request scripts",
		Args:    ArgText,
	},
	{Name: When, Aliases: []Name{SkipIf}, Summary: "Conditionally run or skip a request/step", Args: ArgText},
	{Name: Capture, Summary: "Capture data from the response", Args: ArgText},
	{Name: Assert, Summary: "Evaluate a RestermScript assertion", Args: ArgText},
	{Name: Trace, Summary: "Enable HTTP tracing and latency budgets", Args: ArgOptions},
	{Name: Profile, Summary: "Run the request repeatedly with profiling", Args: ArgOptions},
	{Name: Compare, Summary: "Run the request across multiple environments", Args: ArgOptions},
	{Name: SSH, Summary: "Send request via SSH jump host", Args: ArgOptions},
	{Name: K8s, Summary: "Send request via Kubernetes port-forward", Args: ArgOptions},
	{Name: Workflow, Summary: "Begin a workflow definition", Args: ArgText},
	{Name: Step, Summary: "Add a workflow step", Args: ArgText},
	{Name: If, Summary: "Conditionally run a workflow step", Args: ArgText},
	{Name: Elif, Summary: "Additional workflow condition", Args: ArgText},
	{Name: Else, Summary: "Fallback workflow branch", Args: ArgOptions},
	{Name: Switch, Summary: "Branch workflow steps based on a value", Args: ArgText},
	{Name: Case, Summary: "Match a switch case", Args: ArgText},
	{Name: Default, Summary: "Fallback switch case", Args: ArgOptions},
	{Name: ForEach, Summary: "Run a request once per list item", Args: ArgText},
	{Name: GraphQL, Summary: "Enable GraphQL request handling", Args: ArgToken},
	{
		Name:    GraphQLOperation,
		Aliases: []Name{Operation},
		Summary: "Set the GraphQL operation name",
		Args:    ArgToken,
	},
	{
		Name:    Variables,
		Aliases: []Name{GraphQLVariables},
		Summary: "Provide GraphQL variables (JSON)",
		Args:    ArgText,
	},
	{
		Name:    Query,
		Aliases: []Name{GraphQLQuery},
		Summary: "Inline a GraphQL query",
		Args:    ArgText,
	},
	{Name: GRPC, Summary: "Configure the gRPC method (supports streaming)", Args: ArgText},
	{Name: GRPCDescriptor, Summary: "Load a gRPC descriptor set", Args: ArgText},
	{Name: GRPCReflection, Summary: "Toggle gRPC reflection", Args: ArgToken},
	{Name: GRPCPlaintext, Summary: "Force plaintext gRPC transport", Args: ArgToken},
	{Name: GRPCAuthority, Summary: "Set gRPC authority override", Args: ArgText},
	{
		Name:    GRPCMetadata,
		Summary: "Attach gRPC metadata (Repeatable. Reserved keys rejected - use @timeout)",
		Args:    ArgText,
	},
	{Name: SSE, Summary: "Enable Server-Sent Events streaming", Args: ArgOptions},
	{Name: WebSocket, Summary: "Enable WebSocket streaming", Args: ArgOptions},
	{Name: WS, Summary: "Add a WebSocket scripted step (send/ping/wait/close)", Args: ArgText},
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
