package grpcclient

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func isStreaming(methodDesc protoreflect.MethodDescriptor) bool {
	return methodDesc.IsStreamingClient() || methodDesc.IsStreamingServer()
}

func streamDesc(methodDesc protoreflect.MethodDescriptor) *grpc.StreamDesc {
	return &grpc.StreamDesc{
		StreamName:    string(methodDesc.Name()),
		ClientStreams: methodDesc.IsStreamingClient(),
		ServerStreams: methodDesc.IsStreamingServer(),
	}
}

func (c *Client) executeStream(
	ctx context.Context,
	conn *grpc.ClientConn,
	req *restfile.Request,
	grpcReq *restfile.GRPCRequest,
	methodDesc protoreflect.MethodDescriptor,
	messageJSON string,
	hook StreamHook,
) (*Response, error) {
	callCtx := ctx
	if metaPairs, err := collectMetadata(grpcReq, req); err != nil {
		return nil, err
	} else if len(metaPairs) > 0 {
		callCtx = metadata.NewOutgoingContext(callCtx, metadata.Pairs(metaPairs...))
	}
	callCtx, cancel := context.WithCancel(callCtx)
	defer cancel()

	session := stream.NewSession(callCtx, stream.KindGRPC, stream.Config{})
	if hook != nil {
		hook(session)
	}

	headerMD := metadata.MD{}
	trailerMD := metadata.MD{}
	start := time.Now()
	cs, err := conn.NewStream(
		callCtx,
		streamDesc(methodDesc),
		grpcReq.FullMethod,
		grpc.Header(&headerMD),
		grpc.Trailer(&trailerMD),
	)
	if err != nil {
		finalizeStream(session, grpcReq.FullMethod, err)
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "open grpc stream")
	}
	session.MarkOpen()

	msgs, err := parseInput(messageJSON, methodDesc.Input(), methodDesc.IsStreamingClient())
	if err != nil {
		finalizeStream(session, grpcReq.FullMethod, err)
		return nil, err
	}

	out, streamErr := runStream(
		cs,
		methodDesc,
		msgs,
		grpcReq.FullMethod,
		session,
		cancel,
	)
	resp := newResponse(headerMD, trailerMD, time.Since(start))
	body, bodyErr := buildStreamBody(out)
	if bodyErr != nil {
		finalizeStream(session, grpcReq.FullMethod, bodyErr)
		return nil, errdef.Wrap(errdef.CodeHTTP, bodyErr, "encode grpc stream response")
	}
	resp.Message = string(body)
	resp.Body = body
	ensureContentType(resp)

	if streamErr != nil {
		if st, ok := status.FromError(streamErr); ok {
			resp.StatusCode = st.Code()
			resp.StatusMessage = st.Message()
		}
		finalizeStream(session, grpcReq.FullMethod, streamErr)
		return resp, errdef.Wrap(errdef.CodeHTTP, streamErr, "invoke grpc stream")
	}
	finalizeStream(session, grpcReq.FullMethod, nil)
	return resp, nil
}

func runStream(
	cs grpc.ClientStream,
	methodDesc protoreflect.MethodDescriptor,
	msgs []proto.Message,
	method string,
	session *stream.Session,
	cancel context.CancelFunc,
) ([][]byte, error) {
	inType := string(methodDesc.Input().FullName())
	outDesc := methodDesc.Output()
	switch {
	case methodDesc.IsStreamingClient() && methodDesc.IsStreamingServer():
		return runBidiStream(cs, msgs, inType, outDesc, method, session, cancel)
	case methodDesc.IsStreamingClient():
		return runClientStream(cs, msgs, inType, outDesc, method, session)
	case methodDesc.IsStreamingServer():
		return runServerStream(cs, msgs, inType, outDesc, method, session)
	default:
		return nil, errdef.New(errdef.CodeHTTP, "grpc method is not streaming")
	}
}

