package explain

const (
	StageApply            = "@apply"
	StageCondition        = "condition"
	StageRoute            = "route"
	StageSettings         = "settings"
	StageAuth             = "auth"
	StageRTSPreRequest    = "rts pre-request"
	StageJSPreRequest     = "js pre-request"
	StageGRPCPrepare      = "grpc prepare"
	StageHTTPPrepare      = "http prepare"
	StageWebSocketPrepare = "websocket prepare"
	StageCaptures         = "captures"
)

const (
	RouteKindDirect = "direct"
	RouteKindSSH    = "ssh"
	RouteKindK8s    = "k8s"
)

const (
	SummaryApplyComplete               = "apply complete"
	SummaryApplyFailed                 = "apply failed"
	SummaryConditionPassed             = "condition passed"
	SummaryConditionBlockedRequest     = "condition blocked request"
	SummaryConditionEvaluationFailed   = "condition evaluation failed"
	SummaryRouteSSHResolutionFailed    = "ssh resolution failed"
	SummaryRouteK8sResolutionFailed    = "k8s resolution failed"
	SummaryRouteConfigInvalid          = "route configuration invalid"
	SummarySettingsMerged              = "effective settings merged"
	SummarySettingsApplyFailed         = "settings application failed"
	SummaryAuthPrepared                = "auth prepared"
	SummaryAuthInjectionFailed         = "auth injection failed"
	SummaryOAuthTokenFetchSkipped      = "oauth token fetch skipped"
	SummaryCommandAuthExecutionSkipped = "command auth execution skipped"
	SummaryAuthTypeNotApplied          = "auth type not applied"
	SummaryRTSPreRequestComplete       = "RTS pre-request complete"
	SummaryRTSPreRequestFailed         = "RTS pre-request failed"
	SummaryRTSPreRequestOutputBad      = "RTS pre-request output invalid"
	SummaryJSPreRequestComplete        = "JS pre-request complete"
	SummaryJSPreRequestFailed          = "JS pre-request failed"
	SummaryJSPreRequestOutputBad       = "JS pre-request output invalid"
	SummaryGRPCRequestPrepared         = "gRPC request prepared"
	SummaryGRPCPrepareFailed           = "gRPC preparation failed"
	SummaryHTTPRequestPrepared         = "HTTP request prepared"
	SummaryHTTPRequestBuildFailed      = "HTTP request build failed"
	SummaryWebSocketRequestPrepared    = "WebSocket request prepared"
	SummaryWebSocketPrepareFailed      = "WebSocket preparation failed"
	SummaryCaptureEvaluationFailed     = "capture evaluation failed"
)
