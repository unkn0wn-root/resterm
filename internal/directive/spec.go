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

type Repeat uint8

const (
	repeatUnset Repeat = iota
	Once
	Many
)

// Spec describes a directive for completion, syntax highlighting, and help.
// Topic identifies the corresponding help topic. Resets names directives whose
// declarations are cleared when the feature is disabled.
//
// ValueRequired is set only when an empty argument is invalid. Toggles and
// directives that collect the lines below them must leave it unset.
//
// Continues is explicit because one ArgKind may use different grammars.
type Spec struct {
	Name          Name
	Aliases       []Name
	Summary       string
	Args          ArgKind
	Repeat        Repeat
	Continues     Continuation
	ValueRequired bool
	Resets        []Name
	Topic         string
}

// Keep this list in completion order. Alias lookup and syntax highlighting also
// come from this table, so a directive added elsewhere would be only partly known.
var specs = []Spec{
	{
		Name:    Mock,
		Summary: "Define an interpolated mock response or response sequence",
		Args:    ArgOptions,
		Repeat:  Many,
		Topic:   "mocks",
	},
	{
		Name:      Match,
		Summary:   "Match mock requests by query, header rules, or JSON body",
		Args:      ArgOptions,
		Repeat:    Many,
		Continues: ContinueOptions,
		Topic:     "mocks",
	},
	{
		Name:    Expect,
		Summary: "Declare the expected number of matching mock calls",
		Args:    ArgOptions,
		Repeat:  Once,
		Topic:   "mocks",
	},
	{
		Name:          RequestName,
		Summary:       "Assign a display name to the request",
		Args:          ArgToken,
		Repeat:        Once,
		ValueRequired: true,
		Topic:         "requests",
	},
	{
		Name:    Description,
		Aliases: []Name{Desc},
		Summary: "Add a multi-line description",
		Args:    ArgText,
		Repeat:  Many,
		Topic:   "requests",
	},
	{
		Name:    Tag,
		Aliases: []Name{Tags},
		Summary: "Categorize the request with tags",
		Args:    ArgText,
		Repeat:  Many,
		Topic:   "requests",
	},
	{
		Name:    NoLog,
		Aliases: []Name{Nolog},
		Summary: "Disable logging of response bodies",
		Repeat:  Once,
		Topic:   "requests",
	},
	{
		Name:    LogSensitiveHeaders,
		Aliases: []Name{LogSecretHeaders},
		Summary: "Permit logging sensitive headers",
		Args:    ArgToken,
		Repeat:  Once,
		Topic:   "requests",
	},
	{
		Name:    Auth,
		Summary: "Configure authentication (basic, bearer, etc.)",
		Args:    ArgToken,
		Repeat:  Once,
		Topic:   "authentication",
	},
	{Name: Setting, Summary: "Set options (transport/TLS/etc.)", Args: ArgSetting, Repeat: Many, Topic: "transport"},
	{Name: Settings, Summary: "Set multiple options on one line", Args: ArgOptions, Repeat: Many, Topic: "transport"},
	{Name: Timeout, Summary: "Override the request timeout", Args: ArgToken, Repeat: Once, Topic: "transport"},
	{
		Name:    Body,
		Summary: "Control body parsing and template expansion",
		Args:    ArgText,
		Repeat:  Many,
		Topic:   "requests",
	},
	{Name: Var, Summary: "Declare a request-scoped variable", Args: ArgText, Repeat: Many, Topic: "variables"},
	{Name: Request, Summary: "Define a request-scoped variable", Args: ArgText, Repeat: Many, Topic: "variables"},
	{Name: RequestSecret, Summary: "Define a secret request variable", Args: ArgText, Repeat: Many, Topic: "variables"},
	{Name: File, Summary: "Define a file-scoped variable", Args: ArgText, Repeat: Many, Topic: "variables"},
	{Name: FileSecret, Summary: "Define a secret file variable", Args: ArgText, Repeat: Many, Topic: "variables"},
	{Name: Global, Summary: "Define or override a global variable", Args: ArgText, Repeat: Many, Topic: "variables"},
	{Name: GlobalSecret, Summary: "Define a secret global variable", Args: ArgText, Repeat: Many, Topic: "variables"},
	{Name: Const, Summary: "Define a reusable constant", Args: ArgText, Repeat: Many, Topic: "variables"},
	{Name: Use, Summary: "Import a RestermScript module", Args: ArgText, Repeat: Many, Topic: "rts"},
	{
		Name:    Script,
		Summary: "Start a pre-request or test script block",
		Args:    ArgToken,
		Repeat:  Many,
		Topic:   "scripting",
	},
	{Name: RTS, Summary: "Start a RestermScript pre-request block", Args: ArgToken, Repeat: Many, Topic: "rts"},
	{
		Name:      Patch,
		Summary:   "Define a reusable apply profile at file/global scope",
		Args:      ArgText,
		Repeat:    Many,
		Continues: ContinueExpr,
		Topic:     "rts",
	},
	{
		Name:      Apply,
		Summary:   "Apply an inline patch or reuse profiles (use=...) before pre-request scripts",
		Args:      ArgText,
		Repeat:    Many,
		Continues: ContinueExpr,
		Topic:     "rts",
	},
	{
		Name:      When,
		Aliases:   []Name{SkipIf},
		Summary:   "Conditionally run or skip a request/step",
		Args:      ArgText,
		Repeat:    Once,
		Continues: ContinueExpr,
		Topic:     "rts",
	},
	{
		Name:      Capture,
		Summary:   "Capture data from the response",
		Args:      ArgText,
		Repeat:    Many,
		Continues: ContinueCapture,
		Topic:     "rts",
	},
	{
		Name:      Assert,
		Summary:   "Evaluate a RestermScript assertion",
		Args:      ArgText,
		Repeat:    Many,
		Continues: ContinueExpr,
		Topic:     "rts",
	},
	{
		Name:          Poll,
		Summary:       "Poll until a response expression becomes true",
		Args:          ArgText,
		Repeat:        Once,
		Continues:     ContinueExpr,
		ValueRequired: true,
		Topic:         "polling",
	},
	{
		Name:          Retry,
		Summary:       "Retry a request after a network error or timeout",
		Args:          ArgOptions,
		Repeat:        Once,
		ValueRequired: true,
		Topic:         "polling",
	},
	{
		Name:          RetryWhen,
		Summary:       "Retry when a response condition is true",
		Args:          ArgText,
		Repeat:        Once,
		Continues:     ContinueExpr,
		ValueRequired: true,
		Topic:         "polling",
	},
	{
		Name:          RetryBackoff,
		Summary:       "Set the delay between retries",
		Args:          ArgOptions,
		Repeat:        Once,
		Continues:     ContinueExpr,
		ValueRequired: true,
		Topic:         "polling",
	},
	{Name: Trace, Summary: "Enable HTTP tracing and latency budgets", Args: ArgOptions, Repeat: Once, Topic: "tracing"},
	{
		Name:    Profile,
		Summary: "Run the request repeatedly with profiling",
		Args:    ArgOptions,
		Repeat:  Once,
		Topic:   "profiling",
	},
	{
		Name:    Compare,
		Summary: "Run the request across multiple environments",
		Args:    ArgOptions,
		Repeat:  Once,
		Topic:   "comparison",
	},
	// SSH and K8s parse their scope before checking for duplicates.
	{Name: SSH, Summary: "Send request via SSH jump host", Args: ArgOptions, Repeat: Many, Topic: "ssh"},
	{
		Name:    K8s,
		Summary: "Send request via Kubernetes port-forward",
		Args:    ArgOptions,
		Repeat:  Many,
		Topic:   "kubernetes",
	},
	{Name: Workflow, Summary: "Begin a workflow definition", Args: ArgText, Repeat: Many, Topic: "workflows"},
	{Name: Step, Summary: "Add a workflow step", Args: ArgText, Repeat: Many, Topic: "workflows"},
	{
		Name:      If,
		Summary:   "Conditionally run a workflow step",
		Args:      ArgText,
		Repeat:    Many,
		Continues: ContinueExpr,
		Topic:     "workflows",
	},
	{
		Name:      Elif,
		Summary:   "Additional workflow condition",
		Args:      ArgText,
		Repeat:    Many,
		Continues: ContinueExpr,
		Topic:     "workflows",
	},
	{Name: Else, Summary: "Fallback workflow branch", Args: ArgOptions, Repeat: Many, Topic: "workflows"},
	{
		Name:      Switch,
		Summary:   "Branch workflow steps based on a value",
		Args:      ArgText,
		Repeat:    Many,
		Continues: ContinueExpr,
		Topic:     "workflows",
	},
	{
		Name:      Case,
		Summary:   "Match a switch case",
		Args:      ArgText,
		Repeat:    Many,
		Continues: ContinueExpr,
		Topic:     "workflows",
	},
	{Name: Default, Summary: "Fallback switch case", Args: ArgOptions, Repeat: Many, Topic: "workflows"},
	{
		Name:      ForEach,
		Summary:   "Run a request once per list item",
		Args:      ArgText,
		Repeat:    Once,
		Continues: ContinueExpr,
		Topic:     "workflows",
	},
	// Protocol toggles may repeat because disabling them resets their state.
	{
		Name:    GraphQL,
		Summary: "Enable GraphQL request handling",
		Args:    ArgToken,
		Repeat:  Many,
		Resets:  []Name{GraphQLOperation, Variables, Query},
		Topic:   "graphql",
	},
	{
		Name:          GraphQLOperation,
		Aliases:       []Name{Operation},
		Summary:       "Set the GraphQL operation name",
		Args:          ArgToken,
		Repeat:        Once,
		ValueRequired: true,
		Topic:         "graphql",
	},
	{
		Name:    Variables,
		Aliases: []Name{GraphQLVariables},
		Summary: "Provide GraphQL variables (JSON)",
		Args:    ArgText,
		Repeat:  Once,
		Topic:   "graphql",
	},
	{
		Name:    Query,
		Aliases: []Name{GraphQLQuery},
		Summary: "Inline a GraphQL query",
		Args:    ArgText,
		Repeat:  Once,
		Topic:   "graphql",
	},
	{Name: GRPC, Summary: "Configure the gRPC method (supports streaming)", Args: ArgText, Repeat: Once, Topic: "grpc"},
	{
		Name:          GRPCDescriptor,
		Summary:       "Load a gRPC descriptor set",
		Args:          ArgText,
		Repeat:        Once,
		ValueRequired: true,
		Topic:         "grpc",
	},
	{Name: GRPCReflection, Summary: "Toggle gRPC reflection", Args: ArgToken, Repeat: Once, Topic: "grpc"},
	{Name: GRPCPlaintext, Summary: "Force plaintext gRPC transport", Args: ArgToken, Repeat: Once, Topic: "grpc"},
	{
		Name:          GRPCAuthority,
		Summary:       "Set gRPC authority override",
		Args:          ArgText,
		Repeat:        Once,
		ValueRequired: true,
		Topic:         "grpc",
	},
	{
		Name:          GRPCMetadata,
		Summary:       "Attach gRPC metadata (Repeatable. Reserved keys rejected - use @timeout)",
		Args:          ArgText,
		Repeat:        Many,
		ValueRequired: true,
		Topic:         "grpc",
	},
	{Name: SSE, Summary: "Enable Server-Sent Events streaming", Args: ArgOptions, Repeat: Many, Topic: "streaming"},
	{Name: WebSocket, Summary: "Enable WebSocket streaming", Args: ArgOptions, Repeat: Many, Topic: "streaming"},
	{
		Name:    WS,
		Summary: "Add a WebSocket scripted step (send/ping/wait/close)",
		Args:    ArgText,
		Repeat:  Many,
		Topic:   "streaming",
	},
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

// Known reports whether n is a registered directive name or alias.
func (n Name) Known() bool {
	_, ok := index[n]
	return ok
}

func (n Name) DeclaredOnce() bool {
	spec, ok := index[n]
	return ok && spec.Repeat == Once
}

func (n Name) ValueRequired() bool {
	spec, ok := index[n]
	return ok && spec.ValueRequired
}

// Resets returns the directive names cleared when this feature is disabled.
// The returned slice must not be modified.
func (n Name) Resets() []Name {
	spec, ok := index[n]
	if !ok {
		return nil
	}
	return spec.Resets
}

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
