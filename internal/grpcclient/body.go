package grpcclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func resolveMessage(gr *restfile.GRPCRequest, rd reader) (string, error) {
	if msg, ok := gr.MessageExpanded.Get(); ok {
		return msg, nil
	}
	if gr.Message != "" {
		return gr.Message, nil
	}
	if gr.MessageFile == "" {
		return "", nil
	}

	data, err := rd.read(gr.MessageFile, "grpc message file")
	if err != nil {
		return "", err
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
		return nil, diag.New(
			diag.ClassProtocol,
			"grpc request expects a single message",
			grpcComponent,
		)
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
		return decodeMessageArray(text, desc, cd)
	}

	msg, err := cd.unmarshal([]byte(text), desc)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "decode grpc request body", grpcComponent)
	}
	return []proto.Message{msg}, nil
}

func decodeMessageArray(
	text string,
	desc protoreflect.MessageDescriptor,
	cd codec,
) ([]proto.Message, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "decode grpc request body", grpcComponent)
	}

	msgs := make([]proto.Message, 0, len(raw))
	for i, item := range raw {
		msg, err := cd.unmarshal(item, desc)
		if err != nil {
			return nil, diag.WrapAs(
				diag.ClassProtocol,
				err,
				fmt.Sprintf("decode grpc request body item %d", i),
				grpcComponent,
			)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
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
