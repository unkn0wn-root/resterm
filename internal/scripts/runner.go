package scripts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

type PreRequestInput struct {
	Request   *restfile.Request
	Variables map[string]string
}

type PreRequestOutput struct {
	Headers   http.Header
	Query     map[string]string
	Body      *string
	URL       *string
	Method    *string
	Variables map[string]string
}

type TestInput struct {
	Response  *httpclient.Response
	Variables map[string]string
}

type TestResult struct {
	Name    string
	Message string
	Passed  bool
	Elapsed time.Duration
}

func (r *Runner) RunPreRequest(scripts []restfile.ScriptBlock, input PreRequestInput) (PreRequestOutput, error) {
	result := PreRequestOutput{
		Headers:   make(http.Header),
		Query:     make(map[string]string),
		Variables: make(map[string]string),
	}

	for idx, block := range scripts {
		if strings.ToLower(block.Kind) != "pre-request" {
			continue
		}
		script := normalizeScript(block.Body)
		if script == "" {
			continue
		}
		if err := r.executePreRequestScript(script, input, &result); err != nil {
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

	return result, nil
}

func (r *Runner) RunTests(scripts []restfile.ScriptBlock, input TestInput) ([]TestResult, error) {
	var aggregated []TestResult

	for idx, block := range scripts {
		if kind := strings.ToLower(block.Kind); kind != "test" && kind != "tests" {
			continue
		}
		script := normalizeScript(block.Body)
		if script == "" {
			continue
		}
		results, err := r.executeTestScript(script, input)
		if err != nil {
			return aggregated, errdef.Wrap(errdef.CodeScript, err, "test script %d", idx+1)
		}
		aggregated = append(aggregated, results...)
	}

	return aggregated, nil
}

func (r *Runner) executePreRequestScript(script string, input PreRequestInput, output *PreRequestOutput) error {
	vm := goja.New()
	pre := newPreRequestAPI(output, input)
	bindCommon(vm)
	vm.Set("request", pre.requestAPI())
	vm.Set("vars", pre.varsAPI())

	_, err := vm.RunString(script)
	if err != nil {
		return errdef.Wrap(errdef.CodeScript, err, "execute pre-request script")
	}
	return nil
}

func (r *Runner) executeTestScript(script string, input TestInput) ([]TestResult, error) {
	vm := goja.New()
	tester := newTestAPI(input.Response, input.Variables)
	bindCommon(vm)
	vm.Set("tests", tester.testsAPI())
	vm.Set("client", tester.clientAPI())
	vm.Set("resterm", tester.clientAPI())
	vm.Set("response", tester.responseAPI())
	vm.Set("vars", tester.varsAPI())

	_, err := vm.RunString(script)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeScript, err, "execute test script")
	}
	return tester.results(), nil
}

func bindCommon(vm *goja.Runtime) {
	console := map[string]func(goja.FunctionCall) goja.Value{
		"log":   func(call goja.FunctionCall) goja.Value { return goja.Undefined() },
		"warn":  func(call goja.FunctionCall) goja.Value { return goja.Undefined() },
		"error": func(call goja.FunctionCall) goja.Value { return goja.Undefined() },
	}
	vm.Set("console", console)
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

type preRequestAPI struct {
	request   *restfile.Request
	output    *PreRequestOutput
	variables map[string]string
}

func newPreRequestAPI(output *PreRequestOutput, input PreRequestInput) *preRequestAPI {
	vars := make(map[string]string, len(input.Variables))
	for k, v := range input.Variables {
		vars[k] = v
	}
	return &preRequestAPI{request: input.Request, output: output, variables: vars}
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
	}
}

type testAPI struct {
	response  *httpclient.Response
	variables map[string]string
	cases     []TestResult
}

func newTestAPI(resp *httpclient.Response, vars map[string]string) *testAPI {
	copyVars := make(map[string]string, len(vars))
	for k, v := range vars {
		copyVars[k] = v
	}
	return &testAPI{response: resp, variables: copyVars}
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

func (api *testAPI) responseAPI() map[string]interface{} {
	body := ""
	status := ""
	statusCode := 0
	url := ""
	duration := 0.0
	headers := map[string]string{}
	if api.response != nil {
		body = string(api.response.Body)
		status = api.response.Status
		statusCode = api.response.StatusCode
		url = api.response.EffectiveURL
		duration = api.response.Duration.Seconds()
		for name, values := range api.response.Headers {
			headers[strings.ToLower(name)] = strings.Join(values, ", ")
		}
	}

	return map[string]interface{}{
		"status":     status,
		"statusCode": statusCode,
		"url":        url,
		"duration":   duration,
		"body":       body,
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
		"headers": map[string]interface{}{
			"get": func(name string) string {
				if api.response == nil {
					return ""
				}
				return api.response.Headers.Get(name)
			},
			"has": func(name string) bool {
				if api.response == nil {
					return false
				}
				_, ok := api.response.Headers[name]
				if ok {
					return true
				}
				_, ok = api.response.Headers[http.CanonicalHeaderKey(name)]
				return ok
			},
			"all": headers,
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
	}
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
