package grpcx

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	// Registers the google.rpc.* detail types in the global registry so status
	// details resolve when the request carries no descriptor for them.
	_ "google.golang.org/genproto/googleapis/rpc/errdetails"
)

// codec resolves Any payloads from request descriptors first, then falls back
// to types linked into the binary. This covers service types and standard gRPC
// status details.
type codec struct {
	types *dynamicpb.Types
}

func newCodec(files *protoregistry.Files) codec {
	if files == nil {
		return codec{}
	}
	return codec{types: dynamicpb.NewTypes(files)}
}

func (c codec) resolver() protojsonResolver {
	if c.types == nil {
		return protoregistry.GlobalTypes
	}
	return fallbackResolver{first: c.types, second: protoregistry.GlobalTypes}
}

func (c codec) marshal(msg proto.Message) ([]byte, error) {
	opts := protojson.MarshalOptions{
		Multiline:       true,
		EmitUnpopulated: true,
		Resolver:        c.resolver(),
	}
	return opts.Marshal(msg)
}

type protojsonResolver interface {
	protoregistry.MessageTypeResolver
	protoregistry.ExtensionTypeResolver
}

type fallbackResolver struct {
	first  protojsonResolver
	second protojsonResolver
}

func (r fallbackResolver) FindMessageByName(
	name protoreflect.FullName,
) (protoreflect.MessageType, error) {
	if t, err := r.first.FindMessageByName(name); err == nil {
		return t, nil
	}
	return r.second.FindMessageByName(name)
}

func (r fallbackResolver) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	if t, err := r.first.FindMessageByURL(url); err == nil {
		return t, nil
	}
	return r.second.FindMessageByURL(url)
}

func (r fallbackResolver) FindExtensionByName(
	name protoreflect.FullName,
) (protoreflect.ExtensionType, error) {
	if t, err := r.first.FindExtensionByName(name); err == nil {
		return t, nil
	}
	return r.second.FindExtensionByName(name)
}

func (r fallbackResolver) FindExtensionByNumber(
	msg protoreflect.FullName,
	field protoreflect.FieldNumber,
) (protoreflect.ExtensionType, error) {
	if t, err := r.first.FindExtensionByNumber(msg, field); err == nil {
		return t, nil
	}
	return r.second.FindExtensionByNumber(msg, field)
}

func (c codec) unmarshal(data []byte, desc protoreflect.MessageDescriptor) (proto.Message, error) {
	msg := dynamicpb.NewMessage(desc)
	if len(data) == 0 {
		return msg, nil
	}
	if err := c.unmarshalInto(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (c codec) unmarshalInto(data []byte, msg proto.Message) error {
	return protojson.UnmarshalOptions{Resolver: c.resolver()}.Unmarshal(data, msg)
}
