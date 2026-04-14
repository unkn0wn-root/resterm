package request

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (e *Engine) expandWebSocketSteps(req *restfile.Request, res *vars.Resolver) error {
	if req == nil || req.WebSocket == nil || res == nil {
		return nil
	}
	steps := req.WebSocket.Steps
	if len(steps) == 0 {
		return nil
	}
	for i := range steps {
		st := &steps[i]
		if v := strings.TrimSpace(st.Value); v != "" {
			out, err := res.ExpandTemplates(v)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket step value")
			}
			st.Value = out
		}
		if v := strings.TrimSpace(st.File); v != "" {
			out, err := res.ExpandTemplates(v)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket file path")
			}
			st.File = out
		}
		if v := strings.TrimSpace(st.Reason); v != "" {
			out, err := res.ExpandTemplates(v)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket close reason")
			}
			st.Reason = out
		}
	}
	req.WebSocket.Steps = steps
	return nil
}

func grpcScriptResponse(req *restfile.Request, resp *grpcclient.Response) *scripts.Response {
	if resp == nil {
		return nil
	}

	body := append([]byte(nil), resp.Body...)
	if len(body) == 0 && strings.TrimSpace(resp.Message) != "" {
		body = []byte(resp.Message)
	}
	wire := append([]byte(nil), resp.Wire...)
	wireCT := strings.TrimSpace(resp.WireContentType)
	ct := strings.TrimSpace(resp.ContentType)
	if ct == "" {
		ct = "application/json"
	}

	hdr := make(http.Header)
	for k, vs := range resp.Headers {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	for k, vs := range resp.Trailers {
		key := "Grpc-Trailer-" + k
		for _, v := range vs {
			hdr.Add(key, v)
		}
	}
	if hdr.Get("Content-Type") == "" && ct != "" {
		hdr.Set("Content-Type", ct)
	}

	status := resp.StatusCode.String()
	if msg := strings.TrimSpace(resp.StatusMessage); msg != "" && !strings.EqualFold(msg, status) {
		status = fmt.Sprintf("%s (%s)", status, msg)
	}
	target := ""
	if req != nil && req.GRPC != nil {
		target = strings.TrimSpace(req.GRPC.Target)
	}
	return &scripts.Response{
		Kind:            scripts.ResponseKindGRPC,
		Status:          status,
		Code:            int(resp.StatusCode),
		URL:             target,
		Time:            resp.Duration,
		Header:          hdr,
		Body:            body,
		Wire:            wire,
		WireContentType: wireCT,
		ContentType:     ct,
	}
}

func (e *Engine) prepareGRPCRequest(
	req *restfile.Request,
	res *vars.Resolver,
	base string,
) error {
	grpcReq := req.GRPC
	if grpcReq == nil {
		return nil
	}

	if strings.TrimSpace(grpcReq.FullMethod) == "" {
		svc := strings.TrimSpace(grpcReq.Service)
		mtd := strings.TrimSpace(grpcReq.Method)
		if svc == "" || mtd == "" {
			return errdef.New(errdef.CodeHTTP, "grpc method metadata is incomplete")
		}
		if grpcReq.Package != "" {
			grpcReq.FullMethod = "/" + grpcReq.Package + "." + svc + "/" + mtd
		} else {
			grpcReq.FullMethod = "/" + svc + "/" + mtd
		}
	}

	switch {
	case strings.TrimSpace(req.Body.Text) != "":
		grpcReq.Message = req.Body.Text
		grpcReq.MessageFile = ""
	case strings.TrimSpace(req.Body.FilePath) != "":
		grpcReq.MessageFile = req.Body.FilePath
		grpcReq.Message = ""
	}
	grpcReq.MessageExpanded = ""
	grpcReq.MessageExpandedSet = false

	if err := grpcclient.ValidateMetaPairs(grpcReq.Metadata); err != nil {
		return err
	}
	if err := grpcclient.ValidateHeaderPairs(req.Headers); err != nil {
		return err
	}

	if res != nil {
		target, err := res.ExpandTemplates(grpcReq.Target)
		if err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc target")
		}
		grpcReq.Target = strings.TrimSpace(target)

		if msg := strings.TrimSpace(grpcReq.Message); msg != "" {
			out, err := res.ExpandTemplates(msg)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc message")
			}
			grpcReq.Message = out
		}
		if req.Body.Options.ExpandTemplates && strings.TrimSpace(grpcReq.MessageFile) != "" {
			out, err := expandGRPCMessageFile(grpcReq.MessageFile, base, res)
			if err != nil {
				return err
			}
			grpcReq.MessageExpanded = out
			grpcReq.MessageExpandedSet = true
		}
		for i := range grpcReq.Metadata {
			out, err := res.ExpandTemplates(grpcReq.Metadata[i].Value)
			if err != nil {
				return errdef.Wrap(
					errdef.CodeHTTP,
					err,
					"expand grpc metadata %s",
					grpcReq.Metadata[i].Key,
				)
			}
			grpcReq.Metadata[i].Value = out
		}
		if auth := strings.TrimSpace(grpcReq.Authority); auth != "" {
			out, err := res.ExpandTemplates(auth)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc authority")
			}
			grpcReq.Authority = strings.TrimSpace(out)
		}
		if desc := strings.TrimSpace(grpcReq.DescriptorSet); desc != "" {
			out, err := res.ExpandTemplates(desc)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc descriptor set")
			}
			grpcReq.DescriptorSet = strings.TrimSpace(out)
		}
		for k, vs := range req.Headers {
			for i, v := range vs {
				out, err := res.ExpandTemplates(v)
				if err != nil {
					return errdef.Wrap(errdef.CodeHTTP, err, "expand header %s", k)
				}
				req.Headers[k][i] = out
			}
		}
	}

	grpcReq.Target = normalizeGRPCTarget(strings.TrimSpace(grpcReq.Target), grpcReq)
	if grpcReq.Target == "" {
		return errdef.New(errdef.CodeHTTP, "grpc target not specified")
	}
	req.URL = grpcReq.Target
	return nil
}

