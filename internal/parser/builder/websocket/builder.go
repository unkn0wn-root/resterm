package websocket

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type Builder struct {
	on    bool
	opts  restfile.WebSocketOptions
	steps []restfile.WebSocketStep
}

const (
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
	wsCloseStandard  = 1014
	wsCloseAppMin    = 3000
	wsCloseAppMax    = 4999
)

func New() *Builder {
	return &Builder{}
}

func (b *Builder) HandleDirective(name directive.Name, rest string) (handled, reset bool, err error) {
	switch name {
	case directive.WebSocket:
		reset, err := b.handleWebSocket(rest)
		return true, reset, err
	case directive.WS:
		return true, false, b.handleStep(rest)
	default:
		return false, false, nil
	}
}

func (b *Builder) handleWebSocket(rest string) (reset bool, err error) {
	t := str.Trim(rest)
	if t == "" {
		b.on = true
		return false, nil
	}
	if directive.IsOff(t) {
		b.reset()
		return true, nil
	}

	b.on = true
	return false, directive.ApplyOptions(directive.WebSocket, t, wsAliases, b.applyOption)
}

var wsAliases = [][]string{
	{wsOptIdle, wsOptIdleAlt},
	{wsOptSub, wsOptSubs},
}

func (b *Builder) reset() {
	b.on = false
	b.opts = restfile.WebSocketOptions{}
	b.steps = nil
}

// ParseOptions lowercases keys, so name is already normalized.
func (b *Builder) applyOption(name, value string) error {
	switch name {
	case wsOptTimeout:
		dur, err := wsDuration(name, value)
		if err != nil {
			return err
		}
		b.opts.HandshakeTimeout = dur
	case wsOptIdle, wsOptIdleAlt:
		dur, err := wsDuration(name, value)
		if err != nil {
			return err
		}
		b.opts.IdleTimeout = dur
	case wsOptMaxMsg:
		size, err := bytesize.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid @websocket %s %q: %w", name, value, err)
		}
		b.opts.MaxMessageBytes = size
	case wsOptSub, wsOptSubs:
		list := directive.SplitCSV(value)
		if len(list) == 0 {
			return fmt.Errorf(
				"invalid @websocket %s %q: expected at least one subprotocol",
				name,
				value,
			)
		}
		b.opts.Subprotocols = list
	case wsOptCompression:
		val, ok := directive.ParseBool(value)
		if !ok {
			return fmt.Errorf(
				"invalid @websocket %s %q: expected true or false",
				name,
				value,
			)
		}
		b.opts.Compression = restfile.OptOf(val)
	default:
		return directive.UnknownOption(directive.WebSocket, name)
	}
	return nil
}

func wsDuration(name, value string) (time.Duration, error) {
	dur, ok := duration.Parse(value)
	if !ok || dur < 0 {
		return 0, fmt.Errorf(
			"invalid @websocket %s %q: expected a non-negative duration",
			name,
			value,
		)
	}
	return dur, nil
}

type wsStepParser func(rest string, step *restfile.WebSocketStep) error

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

func (b *Builder) handleStep(rest string) error {
	t := str.Trim(rest)
	if t == "" {
		return errors.New("@ws requires an action")
	}
	b.on = true

	act, rem := directive.CutToken(t)
	if act == "" {
		return errors.New("@ws requires an action")
	}
	act = str.LowerTrim(act)
	rem = str.Trim(rem)

	parse, ok := wsStepParsers[act]
	if !ok {
		return fmt.Errorf("unknown @ws action %q", act)
	}
	step := restfile.WebSocketStep{}
	if err := parse(rem, &step); err != nil {
		return err
	}

	b.steps = append(b.steps, step)
	return nil
}

func parseWSSendText(rest string, step *restfile.WebSocketStep) error {
	step.Type = restfile.WebSocketStepSendText
	step.Value = rest
	return nil
}

func parseWSSendJSON(rest string, step *restfile.WebSocketStep) error {
	step.Type = restfile.WebSocketStepSendJSON
	step.Value = rest
	return nil
}

func parseWSSendBase64(rest string, step *restfile.WebSocketStep) error {
	step.Type = restfile.WebSocketStepSendBase64
	step.Value = rest
	return nil
}

func parseWSSendFile(rest string, step *restfile.WebSocketStep) error {
	step.Type = restfile.WebSocketStepSendFile
	if after, ok := strings.CutPrefix(rest, "<"); ok {
		rest = str.Trim(after)
	}
	if rest == "" {
		return errors.New("@ws send-file requires a path")
	}
	step.File = rest
	return nil
}

func parseWSPing(rest string, step *restfile.WebSocketStep) error {
	step.Type = restfile.WebSocketStepPing
	step.Value = rest
	return nil
}

func parseWSPong(rest string, step *restfile.WebSocketStep) error {
	step.Type = restfile.WebSocketStepPong
	step.Value = rest
	return nil
}

func parseWSWait(rest string, step *restfile.WebSocketStep) error {
	if rest == "" {
		return errors.New("@ws wait requires a duration")
	}
	step.Type = restfile.WebSocketStepWait
	dur, ok := duration.Parse(rest)
	if !ok || dur < 0 {
		return fmt.Errorf(
			"invalid @ws wait duration %q: expected a non-negative duration",
			rest,
		)
	}
	step.Duration = dur
	return nil
}

func parseWSClose(rest string, step *restfile.WebSocketStep) error {
	step.Type = restfile.WebSocketStepClose
	if rest == "" {
		step.Code = wsCloseOK
		return nil
	}
	codeTok, tail := directive.CutToken(rest)
	if codeTok == "" {
		step.Code = wsCloseOK
		return nil
	}
	if code, err := strconv.Atoi(codeTok); err == nil {
		if !validWSCloseCode(code) {
			return fmt.Errorf(
				"invalid @ws close code %q: expected an allowed WebSocket close code",
				codeTok,
			)
		}
		step.Code = code
		step.Reason = str.Trim(tail)
		return nil
	}
	step.Code = wsCloseOK
	step.Reason = str.Trim(rest)
	return nil
}

func validWSCloseCode(code int) bool {
	switch code {
	case 1004, 1005, 1006, 1015:
		return false
	}
	return code >= wsCloseOK && code <= wsCloseStandard ||
		code >= wsCloseAppMin && code <= wsCloseAppMax
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
