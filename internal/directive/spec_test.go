package directive

import "testing"

func TestSpecsDeclareRepeat(t *testing.T) {
	t.Parallel()

	for _, spec := range specs {
		if spec.Repeat == repeatUnset {
			t.Errorf("%s does not declare Repeat, add Once or Many to its spec", spec.Name.Tag())
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
