package hardening

// CentrifugoOKReply encodes a Centrifugo protobuf Reply with id and no error,
// length-delimited for the websocket frame payload.
func CentrifugoOKReply(id uint32) []byte {
	message := []byte{1 << 3} // field 1, varint
	message = appendVarint(message, uint64(id))
	return lengthDelimit(message)
}

// CentrifugoPublication encodes a Centrifugo push carrying publication data.
// Wire shape matches realtime.decodeReplies (Reply.push.pub.data).
func CentrifugoPublication(data []byte) []byte {
	// Publication { data = field 4 }
	pub := appendBytesField(nil, 4, data)
	// Push { pub = field 4 }
	push := appendBytesField(nil, 4, pub)
	// Reply { push = field 4 }
	reply := appendBytesField(nil, 4, push)
	return lengthDelimit(reply)
}

func appendBytesField(out []byte, field uint32, value []byte) []byte {
	out = appendVarint(out, uint64(field<<3|2))
	out = appendVarint(out, uint64(len(value)))
	return append(out, value...)
}

func appendVarint(buf []byte, value uint64) []byte {
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if value == 0 {
			break
		}
	}
	return buf
}

func lengthDelimit(message []byte) []byte {
	out := appendVarint(nil, uint64(len(message)))
	return append(out, message...)
}
