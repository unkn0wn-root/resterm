package vars

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestNestedExpansionBasic(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "{{b}}",
		"b": "value",
		"c": "pre-{{b}}-post",
	}))

	out, err := r.ExpandTemplates("{{a}} {{c}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if out != "value pre-value-post" {
		t.Fatalf("unexpected expansion: %q", out)
	}
}

func TestNestedExpansionChain(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "{{b}}",
		"b": "{{c}}",
		"c": "leaf",
	}))

	out, err := r.ExpandTemplates("{{a}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if out != "leaf" {
		t.Fatalf("unexpected expansion: %q", out)
	}
}

func TestNestedExpansionAcrossProviders(t *testing.T) {
	r := NewResolver(
		NewTemplateProvider("request", map[string]string{"url": "{{base}}/v2"}),
		NewMapProvider("environment", map[string]string{"base": "https://api.example.com"}),
	)

	out, err := r.ExpandTemplates("{{url}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if out != "https://api.example.com/v2" {
		t.Fatalf("unexpected expansion: %q", out)
	}
}

func TestNestedExpansionDynamicStablePerResolver(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"trace.id": "{{$uuid}}",
	}))

	out, err := r.ExpandTemplates("{{trace.id}} {{trace.id}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	parts := strings.Fields(out)
	if len(parts) != 2 || parts[0] != parts[1] {
		t.Fatalf("expected stable value across references, got %q", out)
	}
	if !uuidPattern.MatchString(parts[0]) {
		t.Fatalf("expected uuid, got %q", parts[0])
	}

	again, err := r.ExpandTemplates("{{trace.id}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if again != parts[0] {
		t.Fatalf("expected stable value across renders, got %q vs %q", again, parts[0])
	}
}

func TestNestedExpansionDynamicDoesNotLeakIntoStaticExpansion(t *testing.T) {
	t.Parallel()

	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"trace.id": "{{$uuid}}",
	}))

	dynamic, err := r.ExpandTemplates("{{trace.id}}")
	if err != nil {
		t.Fatalf("dynamic expansion: %v", err)
	}
	if !uuidPattern.MatchString(dynamic) {
		t.Fatalf("dynamic expansion = %q, want UUID", dynamic)
	}

	static, err := r.ExpandTemplatesStatic("{{trace.id}}")
	if err == nil {
		t.Fatal("static expansion unexpectedly accepted a nested dynamic")
	}
	if static != "{{trace.id}}" {
		t.Fatalf("static expansion = %q, want unresolved placeholder", static)
	}
}

func TestNestedExpansionProviderAliasSharesMemoizedValue(t *testing.T) {
	t.Parallel()

	r := NewResolver(
		NewTemplateProvider("request", map[string]string{"trace.id": "{{$uuid}}"}),
		NewTemplateProvider("file", map[string]string{"trace.id": "{{$uuid}}"}),
	)

	direct, err := r.ExpandTemplates("{{trace.id}}")
	if err != nil {
		t.Fatalf("direct expansion: %v", err)
	}
	qualified, err := r.ExpandTemplates("{{request.trace.id}}")
	if err != nil {
		t.Fatalf("qualified expansion: %v", err)
	}
	if direct != qualified {
		t.Fatalf("aliases expanded to different values: %q and %q", direct, qualified)
	}
	fileValue, err := r.ExpandTemplates("{{file.trace.id}}")
	if err != nil {
		t.Fatalf("qualified file expansion: %v", err)
	}
	if fileValue == direct {
		t.Fatalf("qualified lower-precedence variable reused request value %q", direct)
	}
}

func TestNestedExpansionDistinctVariablesDiffer(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"trace.id": "{{$uuid}}",
		"span.id":  "{{$uuid}}",
	}))

	a, err := r.ExpandTemplates("{{trace.id}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	b, err := r.ExpandTemplates("{{span.id}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if a == b {
		t.Fatalf("expected distinct variables to expand independently, both %q", a)
	}
}

func TestInlineDynamicStaysFreshPerOccurrence(t *testing.T) {
	r := NewResolver()
	out, err := r.ExpandTemplates("{{$uuid}} {{$uuid}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	parts := strings.Fields(out)
	if len(parts) != 2 || parts[0] == parts[1] {
		t.Fatalf("expected fresh inline dynamics, got %q", out)
	}
}

func TestNestedExpansionCycle(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "{{b}}",
		"b": "{{a}}",
	}))

	out, err := r.ExpandTemplates("{{a}}")
	if err == nil || !strings.Contains(err.Error(), "variable cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
	if !strings.Contains(err.Error(), "a -> b -> a") {
		t.Fatalf("expected cycle chain in error, got %v", err)
	}
	if out != "{{a}}" {
		t.Fatalf("expected placeholder to stay literal on cycle, got %q", out)
	}
}

