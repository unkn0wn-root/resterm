package runner

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type selectSpec struct {
	request  string
	workflow string
	tag      string
	all      bool
	line     int
}

type selectedTarget struct {
	requests []*restfile.Request
	workflow *restfile.Workflow
}

func newSelectSpec(sel Select) selectSpec {
	return selectSpec{
		request:  str.Trim(sel.Request),
		workflow: str.Trim(sel.Workflow),
		tag:      str.Trim(sel.Tag),
		all:      sel.All,
		line:     sel.Line,
	}
}

func selectTarget(doc *restfile.Document, sel selectSpec) (selectedTarget, error) {
	if sel.line > 0 {
		return selectByLine(doc, sel)
	}
	if sel.workflow != "" {
		if sel.all || sel.request != "" || sel.tag != "" {
			return selectedTarget{}, usageError(
				"--workflow cannot be combined with --request, --tag, or --all",
			)
		}
		wf, err := selectWorkflow(doc, sel.workflow)
		if err != nil {
			return selectedTarget{}, err
		}
		return selectedTarget{workflow: wf}, nil
	}
	reqs, err := selectRequests(doc, sel)
	if err != nil {
		return selectedTarget{}, err
	}
	return selectedTarget{requests: reqs}, nil
}

func selectRequests(doc *restfile.Document, sel selectSpec) ([]*restfile.Request, error) {
	if doc == nil || len(doc.Requests) == 0 {
		return nil, usageError("no requests found")
	}

	if sel.all && sel.line > 0 {
		return nil, usageError("--all cannot be combined with --line")
	}
	if sel.all && (sel.request != "" || sel.tag != "") {
		return nil, usageError("--all cannot be combined with --request or --tag")
	}
	if sel.request != "" && sel.line > 0 {
		return nil, usageError("--request cannot be combined with --line")
	}
	if sel.tag != "" && sel.line > 0 {
		return nil, usageError("--tag cannot be combined with --line")
	}
	if sel.request != "" && sel.tag != "" {
		return nil, usageError("--request cannot be combined with --tag")
	}

	if sel.all {
		return append([]*restfile.Request(nil), doc.Requests...), nil
	}

	if sel.request != "" {
		return selectByRequestName(doc.Requests, sel.request)
	}

	if sel.tag != "" {
		return selectByTag(doc.Requests, sel.tag)
	}

	if len(doc.Requests) == 1 {
		return []*restfile.Request{doc.Requests[0]}, nil
	}
	return nil, usageError("multiple requests found; use --request, --tag, --line, or --all")
}

func selectByLine(doc *restfile.Document, sel selectSpec) (selectedTarget, error) {
	if sel.workflow != "" || sel.request != "" || sel.tag != "" || sel.all {
		return selectedTarget{}, usageError(
			"--line cannot be combined with --workflow, --request, --tag, or --all",
		)
	}
	if sel.line <= 0 {
		return selectedTarget{}, usageError("--line must be greater than zero")
	}

	reqs := selectRequestsByLine(doc, sel.line)
	wfs := selectWorkflowsByLine(doc, sel.line)
	switch total := len(reqs) + len(wfs); total {
	case 0:
		return selectedTarget{}, usageError(
			"line %d did not match any request or workflow",
			sel.line,
		)
	case 1:
		if len(wfs) == 1 {
			return selectedTarget{workflow: wfs[0]}, nil
		}
		return selectedTarget{requests: reqs}, nil
	default:
		return selectedTarget{}, usageError("line %d matched %d entries", sel.line, total)
	}
}

func selectWorkflow(doc *restfile.Document, name string) (*restfile.Workflow, error) {
	if doc == nil || len(doc.Workflows) == 0 {
		return nil, usageError("no workflows found")
	}
	var out []*restfile.Workflow
	for i := range doc.Workflows {
		wf := &doc.Workflows[i]
		if strings.EqualFold(str.Trim(wf.Name), name) {
			out = append(out, wf)
		}
	}
	switch len(out) {
	case 0:
		return nil, usageError("workflow %q not found", name)
	case 1:
		return out[0], nil
	default:
		return nil, usageError("workflow %q matched %d entries", name, len(out))
	}
}

func selectByRequestName(reqs []*restfile.Request, name string) ([]*restfile.Request, error) {
	var out []*restfile.Request
	for _, req := range reqs {
		if strings.EqualFold(str.Trim(req.Metadata.Name), name) {
			out = append(out, req)
		}
	}
	switch len(out) {
	case 0:
		return nil, usageError("request %q not found", name)
	case 1:
		return out, nil
	default:
		return nil, usageError("request %q matched %d entries", name, len(out))
	}
}

func selectByTag(reqs []*restfile.Request, tag string) ([]*restfile.Request, error) {
	var out []*restfile.Request
	for _, req := range reqs {
		for _, item := range req.Metadata.Tags {
			if strings.EqualFold(str.Trim(item), tag) {
				out = append(out, req)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, usageError("tag %q did not match any requests", tag)
	}
	return out, nil
}

func selectRequestsByLine(doc *restfile.Document, line int) []*restfile.Request {
	if doc == nil || line <= 0 {
		return nil
	}
	out := make([]*restfile.Request, 0, 1)
	for _, req := range doc.Requests {
		if req == nil || !lineInRange(line, req.LineRange) {
			continue
		}
		out = append(out, req)
	}
	return out
}

func selectWorkflowsByLine(doc *restfile.Document, line int) []*restfile.Workflow {
	if doc == nil || line <= 0 {
		return nil
	}
	out := make([]*restfile.Workflow, 0, 1)
	for i := range doc.Workflows {
		wf := &doc.Workflows[i]
		if !lineInRange(line, wf.LineRange) {
			continue
		}
		out = append(out, wf)
	}
	return out
}

func lineInRange(line int, rg restfile.LineRange) bool {
	if line <= 0 || rg.Start <= 0 {
		return false
	}
	end := rg.End
	if end < rg.Start {
		end = rg.Start
	}
	return line >= rg.Start && line <= end
}
