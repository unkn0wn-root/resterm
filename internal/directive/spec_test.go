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
