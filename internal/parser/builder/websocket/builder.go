package websocket

import (
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/lex"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	dvalue "github.com/unkn0wn-root/resterm/internal/parser/directive/value"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type Builder struct {
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

func New() *Builder {
	return &Builder{}
}

func (b *Builder) HandleDirective(key, rest string) bool {
	switch normKey(key) {
	case wsKeyWebSocket:
		return b.handleWebSocket(rest)
	case wsKeyWS:
		return b.handleStep(rest)
	default:
		return false
	}
}

func (b *Builder) handleWebSocket(rest string) bool {
	t := str.Trim(rest)
	if t == "" {
		b.on = true
		return true
	}
	if dvalue.IsOffToken(t) {
		b.reset()
		return true
	}

	b.on = true
	opts := options.Parse(t)
	for key, value := range opts {
		b.applyOption(key, value)
	}
	return true
}

func (b *Builder) reset() {
	b.on = false
	b.opts = restfile.WebSocketOptions{}
	b.steps = nil
}

func (b *Builder) applyOption(name, value string) {
	switch normKey(name) {
	case wsOptTimeout:
		if dur, ok := duration.Parse(value); ok && dur >= 0 {
			b.opts.HandshakeTimeout = dur
		}
	case wsOptIdle, wsOptIdleAlt:
		if dur, ok := duration.Parse(value); ok && dur >= 0 {
			b.opts.IdleTimeout = dur
		}
	case wsOptMaxMsg:
		if size, err := dvalue.ParseByteSize(value); err == nil {
			b.opts.MaxMessageBytes = size
		}
	case wsOptSub, wsOptSubs:
		if list := options.SplitCSV(value); len(list) > 0 {
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

func (b *Builder) handleStep(rest string) bool {
	t := str.Trim(rest)
	if t == "" {
		return true
	}
	b.on = true

	act, rem := lex.SplitFirst(t)
	if act == "" {
		return true
	}
	act = str.LowerTrim(act)
	rem = str.Trim(rem)

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
	if after, ok := strings.CutPrefix(rest, "<"); ok {
		rest = str.Trim(after)
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
	dur, ok := duration.Parse(rest)
	if !ok || dur < 0 {
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
	codeTok, tail := lex.SplitFirst(rest)
	if codeTok == "" {
		step.Code = wsCloseOK
		return true
	}
	if code, err := strconv.Atoi(codeTok); err == nil {
		step.Code = code
		step.Reason = str.Trim(tail)
		return true
	}
	step.Code = wsCloseOK
	step.Reason = str.Trim(rest)
	return true
}

func (b *Builder) Finalize() (*restfile.WebSocketRequest, bool) {
	if !b.on {
		return nil, false
	}
	steps := append([]restfile.WebSocketStep(nil), b.steps...)
	req := &restfile.WebSocketRequest{
		Options: b.opts,
		Steps:   steps,
	}
	return req, true
}

func normKey(s string) string {
	return str.LowerTrim(s)
}
