package grpcclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

type codec struct {
	types *dynamicpb.Types
}

func newCodec(files *protoregistry.Files) codec {
	if files == nil {
		return codec{}
	}
	return codec{types: dynamicpb.NewTypes(files)}
}

func (c codec) marshal(msg proto.Message) ([]byte, error) {
	opts := protojson.MarshalOptions{
		Multiline:       true,
		EmitUnpopulated: true,
	}
	if c.types != nil {
		opts.Resolver = c.types
	}
	return opts.Marshal(msg)
}

func (c codec) unmarshal(data []byte, desc protoreflect.MessageDescriptor) (proto.Message, error) {
	msg := dynamicpb.NewMessage(desc)
	if strings.TrimSpace(string(data)) == "" {
		return msg, nil
	}
	if err := c.unmarshalInto(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (c codec) unmarshalInto(data []byte, msg proto.Message) error {
	opts := protojson.UnmarshalOptions{}
	if c.types != nil {
		opts.Resolver = c.types
	}
	return opts.Unmarshal(data, msg)
}

func (c *Client) resolveMessage(gr *restfile.GRPCRequest, baseDir string) (string, error) {
	if gr.MessageExpandedSet {
		return gr.MessageExpanded, nil
	}
	if gr.Message != "" {
		return gr.Message, nil
	}
	if gr.MessageFile == "" {
		return "", nil
	}

	path := gr.MessageFile
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", diag.WrapAsf(
			diag.ClassFilesystem,
			err,
			"read grpc message file %s",
			gr.MessageFile,
		)
	}
	return string(data), nil
}

func parseInput(
	text string,
	desc protoreflect.MessageDescriptor,
	clientStream bool,
	cd codec,
) ([]proto.Message, error) {
	msgs, err := decodeMessages(text, desc, cd)
	if err != nil {
		return nil, err
	}
	if clientStream {
		return msgs, nil
	}
	if len(msgs) == 0 {
		return []proto.Message{dynamicpb.NewMessage(desc)}, nil
	}
	if len(msgs) > 1 {
		return nil, diag.New(diag.ClassProtocol, "grpc request expects a single message")
	}
	return msgs, nil
}

func decodeMessages(
	text string,
	desc protoreflect.MessageDescriptor,
	cd codec,
) ([]proto.Message, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if strings.HasPrefix(text, "[") {
		var raw []json.RawMessage
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			return nil, diag.WrapAs(diag.ClassProtocol, err, "decode grpc request body")
		}
		msgs := make([]proto.Message, 0, len(raw))
		for i, item := range raw {
			msg, err := cd.unmarshal(item, desc)
			if err != nil {
				return nil, diag.WrapAsf(
					diag.ClassProtocol,
					err,
					"decode grpc request body item %d",
					i,
				)
			}
			msgs = append(msgs, msg)
		}
		return msgs, nil
	}

	msg, err := cd.unmarshal([]byte(text), desc)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "decode grpc request body")
	}
	return []proto.Message{msg}, nil
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
