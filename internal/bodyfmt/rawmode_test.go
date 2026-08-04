package bodyfmt

import (
	"reflect"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
)

func TestPayloadModes(t *testing.T) {
	tests := []struct {
		name string
		pl   Payload
		want []RawMode
	}{
		{
			name: "text",
			pl:   Payload{Meta: binaryview.Meta{Kind: binaryview.KindText}, Size: 16},
			want: []RawMode{RawText, RawHex, RawBase64},
		},
		{
			name: "printable binary stays readable",
			pl: Payload{
				Meta: binaryview.Meta{Kind: binaryview.KindBinary, Printable: true},
				Size: 16,
			},
			want: []RawMode{RawText, RawHex, RawBase64},
		},
		{
			name: "small binary",
			pl:   Payload{Meta: binaryview.Meta{Kind: binaryview.KindBinary}, Size: 16},
			want: []RawMode{RawHex, RawBase64},
		},
		{
			name: "heavy binary offers summary",
			pl: Payload{
				Meta: binaryview.Meta{Kind: binaryview.KindBinary},
				Size: RawHeavyLimit + 1,
			},
			want: []RawMode{RawSummary, RawHex, RawBase64},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pl.Modes(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Modes()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestPayloadSizeFallsBackToMeta(t *testing.T) {
	pl := Payload{Meta: binaryview.Meta{Kind: binaryview.KindBinary, Size: RawHeavyLimit + 1}}
	if got := pl.Modes(); got[0] != RawSummary {
		t.Fatalf("expected analyzed size to mark payload heavy, got %v", got)
	}
}

func TestPayloadClampMode(t *testing.T) {
	pl := Payload{Meta: binaryview.Meta{Kind: binaryview.KindBinary}, Size: 16}
	if got := pl.ClampMode(RawBase64); got != RawBase64 {
		t.Fatalf("ClampMode(RawBase64)=%v, want %v", got, RawBase64)
	}
	if got := pl.ClampMode(RawText); got != RawHex {
		t.Fatalf("ClampMode(RawText)=%v, want %v", got, RawHex)
	}
}

func TestPayloadModeLabels(t *testing.T) {
	pl := Payload{Meta: binaryview.Meta{Kind: binaryview.KindBinary}, Size: 16}
	want := []string{"hex", "base64"}
	if got := pl.ModeLabels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ModeLabels()=%v, want %v", got, want)
	}
}

func TestPayloadDefaultModeWithoutHexDump(t *testing.T) {
	tests := []struct {
		name string
		pl   Payload
		want RawMode
	}{
		{
			name: "text falls back to text",
			pl:   Payload{Meta: binaryview.Meta{Kind: binaryview.KindText}, Size: RawHeavyLimit + 1},
			want: RawText,
		},
		{
			name: "heavy binary falls back to summary",
			pl: Payload{
				Meta: binaryview.Meta{Kind: binaryview.KindBinary},
				Size: RawHeavyLimit + 1,
			},
			want: RawSummary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pl.defaultMode(false); got != tt.want {
				t.Fatalf("defaultMode(false)=%v, want %v", got, tt.want)
			}
		})
	}
}
