package grpcx

import (
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type streamKind uint8

const (
	callUnary streamKind = iota
	callClientStream
	callServerStream
	callBidiStream
)

func streamKindOf(md protoreflect.MethodDescriptor) streamKind {
	switch {
	case md.IsStreamingClient() && md.IsStreamingServer():
		return callBidiStream
	case md.IsStreamingClient():
		return callClientStream
	case md.IsStreamingServer():
		return callServerStream
	default:
		return callUnary
	}
}

func (k streamKind) sendsStream() bool {
	return k == callClientStream || k == callBidiStream
}

func (k streamKind) recvsStream() bool {
	return k == callServerStream || k == callBidiStream
}

func (in invocation) streamDesc() *grpc.StreamDesc {
	return &grpc.StreamDesc{
		StreamName:    string(in.method.Name()),
		ClientStreams: in.kind.sendsStream(),
		ServerStreams: in.kind.recvsStream(),
	}
}

func (in invocation) stream(ctx context.Context, hook StreamHook) (*Response, error) {
	session := stream.NewSession(in.md.attach(ctx), stream.KindGRPC, stream.Config{})
	defer session.Cancel()
	if hook != nil {
		hook(session)
	}
	// Use the session context so session.Cancel stops the RPC.
	callCtx := session.Context()

	msgs, err := parseInput(in.body, in.method.Input(), in.kind.sendsStream(), in.codec)
	if err != nil {
		finalizeStream(session, in.name, err)
		return nil, err
	}

	var md mdCapture
	start := time.Now()
	cs, err := in.conn.NewStream(callCtx, in.streamDesc(), in.name, in.callOpts(md.opts()...)...)
	if err != nil {
		finalizeStream(session, in.name, err)
		return nil, diag.WrapAs(diag.ClassProtocol, err, "open grpc stream", grpcComponent)
	}
	session.MarkOpen()

	sc := streamCall{
		cs:      cs,
		kind:    in.kind,
		desc:    in.method,
		method:  in.name,
		session: session,
		cd:      in.codec,
	}
	out, streamErr := runStream(sc, msgs, session.Cancel)
	resp := md.response(start)
	// Streams have no single wire payload, so the raw pane uses the JSON transcript.
	resp.WireContentType = ""
	bodyData, bodyErr := buildStreamBody(out)
	if bodyErr != nil {
		finalizeStream(session, in.name, bodyErr)
		return resp, diag.WrapAs(
			diag.ClassProtocol,
			bodyErr,
			"encode grpc stream response",
			grpcComponent,
		)
	}
	resp.Message = string(bodyData)
	resp.Body = bodyData

	if streamErr != nil {
		setResponseStatus(resp, streamErr, in.codec)
		finalizeStream(session, in.name, streamErr)
		return resp, diag.WrapAs(diag.ClassProtocol, streamErr, "invoke grpc stream", grpcComponent)
	}
	finalizeStream(session, in.name, nil)
	return resp, nil
}

type streamCall struct {
	cs      grpc.ClientStream
	kind    streamKind
	desc    protoreflect.MethodDescriptor
	method  string
	session *stream.Session
	cd      codec
}

func runStream(
	sc streamCall,
	msgs []proto.Message,
	cancel context.CancelFunc,
) ([][]byte, error) {
	if sc.kind == callBidiStream {
		return runBidiStream(sc, msgs, cancel)
	}
	if err := sc.send(msgs); err != nil {
		return nil, err
	}
	if err := sc.cs.CloseSend(); err != nil {
		return nil, err
	}
	return sc.recvAll()
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
	msgType := string(sc.desc.Input().FullName())
	for i, msg := range msgs {
		if err := sc.cs.SendMsg(msg); err != nil {
			return err
		}
		payload, err := sc.cd.marshal(msg)
		if err != nil {
			return err
		}
		sc.publishMsg(stream.DirSend, msgType, i, payload)
	}
	return nil
}

func (sc streamCall) recvAll() ([][]byte, error) {
	var out [][]byte
	outDesc := sc.desc.Output()
	msgType := string(outDesc.FullName())
	for i := 0; ; i++ {
		msg := dynamicpb.NewMessage(outDesc)
		err := sc.cs.RecvMsg(msg)
		if errors.Is(err, io.EOF) {
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
		sc.publishMsg(stream.DirReceive, msgType, i, payload)
	}
}

func (sc streamCall) publishMsg(dir stream.Direction, msgType string, idx int, payload []byte) {
	sc.session.Publish(&stream.Event{
		Kind:      stream.KindGRPC,
		Direction: dir,
		Metadata: map[string]string{
			MetaMethod:   sc.method,
			MetaMsgType:  msgType,
			MetaMsgIndex: strconv.Itoa(idx),
		},
		Payload: payload,
	})
}

func finalizeStream(session *stream.Session, method string, err error) {
	st := status.New(codes.OK, statusTextOK)
	if err != nil {
		st = status.Convert(err)
	}

	meta := map[string]string{MetaMethod: method, MetaStatus: st.Code().String()}
	if st.Message() != "" {
		meta[MetaReason] = st.Message()
	}
	session.Publish(&stream.Event{
		Kind:      stream.KindGRPC,
		Direction: stream.DirNA,
		Metadata:  meta,
	})
	session.Close(err)
}
