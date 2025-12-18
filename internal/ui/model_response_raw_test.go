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
	rawBase64 := binaryview.Base64Lines(body, rawBase64LineWidth)

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
		rawBase64:   binaryview.Base64Lines(body, rawBase64LineWidth),
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

func TestApplyRawViewModeDefersHeavyBinary(t *testing.T) {
	body := bytes.Repeat([]byte{0x00}, rawHeavyLimit+1)
	meta := binaryview.Analyze(body, "application/octet-stream")
	snap := &responseSnapshot{
		rawSummary:  "Status: 200 OK",
		rawMode:     rawViewText,
		body:        body,
		bodyMeta:    meta,
		contentType: "application/octet-stream",
		ready:       true,
	}

	applyRawViewMode(snap, rawViewText)
	if snap.rawMode != rawViewSummary {
		t.Fatalf("expected heavy binary to clamp to summary mode, got %v", snap.rawMode)
	}
	if snap.rawHex != "" || snap.rawBase64 != "" {
		t.Fatalf("expected heavy binary dumps to be deferred")
	}
	if !strings.Contains(snap.raw, "<raw dump deferred>") {
		t.Fatalf("expected raw summary placeholder, got %q", snap.raw)
	}

	applyRawViewMode(snap, rawViewHex)
	if snap.rawHex == "" {
		t.Fatalf("expected heavy hex dump to be generated on demand")
	}
}

func TestCycleRawViewModeTriggersAsyncHexLoad(t *testing.T) {
	body := bytes.Repeat([]byte{0x00}, rawHeavyLimit+1)
	meta := binaryview.Analyze(body, "application/octet-stream")
	snap := &responseSnapshot{
		rawSummary:  "Status: 200 OK",
		rawMode:     rawViewSummary,
		body:        body,
		bodyMeta:    meta,
		contentType: "application/octet-stream",
		ready:       true,
	}
	model := newModelWithResponseTab(responseTabRaw, snap)

	cmd := model.cycleRawViewMode()
	pane := model.pane(responsePanePrimary)
	if pane == nil || pane.snapshot == nil {
		t.Fatalf("expected response pane with snapshot")
	}
	snap = pane.snapshot
	if cmd == nil {
		t.Fatalf("expected async load command for heavy hex view")
	}
	if !snap.rawLoading || snap.rawLoadingMode != rawViewHex {
		t.Fatalf("expected raw hex load to start, got loading=%v mode=%v", snap.rawLoading, snap.rawLoadingMode)
	}
	if snap.rawMode != rawViewHex {
		t.Fatalf("expected raw mode to switch to hex while loading, got %v", snap.rawMode)
	}
	if !strings.Contains(snap.raw, "Loading raw dump (hex)") {
		t.Fatalf("expected loading placeholder, got %q", snap.raw)
	}
}

func TestApplyRawViewModeKeepsSummary(t *testing.T) {
	body := []byte("hello")
	summary := "Status: 200 OK"
	snap := &responseSnapshot{
		rawSummary:  summary,
		rawText:     formatRawBody(body, "text/plain"),
		rawHex:      binaryview.HexDump(body, 16),
		rawBase64:   binaryview.Base64Lines(body, rawBase64LineWidth),
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

func TestShowRawDumpUsesCachedHex(t *testing.T) {
	body := bytes.Repeat([]byte{0x41}, rawHeavyLimit+1)
	meta := binaryview.Analyze(body, "application/octet-stream")
	hex := binaryview.HexDump(body[:32], 16)
	snap := &responseSnapshot{
		rawSummary:  "Status: 200 OK",
		rawHex:      hex,
		rawMode:     rawViewSummary,
		body:        body,
		bodyMeta:    meta,
		contentType: "application/octet-stream",
		ready:       true,
	}

	model := newModelWithResponseTab(responseTabRaw, snap)
	model.showRawDump()
	pane := model.pane(responsePanePrimary)
	if pane == nil || pane.snapshot == nil {
		t.Fatalf("expected response pane with snapshot")
	}
	snap = pane.snapshot
	if snap.rawLoading {
		t.Fatalf("expected cached hex to skip async load")
	}
	if snap.rawMode != rawViewHex {
		t.Fatalf("expected raw mode to switch to hex, got %v", snap.rawMode)
	}
	if !strings.Contains(snap.raw, hex) {
		t.Fatalf("expected cached hex in raw output, got %q", snap.raw)
	}
}

func TestApplyRawViewModeNoDuplicateSummaryOnEmpty(t *testing.T) {
	summary := "Status: 204 No Content"
	snap := &responseSnapshot{
		rawSummary: summary,
		raw:        joinSections(summary, "<empty>"),
		rawMode:    rawViewText,
		ready:      true,
	}

	applyRawViewMode(snap, rawViewText)

	if strings.Count(snap.raw, summary) != 1 {
		t.Fatalf("expected summary to appear once, got %q", snap.raw)
	}
	if !strings.Contains(snap.raw, "<empty>") {
		t.Fatalf("expected empty placeholder to remain, got %q", snap.raw)
	}
}

func TestApplyRawViewModePreservesPrebuiltRawWithoutBody(t *testing.T) {
	t.Run("no summary", func(t *testing.T) {
		raw := "Request Error\nDetails: failed to connect"
		snap := &responseSnapshot{
			raw:     raw,
			rawMode: rawViewText,
			ready:   true,
		}

		applyRawViewMode(snap, rawViewText)

		if snap.raw != raw {
			t.Fatalf("expected raw content to remain unchanged, got %q", snap.raw)
		}
		if snap.rawText != raw {
			t.Fatalf("expected rawText to capture existing content, got %q", snap.rawText)
		}
	})

	t.Run("with summary", func(t *testing.T) {
		summary := "Request Error"
		detail := "Details: failed to connect"
		raw := joinSections(summary, detail)
		snap := &responseSnapshot{
			rawSummary: summary,
			raw:        raw,
			rawMode:    rawViewText,
			ready:      true,
		}

		applyRawViewMode(snap, rawViewText)

		if strings.Count(snap.raw, summary) != 1 {
			t.Fatalf("expected summary to remain once, got %q", snap.raw)
		}
		if !strings.Contains(snap.raw, detail) {
			t.Fatalf("expected detail content to remain, got %q", snap.raw)
		}
		if snap.rawText != detail {
			t.Fatalf("expected rawText to capture body content, got %q", snap.rawText)
		}
	})
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
