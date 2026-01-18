package parser

import (
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type wsBuilder struct {
	on    bool
	opts  restfile.WebSocketOptions
	steps []restfile.WebSocketStep
}

const (
	wsKeyWebSocket   = "websocket"
	wsKeyWS          = "ws"
	wsOptTimeout     = "timeout"
	wsOptIdle        = "idle"
	wsOptIdleAlt     = "idle-timeout"
	wsOptMaxMsg      = "max-message-bytes"
	wsOptSub         = "subprotocol"
	wsOptSubs        = "subprotocols"
	wsOptCompression = "compression"
	wsActSend        = "send"
	wsActSendJSON    = "send-json"
	wsActSendBase64  = "send-base64"
	wsActSendFile    = "send-file"
	wsActPing        = "ping"
	wsActPong        = "pong"
	wsActWait        = "wait"
	wsActClose       = "close"
	wsCloseOK        = 1000
)

func newWebSocketBuilder() *wsBuilder {
	return &wsBuilder{}
}

func (b *wsBuilder) HandleDirective(key, rest string) bool {
	switch normKey(key) {
	case wsKeyWebSocket:
		return b.handleWebSocket(rest)
	case wsKeyWS:
		return b.handleStep(rest)
	default:
		return false
	}
}

func (b *wsBuilder) handleWebSocket(rest string) bool {
	t := trim(rest)
	if t == "" {
		b.on = true
		return true
	}
	if isOffToken(t) {
		b.reset()
		return true
	}

	b.on = true
	opts := parseOptionTokens(t)
	for key, value := range opts {
		b.applyOption(key, value)
	}
	return true
}

func (b *wsBuilder) reset() {
	b.on = false
	b.opts = restfile.WebSocketOptions{}
	b.steps = nil
}

func (b *wsBuilder) applyOption(name, value string) {
	switch normKey(name) {
	case wsOptTimeout:
		if dur, err := time.ParseDuration(value); err == nil && dur >= 0 {
			b.opts.HandshakeTimeout = dur
		}
	case wsOptIdle, wsOptIdleAlt:
		if dur, err := time.ParseDuration(value); err == nil && dur >= 0 {
			b.opts.IdleTimeout = dur
		}
	case wsOptMaxMsg:
		if size, err := parseByteSize(value); err == nil {
			b.opts.MaxMessageBytes = size
		}
	case wsOptSub, wsOptSubs:
		if list := splitCSV(value); len(list) > 0 {
			b.opts.Subprotocols = list
		}
	case wsOptCompression:
		if val, err := strconv.ParseBool(value); err == nil {
			b.opts.Compression = val
			b.opts.CompressionSet = true
		}
	}
}

type wsStepParser func(rest string, step *restfile.WebSocketStep) bool

var wsStepParsers = map[string]wsStepParser{
	wsActSend:       parseWSSendText,
	wsActSendJSON:   parseWSSendJSON,
	wsActSendBase64: parseWSSendBase64,
	wsActSendFile:   parseWSSendFile,
	wsActPing:       parseWSPing,
	wsActPong:       parseWSPong,
	wsActWait:       parseWSWait,
	wsActClose:      parseWSClose,
}

func (b *wsBuilder) handleStep(rest string) bool {
	t := trim(rest)
	if t == "" {
		return true
	}
	b.on = true

	act, rem := splitFirst(t)
	if act == "" {
		return true
	}
	act = strings.ToLower(act)
	rem = trim(rem)

	parse, ok := wsStepParsers[act]
	if !ok {
		return false
	}
	step := restfile.WebSocketStep{}
	if !parse(rem, &step) {
		return true
	}

	b.steps = append(b.steps, step)
	return true
}

func parseWSSendText(rest string, step *restfile.WebSocketStep) bool {
	step.Type = restfile.WebSocketStepSendText
	step.Value = rest
	return true
}

func parseWSSendJSON(rest string, step *restfile.WebSocketStep) bool {
	step.Type = restfile.WebSocketStepSendJSON
	step.Value = rest
	return true
}

func parseWSSendBase64(rest string, step *restfile.WebSocketStep) bool {
	step.Type = restfile.WebSocketStepSendBase64
	step.Value = rest
	return true
}

func parseWSSendFile(rest string, step *restfile.WebSocketStep) bool {
	step.Type = restfile.WebSocketStepSendFile
	if strings.HasPrefix(rest, "<") {
		rest = trim(strings.TrimPrefix(rest, "<"))
	}
	if rest == "" {
		return false
	}
	step.File = rest
	return true
}

func parseWSPing(rest string, step *restfile.WebSocketStep) bool {
	step.Type = restfile.WebSocketStepPing
	step.Value = rest
	return true
}

func parseWSPong(rest string, step *restfile.WebSocketStep) bool {
	step.Type = restfile.WebSocketStepPong
	step.Value = rest
	return true
}

func parseWSWait(rest string, step *restfile.WebSocketStep) bool {
	step.Type = restfile.WebSocketStepWait
	dur, err := time.ParseDuration(rest)
	if err != nil || dur < 0 {
		return false
	}
	step.Duration = dur
	return true
}

func parseWSClose(rest string, step *restfile.WebSocketStep) bool {
	step.Type = restfile.WebSocketStepClose
	if rest == "" {
		step.Code = wsCloseOK
		return true
	}
	codeTok, tail := splitFirst(rest)
	if codeTok == "" {
		step.Code = wsCloseOK
		return true
	}
	if code, err := strconv.Atoi(codeTok); err == nil {
		step.Code = code
		step.Reason = trim(tail)
		return true
	}
	step.Code = wsCloseOK
	step.Reason = trim(rest)
	return true
}

func (b *wsBuilder) Finalize() (*restfile.WebSocketRequest, bool) {
	if !b.on {
		return nil, false
	}
	steps := cloneSlice(b.steps)
	req := &restfile.WebSocketRequest{
		Options: b.opts,
		Steps:   steps,
	}
	return req, true
}
