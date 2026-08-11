package scripts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/filelookup"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Runner struct {
	fs filelookup.FileSystem
}

func NewRunner(fs filelookup.FileSystem) *Runner {
	if fs == nil {
		fs = filelookup.OSFileSystem{}
	}
	return &Runner{fs: fs}
}

type TestInput struct {
	Response  *Response
	Variables map[string]string
	Globals   vars.Globals
	BaseDir   string
	Stream    *StreamInfo
	Trace     *TraceInput
	Secrets   *vars.Secrets
}

type TestResult struct {
	Name    string
	Message string
	Passed  bool
	Elapsed time.Duration
}

func (r *Runner) RunPreRequest(
	scripts []restfile.ScriptBlock,
	input prerequest.Input,
) (prerequest.Output, error) {
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	result := prerequest.Output{
		Headers: make(http.Header),
		Query:   make(map[string]string),
	}
	// Keep one API for the whole batch so each block sees earlier changes.
	api := newPreRequestAPI(&result, input)

	for idx, block := range scripts {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		if !restfile.IsPreRequestScript(block, restfile.ScriptLangJS) {
			continue
		}

		script, err := r.loadScript(block, input.BaseDir)
		if err != nil {
			return result, diag.WrapAsf(diag.ClassScript, err, "pre-request script %d", idx+1)
		}
		if script == "" {
			continue
		}

		if err := r.executePreRequestScript(ctx, script, api); err != nil {
			return result, diag.WrapAsf(diag.ClassScript, err, "pre-request script %d", idx+1)
		}
	}

	prerequest.Normalize(&result)

	return result, nil
}

func (r *Runner) RunTests(
	scripts []restfile.ScriptBlock,
	input TestInput,
) ([]TestResult, vars.Globals, error) {
	var aggregated []TestResult
	var changes vars.Globals

	for idx, block := range scripts {
		if kind := strings.ToLower(block.Kind); kind != "test" && kind != "tests" {
			continue
		}
		if restfile.NormalizeScriptLang(block.Lang) != restfile.ScriptLangJS {
			continue
		}

		script, err := r.loadScript(block, input.BaseDir)
		if err != nil {
			return aggregated, changes, diag.WrapAsf(diag.ClassScript, err, "test script %d", idx+1)
		}
		if script == "" {
			continue
		}

		results, globals, err := r.executeTestScript(script, input)
		if err != nil {
			return aggregated, changes, diag.WrapAsf(diag.ClassScript, err, "test script %d", idx+1)
		}

		aggregated = append(aggregated, results...)
		changes.Merge(globals)
	}

	return aggregated, changes, nil
}

func (r *Runner) executePreRequestScript(
	ctx context.Context,
	script string,
	api *preRequestAPI,
) error {
	vm := goja.New()
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if done := ctx.Done(); done != nil {
			go func() {
				<-done
				vm.Interrupt(ctx.Err())
			}()
		}
	}

	if err := bindCommon(vm); err != nil {
		return diag.WrapAs(diag.ClassScript, err, "bind console api")
	}

	if err := vm.Set("request", api.requestAPI()); err != nil {
		return diag.WrapAs(diag.ClassScript, err, "bind request api")
	}

	if err := vm.Set("vars", api.varsAPI()); err != nil {
		return diag.WrapAs(diag.ClassScript, err, "bind vars api")
	}

	_, err := vm.RunString(script)
	if err != nil {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			var interrupted *goja.InterruptedError
			if errors.As(err, &interrupted) && ctx.Err() != nil {
				return ctx.Err()
			}
		}
		return diag.WrapAs(diag.ClassScript, err, "execute pre-request script")
	}
	return nil
}

