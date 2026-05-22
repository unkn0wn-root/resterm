package grpcclient

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestCodecUsesDescriptorResolverForAny(t *testing.T) {
	files, err := filesFromSet(testAnyDescriptorSet(), "build test descriptors")
	if err != nil {
		t.Fatalf("build descriptors: %v", err)
	}

	desc := msgDesc(t, files, "pkg.Wrap")
	msg, err := newCodec(files).unmarshal(
		[]byte(`{"item":{"@type":"type.googleapis.com/pkg.Inner","name":"ok"}}`),
		desc,
	)
	if err != nil {
		t.Fatalf("unmarshal any: %v", err)
	}

	out, err := newCodec(files).marshal(msg)
	if err != nil {
		t.Fatalf("marshal any: %v", err)
	}
	if !strings.Contains(string(out), "type.googleapis.com/pkg.Inner") {
		t.Fatalf("expected any type URL in output, got %s", out)
	}
}

func msgDesc(
	t *testing.T,
	files *protoregistry.Files,
	name protoreflect.FullName,
) protoreflect.MessageDescriptor {
	t.Helper()

	desc, err := files.FindDescriptorByName(name)
	if err != nil {
		t.Fatalf("find descriptor: %v", err)
	}
	msg, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("%s is not a message", name)
	}
	return msg
}

func testAnyDescriptorSet() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			protoreflectFile(anypb.File_google_protobuf_any_proto),
			{
				Name:       proto.String("any_test.proto"),
				Package:    proto.String("pkg"),
				Syntax:     proto.String("proto3"),
				Dependency: []string{"google/protobuf/any.proto"},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Inner"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("name"),
								JsonName: proto.String("name"),
								Number:   proto.Int32(1),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
							},
						},
					},
					{
						Name: proto.String("Wrap"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("item"),
								JsonName: proto.String("item"),
								Number:   proto.Int32(1),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: proto.String(".google.protobuf.Any"),
							},
						},
					},
				},
			},
		},
	}
}

func protoreflectFile(fd protoreflect.FileDescriptor) *descriptorpb.FileDescriptorProto {
	return protodesc.ToFileDescriptorProto(fd)
}
