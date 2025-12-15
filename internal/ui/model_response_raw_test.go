package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
)

func TestCycleRawViewSkipsTextForBinary(t *testing.T) {
	body := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0x0d, 0x0a, 0x1a}
	meta := binaryview.Analyze(body, "application/octet-stream")
	rawHex := binaryview.HexDump(body, 16)
	rawBase64 := binaryview.Base64Lines(body, 76)

	snap := &responseSnapshot{
		raw:         rawHex,
		rawHex:      rawHex,
		rawBase64:   rawBase64,
		rawText:     formatRawBody(body, "application/octet-stream"),
		rawMode:     rawViewHex,
		body:        body,
		bodyMeta:    meta,
		contentType: "application/octet-stream",
		ready:       true,
	}

	model := newModelWithResponseTab(responseTabRaw, snap)
	pane := model.pane(responsePanePrimary)
	snapshot := pane.snapshot

	model.cycleRawViewMode()
	if snapshot.rawMode != rawViewBase64 {
		t.Fatalf("expected base64 mode after first cycle, got %v", snapshot.rawMode)
	}
	if snapshot.raw != snapshot.rawBase64 {
		t.Fatalf("expected raw content to switch to base64")
	}

	model.cycleRawViewMode()
	if snapshot.rawMode != rawViewHex {
		t.Fatalf("expected hex mode after second cycle, got %v", snapshot.rawMode)
	}
	if snapshot.raw != snapshot.rawHex {
		t.Fatalf("expected raw content to switch back to hex")
	}
}

func TestApplyRawViewModeClampsBinaryText(t *testing.T) {
	body := []byte{0x00, 0x01, 0x02, 0x03}
	snap := &responseSnapshot{
		rawText:     formatRawBody(body, "application/octet-stream"),
		rawHex:      binaryview.HexDump(body, 16),
		rawBase64:   binaryview.Base64Lines(body, 76),
		rawMode:     rawViewText,
		body:        body,
		contentType: "application/octet-stream",
		ready:       true,
	}

	applyRawViewMode(snap, rawViewText)
	if snap.rawMode != rawViewHex {
		t.Fatalf("expected text mode to clamp to hex for binary payloads, got %v", snap.rawMode)
	}
	if snap.raw != snap.rawHex {
		t.Fatalf("expected raw content to use hex view when text is unsafe")
	}
}

func TestApplyRawViewModeKeepsSummary(t *testing.T) {
	body := []byte("hello")
	summary := "Status: 200 OK"
	snap := &responseSnapshot{
		rawSummary:  summary,
		rawText:     formatRawBody(body, "text/plain"),
		rawHex:      binaryview.HexDump(body, 16),
		rawBase64:   binaryview.Base64Lines(body, 76),
		rawMode:     rawViewText,
		body:        body,
		contentType: "text/plain",
		ready:       true,
	}

	applyRawViewMode(snap, rawViewHex)
	if !strings.Contains(snap.raw, summary) {
		t.Fatalf("expected raw view to retain summary")
	}
	if !strings.Contains(snap.raw, snap.rawHex) {
		t.Fatalf("expected raw view to include hex body")
	}

	applyRawViewMode(snap, rawViewBase64)
	if snap.rawMode != rawViewBase64 {
		t.Fatalf("expected base64 mode, got %v", snap.rawMode)
	}
	if !strings.Contains(snap.raw, summary) || !strings.Contains(snap.raw, snap.rawBase64) {
		t.Fatalf("expected raw view to retain summary and base64 body")
	}
}

func TestHeavyHexGeneratedOnDemand(t *testing.T) {
	body := bytes.Repeat([]byte("A"), rawHeavyLimit+1)
	meta := binaryview.Analyze(body, "text/plain")
	bv := buildBodyViews(body, "text/plain", &meta, nil, "")
	rawDefault := bv.raw
	rawText := bv.rawText
	rawHex := bv.rawHex
	rawMode := bv.mode

	if rawHex != "" {
		t.Fatalf("expected heavy hex to be deferred")
	}
	if rawMode != rawViewText {
		t.Fatalf("expected raw mode to default to text for large printable payload")
	}

	snap := &responseSnapshot{
		rawSummary:  "Status: 200 OK",
		raw:         joinSections("Status: 200 OK", rawDefault),
		rawText:     rawText,
		rawHex:      rawHex,
		rawMode:     rawMode,
		body:        body,
		bodyMeta:    meta,
		contentType: "text/plain",
		ready:       true,
	}

	applyRawViewMode(snap, rawViewText)
	if snap.rawMode != rawViewText {
		t.Fatalf("expected to remain in text mode")
	}

	applyRawViewMode(snap, rawViewHex)
	if snap.rawHex == "" {
		t.Fatalf("expected hex dump to be generated on demand")
	}
	if !strings.Contains(snap.raw, snap.rawSummary) {
		t.Fatalf("expected summary to persist in hex view")
	}
}
