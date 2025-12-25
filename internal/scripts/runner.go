package scripts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Runner struct {
	fs httpclient.FileSystem
}

func NewRunner(fs httpclient.FileSystem) *Runner {
	if fs == nil {
		fs = httpclient.OSFileSystem{}
	}
	return &Runner{fs: fs}
}

type GlobalValue struct {
	Name   string
	Value  string
	Secret bool
	Delete bool
}

type PreRequestInput struct {
	Request   *restfile.Request
	Variables map[string]string
	Globals   map[string]GlobalValue
	BaseDir   string
	Context   context.Context
}

type PreRequestOutput struct {
	Headers   http.Header
	Query     map[string]string
	Body      *string
	URL       *string
	Method    *string
	Variables map[string]string
	Globals   map[string]GlobalValue
}

type TestInput struct {
	Response  *Response
	Variables map[string]string
	Globals   map[string]GlobalValue
	BaseDir   string
	Stream    *StreamInfo
	Trace     *TraceInput
}

type TestResult struct {
	Name    string
	Message string
	Passed  bool
	Elapsed time.Duration
}

func (r *Runner) RunPreRequest(
	scripts []restfile.ScriptBlock,
	input PreRequestInput,
) (PreRequestOutput, error) {
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	result := PreRequestOutput{
		Headers:   make(http.Header),
		Query:     make(map[string]string),
		Variables: make(map[string]string),
		Globals:   make(map[string]GlobalValue),
	}

	for idx, block := range scripts {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		if strings.ToLower(block.Kind) != "pre-request" {
			continue
		}
		if scriptLang(block) != "js" {
			continue
		}

		script, err := r.loadScript(block, input.BaseDir)
		if err != nil {
			return result, errdef.Wrap(errdef.CodeScript, err, "pre-request script %d", idx+1)
		}
		if script == "" {
			continue
		}

		if err := r.executePreRequestScript(ctx, script, input, &result); err != nil {
			return result, errdef.Wrap(errdef.CodeScript, err, "pre-request script %d", idx+1)
		}
	}

	if len(result.Headers) == 0 {
		result.Headers = nil
	}
	if len(result.Query) == 0 {
		result.Query = nil
	}
	if len(result.Variables) == 0 {
		result.Variables = nil
	}
	if len(result.Globals) == 0 {
		result.Globals = nil
	}

	return result, nil
}

func (r *Runner) RunTests(
	scripts []restfile.ScriptBlock,
	input TestInput,
) ([]TestResult, map[string]GlobalValue, error) {
	var aggregated []TestResult
	changes := make(map[string]GlobalValue)

	for idx, block := range scripts {
		if kind := strings.ToLower(block.Kind); kind != "test" && kind != "tests" {
			continue
		}
		if scriptLang(block) != "js" {
			continue
		}

		script, err := r.loadScript(block, input.BaseDir)
		if err != nil {
			return aggregated, changes, errdef.Wrap(errdef.CodeScript, err, "test script %d", idx+1)
		}
		if script == "" {
			continue
		}

		results, globals, err := r.executeTestScript(script, input)
		if err != nil {
			return aggregated, changes, errdef.Wrap(errdef.CodeScript, err, "test script %d", idx+1)
		}

		aggregated = append(aggregated, results...)
		for key, value := range globals {
			changes[key] = value
		}
	}

	if len(changes) == 0 {
		changes = nil
	}
	return aggregated, changes, nil
}

func scriptLang(block restfile.ScriptBlock) string {
	lang := strings.ToLower(strings.TrimSpace(block.Lang))
	switch lang {
	case "", "javascript":
		return "js"
	default:
		return lang
	}
}

func (r *Runner) executePreRequestScript(
	ctx context.Context,
	script string,
	input PreRequestInput,
	output *PreRequestOutput,
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

	pre := newPreRequestAPI(output, input)
	if err := bindCommon(vm); err != nil {
		return errdef.Wrap(errdef.CodeScript, err, "bind console api")
	}

	if err := vm.Set("request", pre.requestAPI()); err != nil {
		return errdef.Wrap(errdef.CodeScript, err, "bind request api")
	}

	if err := vm.Set("vars", pre.varsAPI()); err != nil {
		return errdef.Wrap(errdef.CodeScript, err, "bind vars api")
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
		return errdef.Wrap(errdef.CodeScript, err, "execute pre-request script")
	}
	return nil
}