func runServerStream(
	cs grpc.ClientStream,
	msgs []proto.Message,
	inType string,
	outDesc protoreflect.MessageDescriptor,
	method string,
	session *stream.Session,
) ([][]byte, error) {
	if err := sendMsgs(cs, msgs, inType, method, session); err != nil {
		return nil, err
	}
	if err := cs.CloseSend(); err != nil {
		return nil, err
	}
	return recvAll(cs, outDesc, method, session)
}

func runClientStream(
	cs grpc.ClientStream,
	msgs []proto.Message,
	inType string,
	outDesc protoreflect.MessageDescriptor,
	method string,
	session *stream.Session,
) ([][]byte, error) {
	if err := sendMsgs(cs, msgs, inType, method, session); err != nil {
		return nil, err
	}
	if err := cs.CloseSend(); err != nil {
		return nil, err
	}
	return recvOne(cs, outDesc, method, session)
}

func runBidiStream(
	cs grpc.ClientStream,
	msgs []proto.Message,
	inType string,
	outDesc protoreflect.MessageDescriptor,
	method string,
	session *stream.Session,
	cancel context.CancelFunc,
) ([][]byte, error) {
	type recvResult struct {
		msgs [][]byte
		err  error
	}
	ch := make(chan recvResult, 1)
	go func() {
		out, err := recvAll(cs, outDesc, method, session)
		ch <- recvResult{msgs: out, err: err}
	}()

	if err := sendMsgs(cs, msgs, inType, method, session); err != nil {
		cancel()
		res := <-ch
		return res.msgs, err
	}
	if err := cs.CloseSend(); err != nil {
		cancel()
		res := <-ch
		return res.msgs, err
	}

	res := <-ch
	if res.err != nil {
		return res.msgs, res.err
	}
	return res.msgs, nil
}

func sendMsgs(
	cs grpc.ClientStream,
	msgs []proto.Message,
	msgType string,
	method string,
	session *stream.Session,
) error {
	for i, msg := range msgs {
		if err := cs.SendMsg(msg); err != nil {
			return err
		}
		payload, err := marshalMsg(msg)
		if err != nil {
			return err
		}
		publishMsg(session, stream.DirSend, method, msgType, i, payload)
	}
	return nil
}

func recvAll(
	cs grpc.ClientStream,
	outDesc protoreflect.MessageDescriptor,
	method string,
	session *stream.Session,
) ([][]byte, error) {
	var out [][]byte
	idx := 0
	msgType := string(outDesc.FullName())
	for {
		msg := dynamicpb.NewMessage(outDesc)
		err := cs.RecvMsg(msg)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		payload, err := marshalMsg(msg)
		if err != nil {
			return out, err
		}
		out = append(out, payload)
		publishMsg(session, stream.DirReceive, method, msgType, idx, payload)
		idx++
	}
}

func recvOne(
	cs grpc.ClientStream,
	outDesc protoreflect.MessageDescriptor,
	method string,
	session *stream.Session,
) ([][]byte, error) {
	msgType := string(outDesc.FullName())
	msg := dynamicpb.NewMessage(outDesc)
	if err := cs.RecvMsg(msg); err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, err
	}
	payload, err := marshalMsg(msg)
	if err != nil {
		return nil, err
	}
	publishMsg(session, stream.DirReceive, method, msgType, 0, payload)
	return [][]byte{payload}, nil
}

func parseInput(
	text string,
	msgDesc protoreflect.MessageDescriptor,
	clientStream bool,
) ([]proto.Message, error) {
	msgs, err := decodeMessages(text, msgDesc)
	if err != nil {
		return nil, err
	}
	if clientStream {
		return msgs, nil
	}
	if len(msgs) == 0 {
		return []proto.Message{dynamicpb.NewMessage(msgDesc)}, nil
	}
	if len(msgs) > 1 {
		return nil, errdef.New(errdef.CodeHTTP, "grpc request expects a single message")
	}
	return msgs, nil
}

