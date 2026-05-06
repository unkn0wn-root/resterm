package ui

import (
	"strings"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type explainFinalizeInput struct {
	report         *xplain.Report
	request        *restfile.Request
	envName        string
	preview        bool
	status         xplain.Status
	decision       string
	err            error
	trace          *vars.Trace
	mergedSettings map[string]string
	sshPlan        *ssh.Plan
	k8sPlan        *k8s.Plan
	globals        map[string]vars.GlobalMutation
	extraSecrets   []string
}

type explainBuilder struct {
	model          *Model
	request        *restfile.Request
	envName        string
	preview        bool
	report         *xplain.Report
	trace          *vars.Trace
	mergedSettings map[string]string
	sshPlan        *ssh.Plan
	k8sPlan        *k8s.Plan
	globals        map[string]vars.GlobalMutation
	extraSecrets   []string
}

func newExplainBuilder(
	m *Model,
	req *restfile.Request,
	envName string,
	preview bool,
) *explainBuilder {
	b := &explainBuilder{
		model:   m,
		request: req,
		envName: envName,
		preview: preview,
		report:  newExplainReport(req, envName),
	}
	if !preview || req == nil {
		return b
	}
	if req.Metadata.ForEach != nil {
		b.warn("@for-each iterations are not expanded in explain preview")
	}
	if req.Metadata.Compare != nil {
		b.warn("@compare sweep is not executed in explain preview")
	}
	if req.Metadata.Profile != nil {
		b.warn("@profile run is not executed in explain preview")
	}
	return b
}

func (b *explainBuilder) warn(msg string) {
	addExplainWarn(b.report, msg)
}

func (b *explainBuilder) stage(
	name string,
	st xplain.StageStatus,
	sum string,
	before *restfile.Request,
	after *restfile.Request,
	notes ...string,
) {
	addExplainStage(b.report, name, st, sum, before, after, notes...)
}

func (b *explainBuilder) sentHTTP(
	req *restfile.Request,
	resp *httpclient.Response,
	notes ...string,
) {
	addExplainSentHTTPStage(b.report, req, resp, notes...)
}

func (b *explainBuilder) setSettings(mergedSettings map[string]string) {
	b.mergedSettings = mergedSettings
}

func (b *explainBuilder) setRoute(sshPlan *ssh.Plan, k8sPlan *k8s.Plan) {
	b.sshPlan = sshPlan
	b.k8sPlan = k8sPlan
}

func (b *explainBuilder) addSecrets(values ...string) {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		b.extraSecrets = append(b.extraSecrets, value)
	}
}

func (b *explainBuilder) setPrepared(req *restfile.Request) {
	setExplainPrepared(b.report, req, b.mergedSettings, b.sshPlan, b.k8sPlan)
}

func (b *explainBuilder) setHTTP(resp *httpclient.Response) {
	setExplainHTTP(b.report, resp)
}

func (b *explainBuilder) finish(
	status xplain.Status,
	decision string,
	err error,
) *xplain.Report {
	return b.model.finalizeExplainReport(explainFinalizeInput{
		report:         b.report,
		request:        b.request,
		envName:        b.envName,
		preview:        b.preview,
		status:         status,
		decision:       decision,
		err:            err,
		trace:          b.trace,
		mergedSettings: b.mergedSettings,
		sshPlan:        b.sshPlan,
		k8sPlan:        b.k8sPlan,
		globals:        b.globals,
		extraSecrets:   b.extraSecrets,
	})
}

func (m *Model) finalizeExplainReport(in explainFinalizeInput) *xplain.Report {
	if in.report == nil {
		return nil
	}
	if in.trace != nil {
		finalizeExplainVars(in.report, in.trace)
	}
	if in.report.Final == nil {
		setExplainPrepared(in.report, in.request, in.mergedSettings, in.sshPlan, in.k8sPlan)
	}
	fail := in.report.Failure
	if in.err != nil && strings.TrimSpace(fail) == "" {
		fail = in.err.Error()
	}
	decision := in.decision
	if strings.TrimSpace(decision) == "" {
		decision = in.report.Decision
	}
	if strings.TrimSpace(decision) == "" {
		switch in.status {
		case xplain.StatusSkipped:
			decision = "Request skipped"
		case xplain.StatusError:
			decision = "Request preparation failed"
		default:
			if in.preview {
				decision = "Explain preview ready"
			} else {
				decision = "Request prepared"
			}
		}
	}
	setExplainDecision(in.report, in.status, decision, fail)
	return m.redactExplainReportWithState(
		in.report,
		in.envName,
		in.request,
		in.globals,
		in.extraSecrets...,
	)
}
