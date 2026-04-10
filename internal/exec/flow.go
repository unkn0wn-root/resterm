package exec

import (
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

type RequestResult struct {
	Response       *httpclient.Response
	GRPC           *grpcclient.Response
	Stream         *scripts.StreamInfo
	Transcript     []byte
	Err            error
	Tests          []scripts.TestResult
	ScriptErr      error
	Executed       *restfile.Request
	RequestText    string
	RuntimeSecrets []string
	Environment    string
	Skipped        bool
	SkipReason     string
	Preview        bool
	Explain        *explain.Report
	Timing         engine.Timing
}

type RequestFlow interface {
	PendingCancel() *RequestResult
	Finish()
	EvaluateCondition() *RequestResult
	RunPreRequest() *RequestResult
	PrepareRequest() *RequestResult
	PreviewResult() RequestResult
	UseGRPC() bool
	IsInteractiveWebSocket() bool
	ExecuteInteractiveWebSocket() RequestResult
	ExecuteGRPC() RequestResult
	ExecuteHTTP() RequestResult
}

func RunRequest(flow RequestFlow) RequestResult {
	if flow == nil {
		return RequestResult{}
	}
	if res := flow.PendingCancel(); res != nil {
		return *res
	}
	defer flow.Finish()

	if res := flow.EvaluateCondition(); res != nil {
		return *res
	}
	if res := flow.RunPreRequest(); res != nil {
		return *res
	}
	if res := flow.PrepareRequest(); res != nil {
		return *res
	}
	if res := flow.PreviewResult(); res.Preview {
		return res
	}
	if flow.UseGRPC() {
		return flow.ExecuteGRPC()
	}
	if flow.IsInteractiveWebSocket() {
		return flow.ExecuteInteractiveWebSocket()
	}
	return flow.ExecuteHTTP()
}
