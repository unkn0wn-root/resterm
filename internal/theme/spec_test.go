package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func strPtr(value string) *string {
	return &value
}

func TestApplySpecOverridesColorsAndMetadata(t *testing.T) {
	base := DefaultTheme()
	spec := ThemeSpec{
		Colors: ColorsSpec{
			PaneActiveForeground: strPtr("#123456"),
		},
		HeaderSegments: []HeaderSegmentSpec{{
			Background: strPtr("#111111"),
			Foreground: strPtr("#eeeeee"),
		}},
		CommandSegments: []CommandSegmentSpec{{
			Key:  strPtr("#abcdef"),
			Text: strPtr("#fedcba"),
		}},
		EditorMetadata: &EditorMetadataSpec{
			CommentMarker: strPtr("#222222"),
			DirectiveColors: map[string]string{
				"custom": "#333333",
			},
		},
		Styles: StylesSpec{
			ListItemTitle:       &StyleSpec{Foreground: strPtr("#222233")},
			ListItemDescription: &StyleSpec{Foreground: strPtr("#9999aa")},
			ResponseContentRaw:  &StyleSpec{Foreground: strPtr("#abcdef")},
			StreamContent:       &StyleSpec{Foreground: strPtr("#123123")},
		},
	}

	updated, err := ApplySpec(base, spec)
	if err != nil {
		t.Fatalf("ApplySpec returned error: %v", err)
	}

	if got := updated.PaneActiveForeground; got != "#123456" {
		t.Errorf("expected pane active foreground %q, got %q", "#123456", got)
	}
	if len(updated.HeaderSegments) != 1 {
		t.Fatalf("expected 1 header segment, got %d", len(updated.HeaderSegments))
	}
	if updated.HeaderSegments[0].Background != "#111111" {
		t.Errorf(
			"expected header background override, got %q",
			updated.HeaderSegments[0].Background,
		)
	}
	if len(updated.CommandSegments) != 1 {
		t.Fatalf("expected 1 command segment, got %d", len(updated.CommandSegments))
	}
	if updated.CommandSegments[0].Key != "#abcdef" {
		t.Errorf("expected command key color override, got %q", updated.CommandSegments[0].Key)
	}
	if updated.EditorMetadata.CommentMarker != "#222222" {
		t.Errorf(
			"expected metadata comment marker override, got %q",
			updated.EditorMetadata.CommentMarker,
		)
	}
	if color, ok := updated.EditorMetadata.DirectiveColors["custom"]; !ok || color != "#333333" {
		t.Errorf("expected directive color #333333, got %q (present=%v)", color, ok)
	}
	if color := updated.ListItemTitle.GetForeground(); color != lipgloss.Color("#222233") {
		t.Errorf("expected list item title foreground #222233, got %v", color)
	}
	if color := updated.ListItemDescription.GetForeground(); color != lipgloss.Color("#9999aa") {
		t.Errorf("expected list item description foreground #9999aa, got %v", color)
	}
	if color := updated.ResponseContentRaw.GetForeground(); color != lipgloss.Color("#abcdef") {
		t.Errorf("expected raw response foreground #abcdef, got %v", color)
	}
	if color := updated.StreamContent.GetForeground(); color != lipgloss.Color("#123123") {
		t.Errorf("expected stream content foreground #123123, got %v", color)
	}
	if base.PaneActiveForeground == "#123456" {
		t.Errorf("base theme should remain unchanged")
	}
}

func TestApplySpecDirectiveDefaultOverridesExistingEntries(t *testing.T) {
	base := DefaultTheme()
	original := base.EditorMetadata.DirectiveColors
	if len(original) == 0 {
		t.Fatalf("default theme should define directive colors")
	}
	if original["name"] == "" {
		t.Fatalf("expected default directive color for name")
	}

	spec := ThemeSpec{
		EditorMetadata: &EditorMetadataSpec{
			DirectiveDefault: strPtr("#123456"),
			DirectiveColors: map[string]string{
				"tag": "#abcdef",
			},
		},
	}

	updated, err := ApplySpec(base, spec)
	if err != nil {
		t.Fatalf("ApplySpec returned error: %v", err)
	}

	if updated.EditorMetadata.DirectiveDefault != "#123456" {
		t.Fatalf(
			"expected directive default to update, got %q",
			updated.EditorMetadata.DirectiveDefault,
		)
	}
	if color := updated.EditorMetadata.DirectiveColors["name"]; color != "#123456" {
		t.Errorf("expected name directive to follow new default, got %q", color)
	}
	if color := updated.EditorMetadata.DirectiveColors["tag"]; color != "#abcdef" {
		t.Errorf("expected explicit tag directive override, got %q", color)
	}
	if base.EditorMetadata.DirectiveDefault == "#123456" {
		t.Errorf("base theme should remain unchanged")
	}
	if color := base.EditorMetadata.DirectiveColors["name"]; color != original["name"] {
		t.Errorf("base directive colors should remain unchanged")
	}
}

func TestApplySpecRTSKeywordOverrides(t *testing.T) {
	base := DefaultTheme()
	spec := ThemeSpec{
		EditorMetadata: &EditorMetadataSpec{
			RTSKeywordDefault: strPtr("#101010"),
			RTSKeywordDecl:    strPtr("#111111"),
			RTSKeywordControl: strPtr("#222222"),
			RTSKeywordLiteral: strPtr("#333333"),
			RTSKeywordLogical: strPtr("#444444"),
		},
	}

	updated, err := ApplySpec(base, spec)
	if err != nil {
		t.Fatalf("ApplySpec returned error: %v", err)
	}

	if got := updated.EditorMetadata.RTSKeywordDefault; got != "#101010" {
		t.Errorf("expected RTS keyword default %q, got %q", "#101010", got)
	}
	if got := updated.EditorMetadata.RTSKeywordDecl; got != "#111111" {
		t.Errorf("expected RTS keyword decl %q, got %q", "#111111", got)
	}
	if got := updated.EditorMetadata.RTSKeywordControl; got != "#222222" {
		t.Errorf("expected RTS keyword control %q, got %q", "#222222", got)
	}
	if got := updated.EditorMetadata.RTSKeywordLiteral; got != "#333333" {
		t.Errorf("expected RTS keyword literal %q, got %q", "#333333", got)
	}
	if got := updated.EditorMetadata.RTSKeywordLogical; got != "#444444" {
		t.Errorf("expected RTS keyword logical %q, got %q", "#444444", got)
	}
	if base.EditorMetadata.RTSKeywordDefault == "#101010" {
		t.Errorf("base theme should remain unchanged")
	}
}
