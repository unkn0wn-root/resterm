package grpcclient

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type Options struct {
	BaseDir             string
	DefaultPlaintext    bool
	DefaultPlaintextSet bool
	DescriptorPaths     []string
	DialTimeout         time.Duration
}

type Response struct {
	Message       string
	Headers       map[string][]string
	Trailers      map[string][]string
	StatusCode    codes.Code
	StatusMessage string
	Duration      time.Duration
}

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Execute(parent context.Context, req *restfile.Request, grpcReq *restfile.GRPCRequest, options Options) (resp *Response, err error) {
	if grpcReq == nil {
		return nil, errdef.New(errdef.CodeHTTP, "missing grpc metadata")
	}

	target := strings.TrimSpace(grpcReq.Target)
	if target == "" {
		return nil, errdef.New(errdef.CodeHTTP, "grpc target not specified")
	}

	ctx := parent
	cancel := func() {}
	if timeout := req.Settings["timeout"]; timeout != "" {
		if dur, err := time.ParseDuration(timeout); err == nil && dur > 0 {
			ctx, cancel = context.WithTimeout(parent, dur)
		}
	} else if options.DialTimeout > 0 {
		ctx, cancel = context.WithTimeout(parent, options.DialTimeout)
	}
	defer cancel()

	usePlain := shouldUsePlaintext(grpcReq, options)

	dialOpts := []grpc.DialOption{}
	if usePlain {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	}
	if grpcReq.Authority != "" {
		dialOpts = append(dialOpts, grpc.WithAuthority(grpcReq.Authority))
	}

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "dial grpc target")
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && err == nil {
			err = errdef.Wrap(errdef.CodeHTTP, closeErr, "close grpc connection")
		}
	}()

	methodDesc, err := c.resolveMethodDescriptor(ctx, conn, grpcReq, options)
	if err != nil {
		return nil, err
	}

	messageJSON, err := c.resolveMessage(grpcReq, options.BaseDir)
	if err != nil {
		return nil, err
	}

	inputMsg := dynamicpb.NewMessage(methodDesc.Input())
	stripped := strings.TrimSpace(messageJSON)
	if stripped != "" {
		if err := protojson.Unmarshal([]byte(stripped), inputMsg); err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "decode grpc request body")
		}
	}

	headerMD := metadata.MD{}
	trailerMD := metadata.MD{}

	callCtx := ctx
	if metaPairs := collectMetadata(grpcReq, req); len(metaPairs) > 0 {
		callCtx = metadata.NewOutgoingContext(callCtx, metadata.Pairs(metaPairs...))
	}

	outputMsg := dynamicpb.NewMessage(methodDesc.Output())
	start := time.Now()
	invokeErr := conn.Invoke(callCtx, grpcReq.FullMethod, inputMsg, outputMsg, grpc.Header(&headerMD), grpc.Trailer(&trailerMD))
	duration := time.Since(start)

	resp = &Response{
		Headers:       copyMetadata(headerMD),
		Trailers:      copyMetadata(trailerMD),
		StatusCode:    codes.OK,
		StatusMessage: "OK",
		Duration:      duration,
	}

	if invokeErr != nil {
		st, ok := status.FromError(invokeErr)
		if ok {
			resp.StatusCode = st.Code()
			resp.StatusMessage = st.Message()
		}
		return resp, errdef.Wrap(errdef.CodeHTTP, invokeErr, "invoke grpc method")
	}

	marshalled, err := protojson.MarshalOptions{Multiline: true, EmitUnpopulated: true}.Marshal(outputMsg)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "encode grpc response")
	}
	resp.Message = string(marshalled)
	return resp, nil
}

func (c *Client) resolveMethodDescriptor(ctx context.Context, conn *grpc.ClientConn, grpcReq *restfile.GRPCRequest, options Options) (protoreflect.MethodDescriptor, error) {
	if grpcReq.FullMethod == "" {
		return nil, errdef.New(errdef.CodeHTTP, "grpc method not specified")
	}

	if grpcReq.DescriptorSet != "" {
		set, err := c.loadDescriptorSet(grpcReq.DescriptorSet, options.BaseDir)
		if err != nil {
			return nil, err
		}
		files, err := protodesc.NewFiles(set)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "build descriptors from file")
		}
		return findMethodInFiles(files, grpcReq)
	}

	if !grpcReq.UseReflection {
		return nil, errdef.New(errdef.CodeHTTP, "grpc reflection disabled and no descriptor provided")
	}

	fds, err := fetchDescriptorsViaReflection(ctx, conn, grpcReq.FullMethod)
	if err != nil {
		return nil, err
	}

	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "build descriptors from reflection")
	}
	return findMethodInFiles(files, grpcReq)
}