func (r *Runner) executeTestScript(
	script string,
	input TestInput,
) ([]TestResult, vars.Globals, error) {
	vm := goja.New()
	streamInfo := input.Stream.Clone()
	tester := newTestAPI(input, streamInfo)
	tester.vm = vm
	streamBinding := newStreamAPI(vm, streamInfo)

	if err := bindCommon(vm); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "bind console api")
	}

	if err := vm.Set("tests", tester.testsAPI()); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "bind tests api")
	}

	if err := vm.Set("client", tester.clientAPI()); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "bind client api")
	}

	if err := vm.Set("resterm", tester.clientAPI()); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "bind resterm alias")
	}

	if err := vm.Set("response", tester.responseAPI()); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "bind response api")
	}

	if err := vm.Set("vars", tester.varsAPI()); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "bind vars api")
	}

	if err := vm.Set("stream", streamBinding.object()); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "bind stream api")
	}

	if err := vm.Set("trace", tester.traceAPI()); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "bind trace api")
	}

	_, err := vm.RunString(script)
	if err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "execute test script")
	}

	if err := streamBinding.replay(); err != nil {
		return nil, vars.Globals{}, diag.WrapAs(diag.ClassScript, err, "execute stream callbacks")
	}
	return tester.results(), tester.globalChanges(), nil
}

func bindCommon(vm *goja.Runtime) error {
	console := map[string]func(goja.FunctionCall) goja.Value{
		"log":   func(call goja.FunctionCall) goja.Value { return goja.Undefined() },
		"warn":  func(call goja.FunctionCall) goja.Value { return goja.Undefined() },
		"error": func(call goja.FunctionCall) goja.Value { return goja.Undefined() },
	}
	return vm.Set("console", console)
}

func normalizeScript(body string) string {
	script := strings.TrimSpace(body)
	if script == "" {
		return script
	}

	if strings.HasPrefix(script, "{%") && strings.HasSuffix(script, "%}") {
		script = strings.TrimSpace(script[2 : len(script)-2])
	}

	return script
}

func (r *Runner) loadScript(block restfile.ScriptBlock, baseDir string) (string, error) {
	if block.FilePath == "" {
		return normalizeScript(block.Body), nil
	}

	path := block.FilePath
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}

	data, err := r.fs.ReadFile(path)
	if err != nil {
		return "", diag.WrapAsf(diag.ClassFilesystem, err, "read script file %s", path)
	}
	return normalizeScript(string(data)), nil
}

// jsVarsAPI builds the vars object shared by pre-request and test scripts.
// When non-nil, record receives writes that propagate beyond the script.
func jsVarsAPI(
	view map[string]string,
	record func(name, value string),
	global map[string]any,
) map[string]any {
	return map[string]any{
		"get": func(name string) string {
			return view[vars.NameKey(name)]
		},
		"has": func(name string) bool {
			_, ok := view[vars.NameKey(name)]
			return ok
		},
		// Blank writes must not appear in the script view when recorded output drops them.
		"set": func(name, value string) {
			key := vars.NameKey(name)
			if key == "" {
				return
			}
			view[key] = value
			if record != nil {
				record(name, value)
			}
		},
		"global": global,
	}
}

func jsVarsView(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for name, value := range src {
		out[vars.NameKey(name)] = value
	}
	return out
}

type preRequestAPI struct {
	request   requestView
	output    *prerequest.Output
	variables map[string]string
	globals   vars.Globals
	secrets   *vars.Secrets
}

// Scripts read from a copy that is updated after each mutation. Query parameters
// are excluded because they are merged into the URL after the scripts finish.
type requestView struct {
	method  string
	url     string
	headers http.Header
}

func newPreRequestAPI(output *prerequest.Output, input prerequest.Input) *preRequestAPI {
	return &preRequestAPI{
		request:   newRequestView(input.Request),
		output:    output,
		variables: jsVarsView(input.Variables),
		globals:   input.Globals.Clone(),
		secrets:   input.Secrets,
	}
}

func newRequestView(req *restfile.Request) requestView {
	if req == nil {
		return requestView{headers: make(http.Header)}
	}
	headers := req.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	return requestView{method: req.Method, url: req.URL, headers: headers}
}

