package grpcclient

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflectv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// partialReflectServer returns one file per request to exercise dependency walking.
type partialReflectServer struct {
	reflectv1.UnimplementedServerReflectionServer
	files    map[string]*descriptorpb.FileDescriptorProto
	rootFile string
	asked    []string
}

func (s *partialReflectServer) ServerReflectionInfo(
	stream reflectv1.ServerReflection_ServerReflectionInfoServer,
) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return nil
		}

		var name string
		switch r := req.MessageRequest.(type) {
		case *reflectv1.ServerReflectionRequest_FileContainingSymbol:
			s.asked = append(s.asked, "symbol:"+r.FileContainingSymbol)
			name = s.rootFile
		case *reflectv1.ServerReflectionRequest_FileByFilename:
			s.asked = append(s.asked, "file:"+r.FileByFilename)
			name = r.FileByFilename
		default:
			return nil
		}

		fd, ok := s.files[name]
		if !ok {
			return nil
		}
		raw, err := proto.Marshal(fd)
		if err != nil {
			return err
		}
		if err := stream.Send(&reflectv1.ServerReflectionResponse{
			MessageResponse: &reflectv1.ServerReflectionResponse_FileDescriptorResponse{
				FileDescriptorResponse: &reflectv1.FileDescriptorResponse{
					FileDescriptorProto: [][]byte{raw},
				},
			},
		}); err != nil {
			return err
		}
	}
}

func startPartialReflectServer(t *testing.T, srv *partialReflectServer) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	reflectv1.RegisterServerReflectionServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})
	return lis.Addr().String()
}

func TestReflectionFetchesMissingDependencies(t *testing.T) {
	svc := testSvcDescriptorSet().File[0]
	svc.Dependency = []string{"dep.proto"}
	dep := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("dep.proto"),
		Package: proto.String("dep"),
		Syntax:  proto.String("proto3"),
	}

	srv := &partialReflectServer{
		files:    map[string]*descriptorpb.FileDescriptorProto{"svc.proto": svc, "dep.proto": dep},
		rootFile: "svc.proto",
	}
	addr := startPartialReflectServer(t, srv)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	id, err := parseFullMethod("/pkg.Svc/Call")
	if err != nil {
		t.Fatalf("parse method: %v", err)
	}
	set, err := fetchDescriptorsViaReflection(context.Background(), conn, id)
	if err != nil {
		t.Fatalf("fetch descriptors: %v", err)
	}

	names := make([]string, 0, len(set.File))
	for _, fd := range set.File {
		names = append(names, fd.GetName())
	}
	if len(names) != 2 {
		t.Fatalf("files = %v, want svc.proto and dep.proto", names)
	}

	if _, err := filesFromSet(set, "build descriptors from reflection"); err != nil {
		t.Fatalf("build descriptors: %v", err)
	}
}

func TestReflectionDedupesFilesByName(t *testing.T) {
	svc := testSvcDescriptorSet().File[0]
	svc.Dependency = []string{"svc.proto"}

	srv := &partialReflectServer{
		files:    map[string]*descriptorpb.FileDescriptorProto{"svc.proto": svc},
		rootFile: "svc.proto",
	}
	addr := startPartialReflectServer(t, srv)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	id, err := parseFullMethod("/pkg.Svc/Call")
	if err != nil {
		t.Fatalf("parse method: %v", err)
	}
	set, err := fetchDescriptorsViaReflection(context.Background(), conn, id)
	if err != nil {
		t.Fatalf("fetch descriptors: %v", err)
	}
	if len(set.File) != 1 {
		t.Fatalf("files = %d, want the duplicate collapsed", len(set.File))
	}
	if len(srv.asked) != 1 {
		t.Fatalf("asked = %v, want no follow up for a file already held", srv.asked)
	}
}