func TestNestedExpansionSelfCycle(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "x{{a}}y",
	}))

	_, err := r.ExpandTemplates("{{a}}")
	if err == nil || !strings.Contains(err.Error(), "variable cycle") {
		t.Fatalf("expected self cycle error, got %v", err)
	}
}

func TestNestedExpansionDepthLimit(t *testing.T) {
	values := make(map[string]string)
	for i := range 20 {
		values[fmt.Sprintf("v%d", i)] = fmt.Sprintf("{{v%d}}", i+1)
	}
	values["v20"] = "leaf"
	r := NewResolver(NewTemplateProvider("request", values))

	_, err := r.ExpandTemplates("{{v0}}")
	if err == nil || !strings.Contains(err.Error(), "nesting deeper") {
		t.Fatalf("expected depth limit error, got %v", err)
	}
}

func TestDataProviderValuesStayVerbatim(t *testing.T) {
	r := NewResolver(
		NewMapProvider("global", map[string]string{"captured": "{{secret}}"}),
		NewTemplateProvider("file", map[string]string{"secret": "hunter2"}),
	)

	out, err := r.ExpandTemplates("{{captured}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if out != "{{secret}}" {
		t.Fatalf("expected captured data to stay verbatim, got %q", out)
	}
}

func TestNestedExpansionUndefinedFails(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "{{missing}}",
	}))

	out, err := r.ExpandTemplates("{{a}}")
	if err == nil {
		t.Fatalf("expected error for undefined nested variable")
	}
	msg := err.Error()
	if !strings.Contains(msg, "expand a") || !strings.Contains(msg, "undefined variable: missing") {
		t.Fatalf("expected error naming both variables, got %q", msg)
	}
	if out != "{{a}}" {
		t.Fatalf("expected placeholder to stay literal, got %q", out)
	}
}

func TestNestedExpansionComposesWithEnvRef(t *testing.T) {
	t.Setenv("RESTERM_TEST_NESTED", "from-os")
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a":   "env:{{key}}",
		"key": "RESTERM_TEST_NESTED",
	}))
	r.AddRefResolver(EnvRefResolver)

	out, err := r.ExpandTemplates("{{a}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if out != "from-os" {
		t.Fatalf("expected env ref to resolve after expansion, got %q", out)
	}
}

func TestEnvRefResultNotExpanded(t *testing.T) {
	t.Setenv("RESTERM_TEST_NESTED", "{{secret}}")
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a":      "env:RESTERM_TEST_NESTED",
		"secret": "hunter2",
	}))
	r.AddRefResolver(EnvRefResolver)

	out, err := r.ExpandTemplates("{{a}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if out != "{{secret}}" {
		t.Fatalf("expected OS env content to stay verbatim, got %q", out)
	}
}

func TestNestedExpansionCaseInsensitive(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"trace.id": "{{$uuid}}",
	}))

	a, err := r.ExpandTemplates("{{Trace.ID}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	b, err := r.ExpandTemplates("{{trace.id}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if a != b {
		t.Fatalf("expected case-insensitive references to share one value, got %q vs %q", a, b)
	}
}

func TestNestedExpansionStaticContextRejectsDynamics(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "{{$uuid}}",
	}))

	out, err := r.ExpandTemplatesStatic("{{a}}")
	if err == nil {
		t.Fatalf("expected error for dynamic in static context")
	}
	if out != "{{a}}" {
		t.Fatalf("expected placeholder to stay literal, got %q", out)
	}
}

func TestPublicResolveExpands(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "{{b}}",
		"b": "value",
	}))

	got, ok := r.Resolve("a")
	if !ok || got != "value" {
		t.Fatalf("Resolve(a) = %q, %v", got, ok)
	}
}

func TestPublicResolveCycleReportsUnresolved(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "{{a}}",
	}))

	if got, ok := r.Resolve("a"); ok {
		t.Fatalf("expected cycle to resolve as missing, got %q", got)
	}
}

func TestNestedExpansionTraceIncludesInner(t *testing.T) {
	r := NewResolver(NewTemplateProvider("request", map[string]string{
		"a": "{{b}}",
		"b": "value",
	}))
	tr := NewTrace()
	r.SetTrace(tr)

	if _, err := r.ExpandTemplates("{{a}}"); err != nil {
		t.Fatalf("expand err: %v", err)
	}

	names := make(map[string]bool)
	for _, it := range tr.Items() {
		names[strings.ToLower(it.Name)] = true
	}
	if !names["a"] || !names["b"] {
		t.Fatalf("expected trace to include both outer and inner variables, got %v", names)
	}
}