func (api *preRequestAPI) requestAPI() map[string]any {
	return map[string]any{
		"getURL": func() string {
			return api.request.url
		},
		"getMethod": func() string {
			return api.request.method
		},
		"getHeader": func(name string) string {
			return api.request.headers.Get(name)
		},
		"setHeader": func(name, value string) {
			api.output.SetHeader(name, value)
			api.request.headers.Set(name, value)
		},
		"addHeader": func(name, value string) {
			api.output.AddHeader(name, value)
			api.request.headers.Add(name, value)
		},
		"removeHeader": func(name string) {
			api.output.DelHeader(name)
			api.request.headers.Del(name)
		},
		"setQueryParam": api.output.SetQuery,
		"setURL": func(url string) {
			val := strings.TrimSpace(url)
			api.output.URL = &val
			api.request.url = val
		},
		"setMethod": func(method string) {
			val := util.UpperTrim(method)
			api.output.Method = &val
			api.request.method = val
		},
		"setBody": func(body string) {
			copied := body
			api.output.Body = &copied
		},
	}
}

func (api *preRequestAPI) varsAPI() map[string]any {
	return jsVarsAPI(api.variables, api.recordVar, api.globalAPI())
}

func (api *preRequestAPI) recordVar(name, value string) {
	api.output.Variables.Set(name, value)
}

func (api *preRequestAPI) globalAPI() map[string]any {
	return map[string]any{
		"get": func(name string) string {
			entry, _ := api.globals.Get(name)
			return entry.Value
		},
		"set": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			name := call.Arguments[0].String()
			value := call.Arguments[1].String()
			secret := false
			if len(call.Arguments) >= 3 {
				secret = parseGlobalSecret(call.Arguments[2])
			}
			api.setGlobal(name, value, secret)
			return goja.Undefined()
		},
		"has": func(name string) bool {
			return api.globals.Has(name)
		},
		"delete": func(name string) {
			api.deleteGlobal(name)
		},
	}
}

func (api *preRequestAPI) setGlobal(name, value string, secret bool) {
	entry := vars.GlobalMutation{Name: name, Value: value, Secret: secret}
	if !api.output.Globals.Set(name, entry) {
		return
	}
	if secret {
		api.secrets.Add(value)
	}
	api.globals.Set(name, entry)
}

func (api *preRequestAPI) deleteGlobal(name string) {
	if api.output.Globals.Set(name, vars.GlobalMutation{Name: name, Delete: true}) {
		api.globals.Delete(name)
	}
}

func parseGlobalSecret(value goja.Value) bool {
	switch exported := value.Export().(type) {
	case bool:
		return exported
	case map[string]any:
		if secret, ok := exported["secret"].(bool); ok {
			return secret
		}
	}
	return false
}

type testAPI struct {
	response  *Response
	variables map[string]string
	globals   vars.Globals
	changes   vars.Globals
	secrets   *vars.Secrets
	cases     []TestResult
	stream    *StreamInfo
	trace     *traceBinding
	vm        *goja.Runtime
}

func newTestAPI(in TestInput, stream *StreamInfo) *testAPI {
	return &testAPI{
		response:  in.Response,
		variables: jsVarsView(in.Variables),
		globals:   in.Globals.Clone(),
		secrets:   in.Secrets,
		stream:    stream,
		trace:     newTraceBinding(in.Trace),
	}
}

type streamAPI struct {
	vm            *goja.Runtime
	info          *StreamInfo
	eventHandlers []goja.Callable
	closeHandlers []goja.Callable
}

func newStreamAPI(vm *goja.Runtime, info *StreamInfo) *streamAPI {
	clone := info.Clone()
	return &streamAPI{vm: vm, info: clone}
}

func (api *streamAPI) object() map[string]any {
	enabled := api.info != nil
	return map[string]any{
		"enabled": func() bool { return enabled },
		"kind": func() string {
			if api.info == nil {
				return ""
			}
			return api.info.Kind
		},
		"summary": func() map[string]any {
			if api.info == nil || len(api.info.Summary) == 0 {
				return map[string]any{}
			}
			clone := make(map[string]any, len(api.info.Summary))
			maps.Copy(clone, api.info.Summary)
			return clone
		},
		"events": func() []map[string]any {
			if api.info == nil || len(api.info.Events) == 0 {
				return []map[string]any{}
			}
			out := make([]map[string]any, len(api.info.Events))
			for i, evt := range api.info.Events {
				if evt == nil {
					continue
				}
				copyEvt := make(map[string]any, len(evt))
				maps.Copy(copyEvt, evt)
				out[i] = copyEvt
			}
			return out
		},
		"onEvent": api.registerEventHandler,
		"onClose": api.registerCloseHandler,
	}
}

