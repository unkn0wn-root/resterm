package workspace

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type RefKind uint8

const (
	RefBody RefKind = iota
	RefGraphQL
	RefScript
	RefUse
	RefWebSocket
	RefGRPC
	RefRTSJSON
)

type Ref struct {
	Path string
	Kind RefKind
	Line int
}

func Refs(doc *restfile.Document) []Ref {
	if doc == nil {
		return nil
	}
	c := refCollector{doc: doc}
	c.collectDoc()
	return c.refs
}

type refCollector struct {
	doc  *restfile.Document
	refs []Ref
}

func (c *refCollector) collectDoc() {
	c.collectUses(c.doc.Uses)
	c.collectPatches(c.doc.Patches)
	for _, req := range c.doc.Requests {
		c.collectReq(req)
	}
	for _, wf := range c.doc.Workflows {
		c.collectWorkflow(wf)
	}
}

func (c *refCollector) collectReq(req *restfile.Request) {
	if req == nil {
		return
	}

	line := req.LineRange.Start
	c.collectBody(req, line)
	c.collectGRPC(req.GRPC, line)
	c.collectScripts(req.Metadata.Scripts, line)
	c.collectUses(req.Metadata.Uses)
	c.collectMeta(req.Metadata)
	c.collectWebSocket(req.WebSocket, line)
}

func (c *refCollector) collectBody(req *restfile.Request, line int) {
	c.add(RefBody, req.Body.FilePath, line)
	if gql := req.Body.GraphQL; gql != nil {
		c.add(RefGraphQL, gql.QueryFile, line)
		c.add(RefGraphQL, gql.VariablesFile, line)
	}
}

func (c *refCollector) collectGRPC(req *restfile.GRPCRequest, line int) {
	if req == nil {
		return
	}
	c.add(RefGRPC, req.MessageFile, line)
	c.add(RefGRPC, req.DescriptorSet, line)
}

func (c *refCollector) collectScripts(scripts []restfile.ScriptBlock, line int) {
	for _, block := range scripts {
		c.add(RefScript, block.FilePath, line)
		if isRTS(block.Lang) && strings.TrimSpace(block.Body) != "" {
			c.module(block.Body, line)
		}
	}
}

func (c *refCollector) collectUses(uses []restfile.UseSpec) {
	for _, spec := range uses {
		c.add(RefUse, spec.Path, spec.Line)
	}
}

func (c *refCollector) collectPatches(patches []restfile.PatchProfile) {
	for _, prof := range patches {
		c.expr(prof.Expression, prof.Line)
	}
}

func (c *refCollector) collectMeta(meta restfile.RequestMetadata) {
	if meta.When != nil {
		c.expr(meta.When.Expression, meta.When.Line)
	}
	if meta.ForEach != nil {
		c.expr(meta.ForEach.Expression, meta.ForEach.Line)
	}
	for _, spec := range meta.Asserts {
		c.expr(spec.Expression, spec.Line)
	}
	for _, spec := range meta.Applies {
		c.expr(spec.Expression, spec.Line)
	}
	for _, spec := range meta.Captures {
		if spec.Mode != restfile.CaptureExprModeTemplate {
			c.expr(spec.Expression, spec.Line)
		}
	}
}

func (c *refCollector) collectWebSocket(ws *restfile.WebSocketRequest, line int) {
	if ws == nil {
		return
	}
	for _, step := range ws.Steps {
		if step.Type == restfile.WebSocketStepSendFile {
			c.add(RefWebSocket, step.File, line)
		}
	}
}

func (c *refCollector) collectWorkflow(wf restfile.Workflow) {
	for _, step := range wf.Steps {
		c.collectWorkflowStep(step)
	}
}

func (c *refCollector) collectWorkflowStep(step restfile.WorkflowStep) {
	if step.When != nil {
		c.expr(step.When.Expression, step.When.Line)
	}
	if step.If != nil {
		c.expr(step.If.Cond, step.If.Line)
		for _, branch := range step.If.Elifs {
			c.expr(branch.Cond, branch.Line)
		}
		if step.If.Else != nil {
			c.expr(step.If.Else.Cond, step.If.Else.Line)
		}
	}
	if step.Switch != nil {
		c.expr(step.Switch.Expr, step.Switch.Line)
		for _, cs := range step.Switch.Cases {
			c.expr(cs.Expr, cs.Line)
		}
	}
	if step.ForEach != nil {
		c.expr(step.ForEach.Expr, step.ForEach.Line)
	}
}

func (c *refCollector) expr(expr string, line int) {
	for _, path := range jsonFileExprs(c.doc.Path, line, 1, expr) {
		c.add(RefRTSJSON, path, line)
	}
}

func (c *refCollector) module(src string, line int) {
	for _, path := range jsonFileModuleRefs(c.doc.Path, src) {
		c.add(RefRTSJSON, path, line)
	}
}

func (c *refCollector) add(kind RefKind, path string, line int) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	c.refs = append(c.refs, Ref{Path: path, Kind: kind, Line: line})
}

func isRTS(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "rts", "restermlang":
		return true
	default:
		return false
	}
}
