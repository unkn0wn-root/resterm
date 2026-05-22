package grpcclient

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func isStreaming(md protoreflect.MethodDescriptor) bool {
	return md.IsStreamingClient() || md.IsStreamingServer()
}

func streamDesc(md protoreflect.MethodDescriptor) *grpc.StreamDesc {
	return &grpc.StreamDesc{
		StreamName:    string(md.Name()),
		ClientStreams: md.IsStreamingClient(),
		ServerStreams: md.IsStreamingServer(),
	}
}

func (c *Client) executeStream(
	ctx context.Context,
	conn *grpc.ClientConn,
	req *restfile.Request,
	gr *restfile.GRPCRequest,
	md protoreflect.MethodDescriptor,
	body string,
	cd codec,
	hook StreamHook,
) (*Response, error) {
	callCtx, err := outgoingContext(ctx, gr, req)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithCancel(callCtx)
	defer cancel()

	session := stream.NewSession(callCtx, stream.KindGRPC, stream.Config{})
	if hook != nil {
		hook(session)
	}

	msgs, err := parseInput(body, md.Input(), md.IsStreamingClient(), cd)
	if err != nil {
		finalizeStream(session, gr.FullMethod, err)
		return nil, err
	}

	headerMD := metadata.MD{}
	trailerMD := metadata.MD{}
	start := time.Now()
	cs, err := conn.NewStream(
		callCtx,
		streamDesc(md),
		gr.FullMethod,
		grpc.Header(&headerMD),
		grpc.Trailer(&trailerMD),
	)
	if err != nil {
		finalizeStream(session, gr.FullMethod, err)
		return nil, diag.WrapAs(diag.ClassProtocol, err, "open grpc stream")
	}
	session.MarkOpen()

	sc := streamCall{
		cs:      cs,
		md:      md,
		method:  gr.FullMethod,
		session: session,
		cd:      cd,
	}
	out, streamErr := runStream(sc, msgs, cancel)
	resp := newResponse(headerMD, trailerMD, time.Since(start))
	bodyData, bodyErr := buildStreamBody(out)
	if bodyErr != nil {
		finalizeStream(session, gr.FullMethod, bodyErr)
		return nil, diag.WrapAs(diag.ClassProtocol, bodyErr, "encode grpc stream response")
	}
	resp.Message = string(bodyData)
	resp.Body = bodyData
	ensureContentType(resp)

	if streamErr != nil {
		setResponseStatus(resp, streamErr)
		finalizeStream(session, gr.FullMethod, streamErr)
		return resp, diag.WrapAs(diag.ClassProtocol, streamErr, "invoke grpc stream")
	}
	finalizeStream(session, gr.FullMethod, nil)
	return resp, nil
}

type streamCall struct {
	cs      grpc.ClientStream
	md      protoreflect.MethodDescriptor
	method  string
	session *stream.Session
	cd      codec
}

func runStream(
	sc streamCall,
	msgs []proto.Message,
	cancel context.CancelFunc,
) ([][]byte, error) {
	switch {
	case sc.md.IsStreamingClient() && sc.md.IsStreamingServer():
		return runBidiStream(sc, msgs, cancel)
	case sc.md.IsStreamingClient():
		return runClientStream(sc, msgs)
	case sc.md.IsStreamingServer():
		return runServerStream(sc, msgs)
	default:
		return nil, diag.New(diag.ClassProtocol, "grpc method is not streaming")
	}
}

func runServerStream(sc streamCall, msgs []proto.Message) ([][]byte, error) {
	if err := sc.send(msgs); err != nil {
		return nil, err
	}
	if err := sc.cs.CloseSend(); err != nil {
		return nil, err
	}
	return sc.recvAll()
}

func runClientStream(sc streamCall, msgs []proto.Message) ([][]byte, error) {
	if err := sc.send(msgs); err != nil {
		return nil, err
	}
	if err := sc.cs.CloseSend(); err != nil {
		return nil, err
	}
	return sc.recvOne()
}

func runBidiStream(
	sc streamCall,
	msgs []proto.Message,
	cancel context.CancelFunc,
) ([][]byte, error) {
	type recvResult struct {
		msgs [][]byte
		err  error
	}
	ch := make(chan recvResult, 1)
	go func() {
		out, err := sc.recvAll()
		ch <- recvResult{msgs: out, err: err}
	}()

	if err := sc.send(msgs); err != nil {
		cancel()
		res := <-ch
		return res.msgs, err
	}
	if err := sc.cs.CloseSend(); err != nil {
		cancel()
		res := <-ch
		return res.msgs, err
	}

	res := <-ch
	return res.msgs, res.err
}

func (sc streamCall) send(msgs []proto.Message) error {
	msgType := string(sc.md.Input().FullName())
	for i, msg := range msgs {
		if err := sc.cs.SendMsg(msg); err != nil {
			return err
		}
		payload, err := sc.cd.marshal(msg)
		if err != nil {
			return err
		}
		publishMsg(sc.session, stream.DirSend, sc.method, msgType, i, payload)
	}
	return nil
}

func (sc streamCall) recvAll() ([][]byte, error) {
	var out [][]byte
	outDesc := sc.md.Output()
	msgType := string(outDesc.FullName())
	for i := 0; ; i++ {
		msg := dynamicpb.NewMessage(outDesc)
		err := sc.cs.RecvMsg(msg)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		payload, err := sc.cd.marshal(msg)
		if err != nil {
			return out, err
		}
		out = append(out, payload)
		publishMsg(sc.session, stream.DirReceive, sc.method, msgType, i, payload)
	}
}

func (sc streamCall) recvOne() ([][]byte, error) {
	outDesc := sc.md.Output()
	msg := dynamicpb.NewMessage(outDesc)
	if err := sc.cs.RecvMsg(msg); err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, err
	}
	payload, err := sc.cd.marshal(msg)
	if err != nil {
		return nil, err
	}
	publishMsg(sc.session, stream.DirReceive, sc.method, string(outDesc.FullName()), 0, payload)
	return [][]byte{payload}, nil
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
	publishSummary(session, method, summaryStatus(err))
	session.Close(err)
}

func summaryStatus(err error) *status.Status {
	if err == nil {
		return status.New(codes.OK, "OK")
	}
	return status.Convert(err)
}

func publishSummary(session *stream.Session, method string, st *status.Status) {
	if session == nil {
		return
	}
	meta := map[string]string{MetaMethod: method}
	if st != nil {
		if code := st.Code().String(); code != "" {
			meta[MetaStatus] = code
		}
		if st.Message() != "" {
			meta[MetaReason] = st.Message()
		}
	}
	session.Publish(&stream.Event{
		Kind:      stream.KindGRPC,
		Direction: stream.DirNA,
		Metadata:  meta,
	})
}