func (api *streamAPI) registerEventHandler(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return goja.Undefined()
	}

	fn, ok := goja.AssertFunction(call.Arguments[0])
	if ok {
		api.eventHandlers = append(api.eventHandlers, fn)
	}
	return goja.Undefined()
}

func (api *streamAPI) registerCloseHandler(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return goja.Undefined()
	}

	fn, ok := goja.AssertFunction(call.Arguments[0])
	if ok {
		api.closeHandlers = append(api.closeHandlers, fn)
	}
	return goja.Undefined()
}

func (api *streamAPI) replay() error {
	if api.info == nil {
		return nil
	}
	for _, evt := range api.info.Events {
		val := api.vm.ToValue(evt)
		for _, handler := range api.eventHandlers {
			if _, err := handler(goja.Undefined(), val); err != nil {
				return err
			}
		}
	}
	summaryVal := api.vm.ToValue(api.info.Summary)
	for _, handler := range api.closeHandlers {
		if _, err := handler(goja.Undefined(), summaryVal); err != nil {
			return err
		}
	}
	return nil
}

func (api *testAPI) testsAPI() map[string]any {
	return map[string]any{
		"assert": api.assert,
		"fail":   api.fail,
	}
}

func (api *testAPI) clientAPI() map[string]any {
	return map[string]any{
		"test": api.namedTest,
	}
}

func (api *testAPI) traceAPI() map[string]any {
	if api.trace == nil {
		return newTraceBinding(nil).object()
	}
	return api.trace.object()
}

func (api *testAPI) responseAPI() map[string]any {
	body := ""
	status := ""
	code := 0
	url := ""
	seconds := 0.0
	headers := map[string]string{}
	kind := ""
	ct := ""
	wireCT := ""
	disposition := ""
	var meta binaryview.Meta
	if r := api.response; r != nil {
		body = string(r.Body)
		status = r.Status
		code = r.Code
		url = r.URL
		seconds = r.Time.Seconds()
		kind = string(r.Kind)
		ct = strings.TrimSpace(r.ContentType)
		headerCT := ""
		if r.Header != nil {
			headerCT = strings.TrimSpace(r.Header.Get("Content-Type"))
		}
		if ct == "" {
			ct = headerCT
		}
		wireCT = strings.TrimSpace(r.WireContentType)
		if wireCT == "" {
			wireCT = ct
		}
		disposition = r.Header.Get("Content-Disposition")
		metaSrc := r.Wire
		if len(metaSrc) == 0 {
			metaSrc = r.Body
		}
		metaCT := wireCT
		if strings.TrimSpace(metaCT) == "" {
			metaCT = ct
		}
		meta = binaryview.Analyze(metaSrc, metaCT)
		for name, values := range r.Header {
			headers[strings.ToLower(name)] = strings.Join(values, ", ")
		}
	}

	headerLookup := func(name string) string {
		if api.response == nil || api.response.Header == nil {
			return ""
		}
		return api.response.Header.Get(name)
	}

	headerHas := func(name string) bool {
		if api.response == nil || api.response.Header == nil {
			return false
		}
		if _, ok := api.response.Header[name]; ok {
			return true
		}
		_, ok := api.response.Header[http.CanonicalHeaderKey(name)]
		return ok
	}

	return map[string]any{
		"kind":        kind,
		"status":      status,
		"statusCode":  code,
		"url":         url,
		"duration":    seconds,
		"body":        body,
		"contentType": ct,
		"isBinary":    meta.Kind == binaryview.KindBinary,
		"json": func() any {
			if api.response == nil {
				return nil
			}
			var js any
			if err := json.Unmarshal(api.response.Body, &js); err != nil {
				return nil
			}
			return js
		},
		"base64": func() string {
			if api.response == nil {
				return ""
			}
			src := api.response.Wire
			if len(src) == 0 {
				src = api.response.Body
			}
			return base64.StdEncoding.EncodeToString(src)
		},
		"arrayBuffer": func() []byte {
			if api.response == nil {
				return nil
			}
			src := api.response.Wire
			if len(src) == 0 {
				src = api.response.Body
			}
			return append([]byte(nil), src...)
		},
		"bytes": func() []byte {
			if api.response == nil {
				return nil
			}
			src := api.response.Wire
			if len(src) == 0 {
				src = api.response.Body
			}
			return append([]byte(nil), src...)
		},
		"filename": func() string {
			nameCT := wireCT
			if strings.TrimSpace(nameCT) == "" {
				nameCT = ct
			}
			return binaryview.FilenameHint(disposition, url, nameCT)
		},
		"saveBody": func(path string) bool {
			if api.response == nil {
				return false
			}
			trimmed := strings.TrimSpace(path)
			if trimmed == "" {
				return false
			}
			src := api.response.Wire
			if len(src) == 0 {
				src = api.response.Body
			}
			if err := os.WriteFile(trimmed, src, 0o644); err != nil {
				panic(api.vm.NewGoError(err))
			}
			return true
		},
		"headers": map[string]any{
			"get": headerLookup,
			"has": headerHas,
			"all": headers,
		},
		"stream": func() map[string]any {
			if api.stream == nil {
				return map[string]any{"enabled": false}
			}
			clone := api.stream.Clone()
			if clone.Summary == nil {
				clone.Summary = make(map[string]any)
			}
			if clone.Events == nil {
				clone.Events = []map[string]any{}
			}
			return map[string]any{
				"enabled": true,
				"kind":    clone.Kind,
				"summary": clone.Summary,
				"events":  clone.Events,
			}
		},
	}
}

