package rtspre

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rtssrc"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
)

// ExecInput describes the RTS script blocks and host runtime builder to execute.
type ExecInput struct {
	Doc     *restfile.Document
	Scripts []restfile.ScriptBlock
	BaseDir string
	BuildRT func() rts.RT
}

// Run executes RTS pre-request blocks and annotates errors with script source.
func Run(ctx context.Context, eng *rts.Eng, in ExecInput) error {
	if ctx == nil {
		ctx = context.Background()
	}
	buildRT := in.BuildRT
	if buildRT == nil {
		buildRT = func() rts.RT { return rts.RT{} }
	}
	for idx, block := range in.Scripts {
		if !restfile.IsPreRequestScript(block, restfile.ScriptLangRTS) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		src, err := rtssrc.Load(in.Doc, block, in.BaseDir)
		if err != nil {
			return fmt.Errorf("rts pre-request script %d: %w", idx+1, err)
		}
		if strings.TrimSpace(src.Text) == "" {
			continue
		}
		if _, err := eng.ExecModule(ctx, buildRT(), src.Text, src.Pos); err != nil {
			return rtssrc.Annotate(err, src)
		}
	}
	return nil
}

// RuntimeGlobals returns globals keyed the way RTS variable lookup expects.
func RuntimeGlobals(globals map[string]prerequest.GlobalValue, safe bool) map[string]string {
	if len(globals) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(globals))
	for key, value := range globals {
		if value.Delete || safe && value.Secret {
			continue
		}
		name := strings.TrimSpace(value.Name)
		if name == "" {
			name = strings.TrimSpace(key)
		}
		if name == "" {
			continue
		}
		out[strings.ToLower(name)] = value.Value
	}
	return out
}

// Mutator adapts RTS mutation interfaces to prerequest.Output.
type Mutator struct {
	out     *prerequest.Output
	req     *rts.Req
	vars    map[string]string
	globals map[string]string
}

// NewMutator creates the request, variable, and global mutation surface for RTS.
func NewMutator(
	out *prerequest.Output,
	req *rts.Req,
	vars map[string]string,
	globals map[string]string,
) *Mutator {
	return &Mutator{out: out, req: req, vars: vars, globals: globals}
}

// Request returns the live request view mutated during script execution.
func (m *Mutator) Request() *rts.Req {
	if m == nil {
		return nil
	}
	return m.req
}

func (m *Mutator) SetMethod(value string) {
	if m == nil || m.out == nil {
		return
	}
	val := strings.ToUpper(strings.TrimSpace(value))
	setPtr(&m.out.Method, val)
	if m.req != nil {
		m.req.Method = val
	}
}

func (m *Mutator) SetURL(value string) {
	if m == nil || m.out == nil {
		return
	}
	val := strings.TrimSpace(value)
	setPtr(&m.out.URL, val)
	if m.req != nil {
		m.req.URL = val
		m.req.Q = nil
	}
}

func (m *Mutator) SetHeader(name, value string) {
	if m == nil || m.out == nil {
		return
	}
	if m.out.Headers == nil {
		m.out.Headers = make(http.Header)
	}
	m.out.Headers.Set(name, value)
	setReqHeader(m.req, name, value, false)
}

func (m *Mutator) AddHeader(name, value string) {
	if m == nil || m.out == nil {
		return
	}
	if m.out.Headers == nil {
		m.out.Headers = make(http.Header)
	}
	m.out.Headers.Add(name, value)
	setReqHeader(m.req, name, value, true)
}

func (m *Mutator) DelHeader(name string) {
	if m == nil || m.out == nil {
		return
	}
	if m.out.Headers != nil {
		m.out.Headers.Del(name)
	}
	if m.req != nil && m.req.H != nil {
		delete(m.req.H, lowerKey(name))
	}
}

func (m *Mutator) SetQuery(name, value string) {
	if m == nil || m.out == nil {
		return
	}
	if m.out.Query == nil {
		m.out.Query = make(map[string]string)
	}
	m.out.Query[name] = value
	setReqQuery(m.req, name, value)
}

func (m *Mutator) SetBody(value string) {
	if m == nil || m.out == nil {
		return
	}
	setPtr(&m.out.Body, value)
}

func (m *Mutator) SetVar(name, value string) {
	if m == nil || m.out == nil {
		return
	}
	if m.out.Variables == nil {
		m.out.Variables = make(map[string]string)
	}
	m.out.Variables[name] = value
	if m.vars != nil {
		m.vars[name] = value
	}
}

func (m *Mutator) SetGlobal(name, value string, secret bool) {
	if m == nil || m.out == nil {
		return
	}
	if m.out.Globals == nil {
		m.out.Globals = make(map[string]prerequest.GlobalValue)
	}
	m.out.Globals[name] = prerequest.GlobalValue{Name: name, Value: value, Secret: secret}
	if m.globals != nil {
		m.globals[lowerKey(name)] = value
	}
}

func (m *Mutator) DelGlobal(name string) {
	if m == nil || m.out == nil {
		return
	}
	if m.out.Globals == nil {
		m.out.Globals = make(map[string]prerequest.GlobalValue)
	}
	m.out.Globals[name] = prerequest.GlobalValue{Name: name, Delete: true}
	if m.globals != nil {
		delete(m.globals, lowerKey(name))
	}
}

func setPtr(dst **string, value string) {
	if dst == nil {
		return
	}
	val := value
	*dst = &val
}

func setReqHeader(req *rts.Req, name, value string, appendValue bool) {
	if req == nil {
		return
	}
	if req.H == nil {
		req.H = make(map[string][]string)
	}
	key := lowerKey(name)
	if appendValue {
		req.H[key] = append(req.H[key], value)
		return
	}
	req.H[key] = []string{value}
}

func setReqQuery(req *rts.Req, name, value string) {
	if req == nil {
		return
	}
	if req.Q == nil {
		req.Q = make(map[string][]string)
	}
	req.Q[name] = []string{value}
	raw := strings.TrimSpace(req.URL)
	if raw == "" {
		return
	}
	val := value
	updated, err := urltpl.PatchQuery(raw, map[string]*string{name: &val})
	if err != nil {
		return
	}
	req.URL = updated
}

func lowerKey(name string) string {
	return strings.ToLower(name)
}
