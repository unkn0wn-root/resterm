package grpcclient

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// resolves the proto descriptors required to invoke a method.
// a compiled descriptor set on disk or the server reflection API.
type descriptorSource interface {
	files(ctx context.Context, id methodID) (*protoregistry.Files, error)
}

type fileDescriptorSource struct {
	path    string
	baseDir string
}

func (s fileDescriptorSource) files(_ context.Context, _ methodID) (*protoregistry.Files, error) {
	set, err := readDescriptorSet(s.path, s.baseDir)
	if err != nil {
		return nil, err
	}
	return filesFromSet(set, "build descriptors from file")
}

type reflectionSource struct {
	conn *grpc.ClientConn
}

func (s reflectionSource) files(ctx context.Context, id methodID) (*protoregistry.Files, error) {
	set, err := fetchDescriptorsViaReflection(ctx, s.conn, id)
	if err != nil {
		return nil, err
	}
	return filesFromSet(set, "build descriptors from reflection")
}

// selects the descriptor strategy by the request:
// an explicit descriptor set takes precedence, otherwise reflection is used when
// enabled.
func descriptorSourceFor(
	conn *grpc.ClientConn,
	gr *restfile.GRPCRequest,
	opt Options,
) (descriptorSource, error) {
	if gr.DescriptorSet != "" {
		return fileDescriptorSource{path: gr.DescriptorSet, baseDir: opt.BaseDir}, nil
	}
	if !gr.UseReflection {
		return nil, diag.New(
			diag.ClassProtocol,
			"grpc reflection disabled and no descriptor provided",
		)
	}
	return reflectionSource{conn: conn}, nil
}