func (r *Runner) executeTestScript(
	script string,
	input TestInput,
) ([]TestResult, map[string]GlobalValue, error) {
	vm := goja.New()
	streamInfo := input.Stream.Clone()
	tester := newTestAPI(input.Response, input.Variables, input.Globals, streamInfo, input.Trace)
	tester.vm = vm
	streamBinding := newStreamAPI(vm, streamInfo)

	if err := bindCommon(vm); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "bind console api")
	}

	if err := vm.Set("tests", tester.testsAPI()); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "bind tests api")
	}

	if err := vm.Set("client", tester.clientAPI()); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "bind client api")
	}

	if err := vm.Set("resterm", tester.clientAPI()); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "bind resterm alias")
	}

	if err := vm.Set("response", tester.responseAPI()); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "bind response api")
	}

	if err := vm.Set("vars", tester.varsAPI()); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "bind vars api")
	}

	if err := vm.Set("stream", streamBinding.object()); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "bind stream api")
	}

	if err := vm.Set("trace", tester.traceAPI()); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "bind trace api")
	}

	_, err := vm.RunString(script)
	if err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "execute test script")
	}

	if err := streamBinding.replay(); err != nil {
		return nil, nil, errdef.Wrap(errdef.CodeScript, err, "execute stream callbacks")
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
	if strings.TrimSpace(block.FilePath) == "" {
		return normalizeScript(block.Body), nil
	}

	path := block.FilePath
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}

	data, err := r.fs.ReadFile(path)
	if err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "read script file %s", path)
	}
	return normalizeScript(string(data)), nil
}

func normalizeGlobalKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

type preRequestAPI struct {
	request   *restfile.Request
	output    *PreRequestOutput
	variables map[string]string
	globals   map[string]GlobalValue
}

func newPreRequestAPI(output *PreRequestOutput, input PreRequestInput) *preRequestAPI {
	vars := make(map[string]string, len(input.Variables))
	for k, v := range input.Variables {
		vars[k] = v
	}

	globals := make(map[string]GlobalValue, len(input.Globals))
	for key, value := range input.Globals {
		if strings.TrimSpace(value.Name) == "" {
			value.Name = key
		}
		globals[normalizeGlobalKey(value.Name)] = value
	}
	return &preRequestAPI{request: input.Request, output: output, variables: vars, globals: globals}
}

func (api *preRequestAPI) requestAPI() map[string]interface{} {
	return map[string]interface{}{
		"getURL": func() string {
			if api.request == nil {
				return ""
			}
			return api.request.URL
		},
		"getMethod": func() string {
			if api.request == nil {
				return ""
			}
			return api.request.Method
		},
		"getHeader": func(name string) string {
			if api.request == nil || api.request.Headers == nil {
				return ""
			}
			return api.request.Headers.Get(name)
		},
		"setHeader": func(name, value string) {
			if api.output.Headers == nil {
				api.output.Headers = make(http.Header)
			}
			api.output.Headers.Set(name, value)
		},
		"addHeader": func(name, value string) {
			if api.output.Headers == nil {
				api.output.Headers = make(http.Header)
			}
			api.output.Headers.Add(name, value)
		},
		"removeHeader": func(name string) {
			if api.output.Headers != nil {
				api.output.Headers.Del(name)
			}
		},
		"setQueryParam": func(name, value string) {
			if api.output.Query == nil {
				api.output.Query = make(map[string]string)
			}
			api.output.Query[name] = value
		},
		"setURL": func(url string) {
			copied := url
			api.output.URL = &copied
		},
		"setMethod": func(method string) {
			copied := strings.ToUpper(method)
			api.output.Method = &copied
		},
		"setBody": func(body string) {
			copied := body
			api.output.Body = &copied
		},
	}
}

func (api *preRequestAPI) varsAPI() map[string]interface{} {
	return map[string]interface{}{
		"get": func(name string) string {
			return api.variables[name]
		},
		"set": func(name, value string) {
			if api.output.Variables == nil {
				api.output.Variables = make(map[string]string)
			}
			api.output.Variables[name] = value
			api.variables[name] = value
		},
		"has": func(name string) bool {
			_, ok := api.variables[name]
			return ok
		},
		"global": api.globalAPI(),
	}
}

func (api *preRequestAPI) globalAPI() map[string]interface{} {
	return map[string]interface{}{
		"get": func(name string) string {
			entry, ok := api.globals[normalizeGlobalKey(name)]
			if !ok {
				return ""
			}
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
			_, ok := api.globals[normalizeGlobalKey(name)]
			return ok
		},
		"delete": func(name string) {
			api.deleteGlobal(name)
		},
	}
}

func (api *preRequestAPI) setGlobal(name, value string, secret bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}

	key := normalizeGlobalKey(name)
	entry := GlobalValue{Name: name, Value: value, Secret: secret}
	api.globals[key] = entry
	if api.output.Globals == nil {
		api.output.Globals = make(map[string]GlobalValue)
	}
	api.output.Globals[key] = entry
}

func (api *preRequestAPI) deleteGlobal(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}

	key := normalizeGlobalKey(name)
	delete(api.globals, key)
	if api.output.Globals == nil {
		api.output.Globals = make(map[string]GlobalValue)
	}
	api.output.Globals[key] = GlobalValue{Name: name, Delete: true}
}

func parseGlobalSecret(value goja.Value) bool {
	switch exported := value.Export().(type) {
	case bool:
		return exported
	case map[string]interface{}:
		if secret, ok := exported["secret"].(bool); ok {
			return secret
		}
	}
	return false
}

type testAPI struct {
	response  *Response
	variables map[string]string
	globals   map[string]GlobalValue
	changes   map[string]GlobalValue
	cases     []TestResult
	stream    *StreamInfo
	trace     *traceBinding
	vm        *goja.Runtime
}

