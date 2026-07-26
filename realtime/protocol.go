package realtime

import (
	"fmt"
)

type protocolError struct {
	code      uint32
	message   string
	temporary bool
}

type incomingKind uint8

const (
	incomingReply incomingKind = iota
	incomingPublication
	incomingPing
)

type incomingMessage struct {
	kind incomingKind
	id   uint32
	err  *protocolError
	data []byte
}

func connectCommand(id uint32, token string) []byte {
	request := []byte(nil)
	if token != "" {
		request = appendStringField(request, 1, token)
	}
	return command(id, 4, request)
}

func subscribeCommand(id uint32, channel, token string) []byte {
	request := appendStringField(nil, 1, channel)
	if token != "" {
		request = appendStringField(request, 2, token)
	}
	return command(id, 5, request)
}

func pongCommand() []byte {
	return []byte{0}
}

func command(id uint32, field uint32, request []byte) []byte {
	message := appendVarintField(nil, 1, uint64(id))
	message = appendBytesField(message, field, request)
	out := appendVarint(nil, uint64(len(message)))
	return append(out, message...)
}

func decodeReplies(frame []byte) ([]incomingMessage, error) {
	if int64(len(frame)) > MaxRealtimeMessageBytes {
		return nil, fmt.Errorf("centrifugo protobuf message exceeds %d bytes", MaxRealtimeMessageBytes)
	}
	cursor := protoCursor{data: frame}
	var incoming []incomingMessage
	for cursor.remaining() > 0 {
		length, err := cursor.varint()
		if err != nil {
			return nil, err
		}
		if length > uint64(MaxRealtimeMessageBytes) {
			return nil, fmt.Errorf("centrifugo protobuf record exceeds %d bytes", MaxRealtimeMessageBytes)
		}
		reply, err := cursor.take(int(length))
		if err != nil {
			return nil, err
		}
		if err := decodeReply(reply, &incoming); err != nil {
			return nil, err
		}
	}
	return incoming, nil
}

func decodeReply(data []byte, incoming *[]incomingMessage) error {
	if len(data) == 0 {
		*incoming = append(*incoming, incomingMessage{kind: incomingPing})
		return nil
	}
	cursor := protoCursor{data: data}
	var id uint32
	var replyErr *protocolError
	sawPush := false
	for cursor.remaining() > 0 {
		field, wire, err := cursor.key()
		if err != nil {
			return err
		}
		switch {
		case field == 1 && wire == 0:
			value, err := cursor.varint()
			if err != nil {
				return err
			}
			if value > uint64(^uint32(0)) {
				return fmt.Errorf("centrifugo reply id exceeds uint32")
			}
			id = uint32(value)
		case field == 2 && wire == 2:
			value, err := cursor.lengthDelimited()
			if err != nil {
				return err
			}
			replyErr, err = decodeProtocolError(value)
			if err != nil {
				return err
			}
		case field == 4 && wire == 2:
			sawPush = true
			value, err := cursor.lengthDelimited()
			if err != nil {
				return err
			}
			if err := decodePush(value, incoming); err != nil {
				return err
			}
		default:
			if err := cursor.skip(wire); err != nil {
				return err
			}
		}
	}
	if id != 0 || replyErr != nil || !sawPush {
		*incoming = append(*incoming, incomingMessage{kind: incomingReply, id: id, err: replyErr})
	}
	return nil
}

