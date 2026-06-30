package wire

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

var protoUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// MessageFromMap populates a protobuf message from a JSON-compatible map.
func MessageFromMap(msg proto.Message, value map[string]any) error {
	if value == nil {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return protoUnmarshal.Unmarshal(b, msg)
}

// StructFromMap builds a protobuf Struct from a map.
func StructFromMap(value map[string]any) (*structpb.Struct, error) {
	if value == nil {
		return structpb.NewStruct(nil)
	}
	return structpb.NewStruct(value)
}
