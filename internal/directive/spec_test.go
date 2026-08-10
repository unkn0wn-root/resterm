package directive

import (
	"slices"
	"testing"
)

func TestSpecsDeclareRepeat(t *testing.T) {
	t.Parallel()

	for _, spec := range specs {
		if spec.Repeat == repeatUnset {
			t.Errorf("%s does not declare Repeat, add Once or Many to its spec", spec.Name.Tag())
		}
	}
}

func TestSpecsRequireValuesTheyCanHold(t *testing.T) {
	t.Parallel()

	for _, spec := range specs {
		if spec.ValueRequired && spec.Args == ArgNone {
			t.Errorf("%s requires a value but takes no argument", spec.Name.Tag())
		}
	}
}

func TestValueRequired(t *testing.T) {
	t.Parallel()

	tests := map[Name]bool{
		RequestName:      true,
		GraphQLOperation: true,
		Operation:        true,
		GraphQL:          false,
		Query:            false,
		Timeout:          false,
		NoLog:            false,
		"nonesuch":       false,
	}
	for name, want := range tests {
		if got := name.ValueRequired(); got != want {
			t.Errorf("%s.ValueRequired() = %t, want %t", name.Tag(), got, want)
		}
	}
}

func TestSpecsResetOnlySingleDeclarationDirectives(t *testing.T) {
	t.Parallel()

	for _, spec := range specs {
		for _, cleared := range spec.Resets {
			if !cleared.DeclaredOnce() {
				t.Errorf(
					"%s resets %s, which is not a single-declaration directive",
					spec.Name.Tag(),
					cleared.Tag(),
				)
			}
		}
	}
}

func TestResets(t *testing.T) {
	t.Parallel()

	want := []Name{GraphQLOperation, Variables, Query}
	if got := GraphQL.Resets(); !slices.Equal(got, want) {
		t.Errorf("%s.Resets() = %v, want %v", GraphQL.Tag(), got, want)
	}
	for _, name := range []Name{Operation, Tag, "nonesuch"} {
		if got := name.Resets(); got != nil {
			t.Errorf("%s.Resets() = %v, want nil", name.Tag(), got)
		}
	}
}

func TestDeclaredOnce(t *testing.T) {
	t.Parallel()

	tests := map[Name]bool{
		Auth:        true,
		RequestName: true,
		When:        true,
		SkipIf:      true,
		Tag:         false,
		Capture:     false,
		SSH:         false,
		"nonesuch":  false,
	}
	for name, want := range tests {
		if got := name.DeclaredOnce(); got != want {
			t.Errorf("%s.DeclaredOnce() = %t, want %t", name.Tag(), got, want)
		}
	}
}

func TestNameContinuation(t *testing.T) {
	tests := map[Name]Continuation{
		Assert:       ContinueExpr,
		SkipIf:       ContinueExpr,
		When:         ContinueExpr,
		Capture:      ContinueCapture,
		Match:        ContinueOptions,
		RequestName:  ContinueNone,
		Step:         ContinueNone,
		Name("nope"): ContinueNone,
	}
	for name, want := range tests {
		t.Run(name.String(), func(t *testing.T) {
			if got := name.Continuation(); got != want {
				t.Fatalf("%s continuation = %d, want %d", name.Tag(), got, want)
			}
		})
	}
}

func TestSpecsContinueOnlyWithStructuredArguments(t *testing.T) {
	for _, spec := range Specs() {
		if spec.Continues == ContinueNone {
			continue
		}
		if spec.Args != ArgText && spec.Args != ArgOptions {
			t.Fatalf("%s continues with args kind %d", spec.Name.Tag(), spec.Args)
		}
	}
}

func TestNameKnown(t *testing.T) {
	tests := map[Name]bool{
		Assert:          true,
		SkipIf:          true,
		Name("nope"):    false,
		Name("ASSERT"):  false,
		Name(""):        false,
		Name("assert:"): false,
	}
	for name, want := range tests {
		t.Run(name.String(), func(t *testing.T) {
			if got := name.Known(); got != want {
				t.Fatalf("%q known = %t, want %t", name, got, want)
			}
		})
	}
}
