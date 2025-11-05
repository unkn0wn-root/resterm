package parser

import (
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type wsBuilder struct {
	enabled bool
	options restfile.WebSocketOptions
	steps   []restfile.WebSocketStep
}

// newWebSocketBuilder returns a disabled builder with empty options.
func newWebSocketBuilder() *wsBuilder {
	return &wsBuilder{}
}

// HandleDirective routes websocket directives to either session options or steps.
func (b *wsBuilder) HandleDirective(key, rest string) bool {
	switch strings.ToLower(key) {
	case "websocket":
		return b.handleWebSocket(rest)
	case "ws":
		return b.handleStep(rest)
	default:
		return false
	}
}

// handleWebSocket toggles websocket mode and parses option assignments.
func (b *wsBuilder) handleWebSocket(rest string) bool {
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		b.enabled = true
		return true
	}

	lowered := strings.ToLower(trimmed)
	switch lowered {
	case "0", "false", "off", "disable", "disabled":
		b.enabled = false
		b.options = restfile.WebSocketOptions{}
		b.steps = nil
		return true
	}

	b.enabled = true
	assignments := parseOptionTokens(trimmed)
	for key, value := range assignments {
		b.applyOption(key, value)
	}
	return true
}

// applyOption updates individual websocket option fields.
func (b *wsBuilder) applyOption(name, value string) {
	switch strings.ToLower(name) {
	case "timeout":
		if dur, err := time.ParseDuration(value); err == nil && dur >= 0 {
			b.options.HandshakeTimeout = dur
		}
	case "receive", "receive-timeout":
		if dur, err := time.ParseDuration(value); err == nil && dur >= 0 {
			b.options.ReceiveTimeout = dur
		}
	case "max-message-bytes":
		if size, err := parseByteSize(value); err == nil {
			b.options.MaxMessageBytes = size
		}
	case "subprotocol", "subprotocols":
		cleaned := strings.TrimSpace(value)
		if cleaned == "" {
			return
		}
		parts := strings.Split(cleaned, ",")
		protocols := make([]string, 0, len(parts))
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if p != "" {
				protocols = append(protocols, p)
			}
		}
		if len(protocols) > 0 {
			b.options.Subprotocols = protocols
		}
	case "compression":
		if val, err := strconv.ParseBool(value); err == nil {
			b.options.Compression = val
			b.options.CompressionSet = true
		}
	}
}

// handleStep adds websocket scripted steps such as send, wait, or close.
func (b *wsBuilder) handleStep(rest string) bool {
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return true
	}
	b.enabled = true

	action, remainder := splitFirst(trimmed)
	if action == "" {
		return true
	}

	step := restfile.WebSocketStep{}
	switch strings.ToLower(action) {
	case "send":
		step.Type = restfile.WebSocketStepSendText
		step.Value = strings.TrimSpace(remainder)
	case "send-json":
		step.Type = restfile.WebSocketStepSendJSON
		step.Value = strings.TrimSpace(remainder)
	case "send-base64":
		step.Type = restfile.WebSocketStepSendBase64
		step.Value = strings.TrimSpace(remainder)
	case "send-file":
		step.Type = restfile.WebSocketStepSendFile
		path := strings.TrimSpace(remainder)
		if strings.HasPrefix(path, "<") {
			path = strings.TrimSpace(strings.TrimPrefix(path, "<"))
		}
		if path == "" {
			return true
		}
		step.File = path
	case "ping":
		step.Type = restfile.WebSocketStepPing
		step.Value = strings.TrimSpace(remainder)
	case "pong":
		step.Type = restfile.WebSocketStepPong
		step.Value = strings.TrimSpace(remainder)
	case "wait":
		step.Type = restfile.WebSocketStepWait
		durationText := strings.TrimSpace(remainder)
		if dur, err := time.ParseDuration(durationText); err == nil && dur >= 0 {
			step.Duration = dur
		} else {
			return true
		}
	case "close":
		step.Type = restfile.WebSocketStepClose
		remainder = strings.TrimSpace(remainder)
		if remainder == "" {
			step.Code = 1000
			break
		}
		codeToken, tail := splitFirst(remainder)
		if codeToken == "" {
			step.Code = 1000
			break
		}
		if code, err := strconv.Atoi(codeToken); err == nil {
			step.Code = code
			step.Reason = strings.TrimSpace(tail)
		} else {
			step.Code = 1000
			step.Reason = strings.TrimSpace(remainder)
		}
	default:
		return false
	}

	b.steps = append(b.steps, step)
	return true
}

// Finalize returns the websocket request specification if enabled.
func (b *wsBuilder) Finalize() (*restfile.WebSocketRequest, bool) {
	if !b.enabled {
		return nil, false
	}
	steps := append([]restfile.WebSocketStep(nil), b.steps...)
	req := &restfile.WebSocketRequest{
		Options: b.options,
		Steps:   steps,
	}
	return req, true
}