func decodeMessages(
	text string,
	msgDesc protoreflect.MessageDescriptor,
) ([]proto.Message, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var raw []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "decode grpc request body")
		}
		msgs := make([]proto.Message, 0, len(raw))
		for i, item := range raw {
			msg, err := unmarshalMsg(item, msgDesc)
			if err != nil {
				return nil, errdef.Wrap(
					errdef.CodeHTTP,
					err,
					"decode grpc request body item %d",
					i,
				)
			}
			msgs = append(msgs, msg)
		}
		return msgs, nil
	}
	msg, err := unmarshalMsg([]byte(trimmed), msgDesc)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "decode grpc request body")
	}
	return []proto.Message{msg}, nil
}

func unmarshalMsg(
	data []byte,
	msgDesc protoreflect.MessageDescriptor,
) (proto.Message, error) {
	msg := dynamicpb.NewMessage(msgDesc)
	if strings.TrimSpace(string(data)) == "" {
		return msg, nil
	}
	if err := protojson.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func buildStreamBody(msgs [][]byte) ([]byte, error) {
	if len(msgs) == 0 {
		return []byte("[]"), nil
	}
	raw := make([]json.RawMessage, len(msgs))
	for i, msg := range msgs {
		raw[i] = json.RawMessage(msg)
	}
	return json.MarshalIndent(raw, "", "  ")
}

func publishMsg(
	session *stream.Session,
	dir stream.Direction,
	method string,
	msgType string,
	idx int,
	payload []byte,
) {
	if session == nil {
		return
	}
	meta := map[string]string{MetaMethod: method}
	if msgType != "" {
		meta[MetaMsgType] = msgType
	}
	if idx >= 0 {
		meta[MetaMsgIndex] = strconv.Itoa(idx)
	}
	session.Publish(&stream.Event{
		Kind:      stream.KindGRPC,
		Direction: dir,
		Metadata:  meta,
		Payload:   payload,
	})
}

func finalizeStream(session *stream.Session, method string, err error) {
	if session == nil {
		return
	}
	st := summaryStatus(err)
	publishSummary(session, method, st)
	if err != nil {
		session.Close(err)
		return
	}
	session.Close(nil)
}

func summaryStatus(err error) *status.Status {
	if err == nil {
		return status.New(codes.OK, "OK")
	}
	if st, ok := status.FromError(err); ok {
		return st
	}
	return status.New(codes.Unknown, err.Error())
}

func publishSummary(session *stream.Session, method string, st *status.Status) {
	if session == nil {
		return
	}
	meta := map[string]string{MetaMethod: method}
	if st != nil {
		code := st.Code().String()
		if code != "" {
			meta[MetaStatus] = code
		}
		msg := strings.TrimSpace(st.Message())
		if msg != "" {
			meta[MetaReason] = msg
		}
	}
	session.Publish(&stream.Event{
		Kind:      stream.KindGRPC,
		Direction: stream.DirNA,
		Metadata:  meta,
	})
}

func marshalMsg(msg proto.Message) ([]byte, error) {
	return protojson.MarshalOptions{
		Multiline:       true,
		EmitUnpopulated: true,
	}.Marshal(msg)
}

func newResponse(headerMD, trailerMD metadata.MD, dur time.Duration) *Response {
	return &Response{
		Headers:         copyMetadata(headerMD),
		Trailers:        copyMetadata(trailerMD),
		StatusCode:      codes.OK,
		StatusMessage:   "OK",
		ContentType:     "application/json",
		WireContentType: "application/grpc+proto",
		Duration:        dur,
	}
}

func ensureContentType(resp *Response) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = make(map[string][]string)
	}
	if len(resp.Headers["Content-Type"]) == 0 && strings.TrimSpace(resp.ContentType) != "" {
		resp.Headers["Content-Type"] = []string{resp.ContentType}
	}
}