// Test-script variable writes are local to the script.
func (api *testAPI) varsAPI() map[string]any {
	return jsVarsAPI(api.variables, nil, api.globalAPI())
}

func (api *testAPI) globalAPI() map[string]any {
	return map[string]any{
		"get": func(name string) string {
			entry, _ := api.globals.Get(name)
			return entry.Value
		},
		"set": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}

			name := call.Arguments[0].String()
			value := call.Arguments[1].String()
			secret := false
			if len(call.Arguments) >= 3 {
				secret = parseGlobalSecret(call.Arguments[2])
			}

			api.setGlobal(name, value, secret)
			return goja.Undefined()
		},
		"has": func(name string) bool {
			return api.globals.Has(name)
		},
		"delete": func(name string) {
			api.deleteGlobal(name)
		},
	}
}

func (api *testAPI) setGlobal(name, value string, secret bool) {
	entry := vars.GlobalMutation{Name: name, Value: value, Secret: secret}
	if !api.changes.Set(name, entry) {
		return
	}
	if secret {
		api.secrets.Add(value)
	}
	api.globals.Set(name, entry)
}

func (api *testAPI) deleteGlobal(name string) {
	if api.changes.Set(name, vars.GlobalMutation{Name: name, Delete: true}) {
		api.globals.Delete(name)
	}
}

func (api *testAPI) globalChanges() vars.Globals {
	return api.changes.Clone()
}

func (api *testAPI) assert(condition bool, message string) {
	name := message
	if name == "" {
		name = "assert"
	}

	result := TestResult{
		Name:   name,
		Passed: condition,
	}

	if !condition && message != "" {
		result.Message = message
	}
	api.cases = append(api.cases, result)
}

func (api *testAPI) fail(message string) {
	if message == "" {
		message = "fail"
	}

	api.cases = append(api.cases, TestResult{
		Name:    message,
		Message: message,
		Passed:  false,
	})
}

func (api *testAPI) namedTest(name string, callable goja.Callable) {
	start := time.Now()
	passed := true
	message := ""

	defer func() {
		if r := recover(); r != nil {
			passed = false
			message = fmt.Sprintf("panic: %v", r)
		}
		api.cases = append(api.cases, TestResult{
			Name:    name,
			Message: message,
			Passed:  passed,
			Elapsed: time.Since(start),
		})
	}()

	if callable == nil {
		passed = false
		message = "client.test requires a function argument"
		return
	}

	if _, err := callable(goja.Undefined()); err != nil {
		passed = false
		message = err.Error()
	}
}

func (api *testAPI) results() []TestResult {
	return append([]TestResult(nil), api.cases...)
}