func expandGRPCMessageFile(path, base string, res *vars.Resolver) (string, error) {
	if res == nil {
		return "", nil
	}
	full := path
	if !filepath.IsAbs(full) && base != "" {
		full = filepath.Join(base, full)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "read grpc message file %s", path)
	}
	out, err := res.ExpandTemplates(string(data))
	if err != nil {
		return "", errdef.Wrap(errdef.CodeHTTP, err, "expand grpc message file")
	}
	return out, nil
}

func normalizeGRPCTarget(target string, req *restfile.GRPCRequest) string {
	val := strings.TrimSpace(target)
	if val == "" {
		return ""
	}
	low := strings.ToLower(val)
	switch {
	case strings.HasPrefix(low, "grpcs://"):
		if req != nil && !req.PlaintextSet {
			req.Plaintext = false
			req.PlaintextSet = true
		}
		return val[len("grpcs://"):]
	case strings.HasPrefix(low, "https://"):
		if req != nil && !req.PlaintextSet {
			req.Plaintext = false
			req.PlaintextSet = true
		}
		return val[len("https://"):]
	case strings.HasPrefix(low, "grpc://"):
		return val[len("grpc://"):]
	case strings.HasPrefix(low, "http://"):
		return val[len("http://"):]
	default:
		return val
	}
}

func grpcStreamInfoFromSession(sess *stream.Session) (*scripts.StreamInfo, []byte, error) {
	if sess == nil {
		return nil, nil, nil
	}
	evs := sess.EventsSnapshot()
	stats := sess.StatsSnapshot()
	st, err := sess.State()
	info := &scripts.StreamInfo{
		Kind:    "grpc",
		Summary: make(map[string]interface{}),
		Events:  make([]map[string]interface{}, 0, len(evs)),
	}

	cnt := struct {
		sent int
		recv int
	}{}
	status := ""
	reason := ""
	for _, ev := range evs {
		if ev == nil {
			continue
		}
		item := map[string]interface{}{
			"timestamp": ev.Timestamp.Format(time.RFC3339Nano),
		}
		if mtd := grpcMetaTrim(ev.Metadata, grpcclient.MetaMethod); mtd != "" {
			item["method"] = mtd
		}
		switch ev.Direction {
		case stream.DirSend:
			item["direction"] = "send"
			cnt.sent++
		case stream.DirReceive:
			item["direction"] = "receive"
			cnt.recv++
		default:
			item["direction"] = "summary"
		}
		if typ := grpcMetaTrim(ev.Metadata, grpcclient.MetaMsgType); typ != "" {
			item["messageType"] = typ
		}
		if idxText := grpcMetaTrim(ev.Metadata, grpcclient.MetaMsgIndex); idxText != "" {
			item["index"] = idxText
			if idx, convErr := strconv.Atoi(idxText); convErr == nil {
				item["indexNum"] = idx
			}
		}
		if ev.Direction == stream.DirNA {
			if code := grpcMetaTrim(ev.Metadata, grpcclient.MetaStatus); code != "" {
				item["status"] = code
				status = code
			}
			if msg := grpcMetaTrim(ev.Metadata, grpcclient.MetaReason); msg != "" {
				item["reason"] = msg
				reason = msg
			}
		} else {
			txt := strings.TrimSpace(string(ev.Payload))
			item["size"] = len(ev.Payload)
			item["text"] = txt
			if len(ev.Payload) > 0 {
				var payload any
				if json.Unmarshal(ev.Payload, &payload) == nil {
					item["json"] = payload
				}
			}
		}
		info.Events = append(info.Events, item)
	}

	if status == "" {
		switch st {
		case stream.StateClosed:
			status = "OK"
		case stream.StateFailed:
			status = "FAILED"
		default:
			status = strings.ToUpper(strings.TrimSpace(streamStateString(st, err)))
		}
	}
	if reason == "" && err != nil {
		reason = err.Error()
	}
	dur := time.Duration(0)
	if !stats.EndedAt.IsZero() {
		dur = stats.EndedAt.Sub(stats.StartedAt)
	}
	info.Summary["sentCount"] = cnt.sent
	info.Summary["receivedCount"] = cnt.recv
	info.Summary["eventCount"] = len(info.Events)
	info.Summary["duration"] = dur
	info.Summary["status"] = status
	info.Summary["reason"] = reason

	raw, encErr := json.MarshalIndent(map[string]any{
		"summary": info.Summary,
		"events":  info.Events,
	}, "", "  ")
	if encErr != nil {
		return nil, nil, encErr
	}
	return info, raw, nil
}

func grpcMetaTrim(md map[string]string, key string) string {
	if md == nil {
		return ""
	}
	return strings.TrimSpace(md[key])
}

func streamStateString(st stream.State, err error) string {
	switch st {
	case stream.StateConnecting:
		return "connecting"
	case stream.StateOpen:
		return "open"
	case stream.StateClosing:
		return "closing"
	case stream.StateClosed:
		if err != nil {
			return "closed (error)"
		}
		return "closed"
	case stream.StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}