func newTestAPI(
	resp *Response,
	vars map[string]string,
	globals map[string]GlobalValue,
	stream *StreamInfo,
	trace *TraceInput,
) *testAPI {
	copyVars := make(map[string]string, len(vars))
	for k, v := range vars {
		copyVars[k] = v
	}

	globalCopy := make(map[string]GlobalValue, len(globals))
	for key, value := range globals {
		if strings.TrimSpace(value.Name) == "" {
			value.Name = key
		}
		globalCopy[normalizeGlobalKey(value.Name)] = value
	}
	return &testAPI{
		response:  resp,
		variables: copyVars,
		globals:   globalCopy,
		changes:   make(map[string]GlobalValue),
		stream:    stream,
		trace:     newTraceBinding(trace),
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

func (api *streamAPI) object() map[string]interface{} {
	enabled := api.info != nil
	return map[string]interface{}{
		"enabled": func() bool { return enabled },
		"kind": func() string {
			if api.info == nil {
				return ""
			}
			return api.info.Kind
		},
		"summary": func() map[string]interface{} {
			if api.info == nil || len(api.info.Summary) == 0 {
				return map[string]interface{}{}
			}
			clone := make(map[string]interface{}, len(api.info.Summary))
			for k, v := range api.info.Summary {
				clone[k] = v
			}
			return clone
		},
		"events": func() []map[string]interface{} {
			if api.info == nil || len(api.info.Events) == 0 {
				return []map[string]interface{}{}
			}
			out := make([]map[string]interface{}, len(api.info.Events))
			for i, evt := range api.info.Events {
				if evt == nil {
					continue
				}
				copyEvt := make(map[string]interface{}, len(evt))
				for k, v := range evt {
					copyEvt[k] = v
				}
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

func (api *testAPI) testsAPI() map[string]interface{} {
	return map[string]interface{}{
		"assert": api.assert,
		"fail":   api.fail,
	}
}

func (api *testAPI) clientAPI() map[string]interface{} {
	return map[string]interface{}{
		"test": api.namedTest,
	}
}

func (api *testAPI) traceAPI() map[string]interface{} {
	if api.trace == nil {
		return newTraceBinding(nil).object()
	}
	return api.trace.object()
}

func (api *testAPI) responseAPI() map[string]interface{} {
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

	return map[string]interface{}{
		"kind":        kind,
		"status":      status,
		"statusCode":  code,
		"url":         url,
		"duration":    seconds,
		"body":        body,
		"contentType": ct,
		"isBinary":    meta.Kind == binaryview.KindBinary,
		"json": func() interface{} {
			if api.response == nil {
				return nil
			}
			var js interface{}
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
		"headers": map[string]interface{}{
			"get": headerLookup,
			"has": headerHas,
			"all": headers,
		},
		"stream": func() map[string]interface{} {
			if api.stream == nil {
				return map[string]interface{}{"enabled": false}
			}
			clone := api.stream.Clone()
			if clone.Summary == nil {
				clone.Summary = make(map[string]interface{})
			}
			if clone.Events == nil {
				clone.Events = []map[string]interface{}{}
			}
			return map[string]interface{}{
				"enabled": true,
				"kind":    clone.Kind,
				"summary": clone.Summary,
				"events":  clone.Events,
			}
		},
	}
}

func (api *testAPI) varsAPI() map[string]interface{} {
	return map[string]interface{}{
		"get": func(name string) string {
			return api.variables[name]
		},
		"set": func(name, value string) {
			api.variables[name] = value
		},
		"has": func(name string) bool {
			_, ok := api.variables[name]
			return ok
		},
		"global": api.globalAPI(),
	}
}

func (api *testAPI) globalAPI() map[string]interface{} {
	return map[string]interface{}{
		"get": func(name string) string {
			entry, ok := api.globals[normalizeGlobalKey(name)]
			if !ok {
				return ""
			}
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
			_, ok := api.globals[normalizeGlobalKey(name)]
			return ok
		},
		"delete": func(name string) {
			api.deleteGlobal(name)
		},
	}
}

func (api *testAPI) setGlobal(name, value string, secret bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}

	key := normalizeGlobalKey(name)
	entry := GlobalValue{Name: name, Value: value, Secret: secret}
	api.globals[key] = entry
	if api.changes == nil {
		api.changes = make(map[string]GlobalValue)
	}
	api.changes[key] = entry
}

func (api *testAPI) deleteGlobal(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}

	key := normalizeGlobalKey(name)
	delete(api.globals, key)
	if api.changes == nil {
		api.changes = make(map[string]GlobalValue)
	}
	api.changes[key] = GlobalValue{Name: name, Delete: true}
}

func (api *testAPI) globalChanges() map[string]GlobalValue {
	if len(api.changes) == 0 {
		return nil
	}

	clone := make(map[string]GlobalValue, len(api.changes))
	for key, value := range api.changes {
		clone[key] = value
	}
	return clone
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