func (c *Client) loadDescriptorSet(descriptorPath, baseDir string) (*descriptorpb.FileDescriptorSet, error) {
	path := descriptorPath
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeFilesystem, err, "read grpc descriptor %s", descriptorPath)
	}

	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, fds); err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse descriptor set")
	}
	return fds, nil
}

func findMethodInFiles(files *protoregistry.Files, grpcReq *restfile.GRPCRequest) (protoreflect.MethodDescriptor, error) {
	serviceName := protoreflect.FullName(grpcReq.Service)
	if grpcReq.Package != "" {
		serviceName = protoreflect.FullName(grpcReq.Package + "." + grpcReq.Service)
	}

	desc, err := files.FindDescriptorByName(serviceName)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "service %s not found", serviceName)
	}

	svcDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, errdef.New(errdef.CodeHTTP, "descriptor for %s is not a service", serviceName)
	}

	method := svcDesc.Methods().ByName(protoreflect.Name(grpcReq.Method))
	if method == nil {
		return nil, errdef.New(errdef.CodeHTTP, "method %s not found on %s", grpcReq.Method, serviceName)
	}
	return method, nil
}

func fetchDescriptorsViaReflection(ctx context.Context, conn *grpc.ClientConn, fullMethod string) (set *descriptorpb.FileDescriptorSet, err error) {
	client := reflectpb.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "open reflection stream")
	}
	defer func() {
		if closeErr := stream.CloseSend(); closeErr != nil && err == nil {
			err = errdef.Wrap(errdef.CodeHTTP, closeErr, "close reflection stream")
		}
	}()

	symbol := strings.TrimSpace(strings.TrimPrefix(fullMethod, "/"))
	if idx := strings.LastIndex(symbol, "/"); idx > 0 && idx < len(symbol)-1 {
		service := symbol[:idx]
		method := symbol[idx+1:]
		if service != "" && method != "" {
			symbol = service + "." + method
		}
	}
	request := &reflectpb.ServerReflectionRequest{
		MessageRequest: &reflectpb.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: symbol},
	}
	if err := stream.Send(request); err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "send reflection request")
	}

	response, err := stream.Recv()
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "receive reflection response")
	}

	fileResp := response.GetFileDescriptorResponse()
	if fileResp == nil {
		return nil, errdef.New(errdef.CodeHTTP, "reflection response missing descriptors")
	}

	set = &descriptorpb.FileDescriptorSet{}
	for _, raw := range fileResp.FileDescriptorProto {
		fd := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(raw, fd); err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "decode reflected descriptor")
		}
		set.File = append(set.File, fd)
	}
	return set, nil
}

func shouldUsePlaintext(grpcReq *restfile.GRPCRequest, options Options) bool {
	if grpcReq != nil && grpcReq.PlaintextSet {
		return grpcReq.Plaintext
	}
	if options.DefaultPlaintextSet {
		return options.DefaultPlaintext
	}
	return true
}

func collectMetadata(grpcReq *restfile.GRPCRequest, req *restfile.Request) []string {
	pairs := []string{}
	if grpcReq != nil && len(grpcReq.Metadata) > 0 {
		for k, v := range grpcReq.Metadata {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				continue
			}
			pairs = append(pairs, key, v)
		}
	}

	if req != nil && len(req.Headers) > 0 {
		for k, values := range req.Headers {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				continue
			}
			for _, v := range values {
				pairs = append(pairs, key, v)
			}
		}
	}
	return pairs
}

func copyMetadata(md metadata.MD) map[string][]string {
	if md == nil {
		return nil
	}

	out := make(map[string][]string, len(md))
	for k, values := range md {
		copied := make([]string, len(values))
		copy(copied, values)
		out[k] = copied
	}
	return out
}

func (c *Client) resolveMessage(grpcReq *restfile.GRPCRequest, baseDir string) (string, error) {
	if grpcReq.Message != "" {
		return grpcReq.Message, nil
	}
	if grpcReq.MessageFile == "" {
		return "", nil
	}

	path := grpcReq.MessageFile
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "read grpc message file %s", grpcReq.MessageFile)
	}
	return string(data), nil
}