func decodeProtocolError(data []byte) (*protocolError, error) {
	cursor := protoCursor{data: data}
	out := &protocolError{}
	for cursor.remaining() > 0 {
		field, wire, err := cursor.key()
		if err != nil {
			return nil, err
		}
		switch {
		case field == 1 && wire == 0:
			value, err := cursor.varint()
			if err != nil {
				return nil, err
			}
			if value > uint64(^uint32(0)) {
				return nil, fmt.Errorf("centrifugo error code exceeds uint32")
			}
			out.code = uint32(value)
		case field == 2 && wire == 2:
			value, err := cursor.lengthDelimited()
			if err != nil {
				return nil, err
			}
			out.message = string(value)
		case field == 3 && wire == 0:
			value, err := cursor.varint()
			if err != nil {
				return nil, err
			}
			out.temporary = value != 0
		default:
			if err := cursor.skip(wire); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func decodePush(data []byte, incoming *[]incomingMessage) error {
	cursor := protoCursor{data: data}
	for cursor.remaining() > 0 {
		field, wire, err := cursor.key()
		if err != nil {
			return err
		}
		if field == 4 && wire == 2 {
			value, err := cursor.lengthDelimited()
			if err != nil {
				return err
			}
			if err := decodePublication(value, incoming); err != nil {
				return err
			}
		} else if err := cursor.skip(wire); err != nil {
			return err
		}
	}
	return nil
}

func decodePublication(data []byte, incoming *[]incomingMessage) error {
	cursor := protoCursor{data: data}
	for cursor.remaining() > 0 {
		field, wire, err := cursor.key()
		if err != nil {
			return err
		}
		if field == 4 && wire == 2 {
			value, err := cursor.lengthDelimited()
			if err != nil {
				return err
			}
			payload := append([]byte(nil), value...)
			*incoming = append(*incoming, incomingMessage{kind: incomingPublication, data: payload})
		} else if err := cursor.skip(wire); err != nil {
			return err
		}
	}
	return nil
}

func appendVarintField(out []byte, field uint32, value uint64) []byte {
	out = appendVarint(out, uint64(field<<3))
	return appendVarint(out, value)
}

func appendStringField(out []byte, field uint32, value string) []byte {
	return appendBytesField(out, field, []byte(value))
}

func appendBytesField(out []byte, field uint32, value []byte) []byte {
	out = appendVarint(out, uint64(field<<3|2))
	out = appendVarint(out, uint64(len(value)))
	return append(out, value...)
}

func appendVarint(out []byte, value uint64) []byte {
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}

type protoCursor struct {
	data   []byte
	offset int
}

func (c *protoCursor) remaining() int {
	return len(c.data) - c.offset
}

func (c *protoCursor) varint() (uint64, error) {
	var value uint64
	for shift := 0; shift < 70; shift += 7 {
		if c.remaining() == 0 {
			return 0, fmt.Errorf("truncated centrifugo protobuf")
		}
		current := c.data[c.offset]
		c.offset++
		if shift == 63 && current > 1 {
			return 0, fmt.Errorf("centrifugo protobuf varint overflow")
		}
		value |= uint64(current&0x7f) << shift
		if current&0x80 == 0 {
			return value, nil
		}
	}
	return 0, fmt.Errorf("centrifugo protobuf varint overflow")
}

func (c *protoCursor) key() (uint32, byte, error) {
	key, err := c.varint()
	if err != nil {
		return 0, 0, err
	}
	field := uint32(key >> 3)
	if field == 0 {
		return 0, 0, fmt.Errorf("centrifugo protobuf field number is zero")
	}
	return field, byte(key & 7), nil
}

func (c *protoCursor) take(length int) ([]byte, error) {
	if length < 0 || c.offset+length > len(c.data) {
		return nil, fmt.Errorf("truncated centrifugo protobuf")
	}
	value := c.data[c.offset : c.offset+length]
	c.offset += length
	return value, nil
}

func (c *protoCursor) lengthDelimited() ([]byte, error) {
	length, err := c.varint()
	if err != nil {
		return nil, err
	}
	if length > uint64(MaxRealtimeMessageBytes) {
		return nil, fmt.Errorf("centrifugo protobuf field exceeds %d bytes", MaxRealtimeMessageBytes)
	}
	if length > uint64(c.remaining()) {
		return nil, fmt.Errorf("truncated centrifugo protobuf")
	}
	return c.take(int(length))
}

func (c *protoCursor) skip(wire byte) error {
	switch wire {
	case 0:
		_, err := c.varint()
		return err
	case 1:
		_, err := c.take(8)
		return err
	case 2:
		_, err := c.lengthDelimited()
		return err
	case 5:
		_, err := c.take(4)
		return err
	default:
		return fmt.Errorf("unsupported centrifugo protobuf wire type %d", wire)
	}
}
