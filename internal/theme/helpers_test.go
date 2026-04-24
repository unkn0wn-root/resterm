package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestOrDefault(t *testing.T) {
	custom := Definition{
		Key:         "daybreak",
		DisplayName: "Daybreak",
		Metadata: Metadata{
			Name: "Daybreak",
			Tags: []string{"light"},
		},
		Theme: DefaultTheme(),
	}

	tests := []struct {
		name string
		def  *Definition
		want string
	}{
		{name: "nil definition", def: nil, want: "default"},
		{name: "empty key", def: &Definition{}, want: "default"},
		{name: "whitespace key", def: &Definition{Key: "   "}, want: "default"},
		{name: "named definition", def: &custom, want: "daybreak"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OrDefault(tt.def)
			if got.Key != tt.want {
				t.Fatalf("OrDefault(%v) key = %q, want %q", tt.def, got.Key, tt.want)
			}
		})
	}
}

func TestColorForAppearance(t *testing.T) {
	tests := []struct {
		name    string
		ap      Appearance
		light   string
		dark    string
		want    string
		defined bool
	}{
		{
			name:    "light appearance",
			ap:      AppearanceLight,
			light:   "#64748b",
			dark:    "#A6A1BB",
			want:    "#64748b",
			defined: true,
		},
		{
			name:    "dark appearance",
			ap:      AppearanceDark,
			light:   "#64748b",
			dark:    "#A6A1BB",
			want:    "#A6A1BB",
			defined: true,
		},
		{
			name:    "empty dark fallback means no color",
			ap:      AppearanceDark,
			light:   "#F8FAFC",
			dark:    "",
			defined: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ColorForAppearance(tt.ap, tt.light, tt.dark)
			if ColorDefined(got) != tt.defined {
				t.Fatalf(
					"ColorDefined(ColorForAppearance(...)) = %v, want %v",
					ColorDefined(got),
					tt.defined,
				)
			}
			if !tt.defined {
				return
			}
			if got != lipgloss.Color(tt.want) {
				t.Fatalf("ColorForAppearance(...) = %v, want %v", got, lipgloss.Color(tt.want))
			}
		})
	}
}
