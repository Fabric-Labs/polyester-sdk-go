package realtime

import (
	"bytes"
	"testing"
)

func TestProtocolCommandsAreLengthDelimitedProtobuf(t *testing.T) {
	if got, want := connectCommand(1, ""), []byte{4, 8, 1, 34, 0}; !bytes.Equal(got, want) {
		t.Fatalf("connect command = %v, want %v", got, want)
	}
	if got, want := subscribeCommand(2, "x", ""), []byte{7, 8, 2, 42, 3, 10, 1, 'x'}; !bytes.Equal(got, want) {
		t.Fatalf("subscribe command = %v, want %v", got, want)
	}
	if got := pongCommand(); !bytes.Equal(got, []byte{0}) {
		t.Fatalf("pong command = %v", got)
	}
}

func TestDecodeRepliesHandlesAckPingAndPublicationBatch(t *testing.T) {
	frame := []byte{
		2, 8, 1,
		0,
		9, 34, 7, 34, 5, 34, 3, 1, 2, 3,
	}
	messages, err := decodeReplies(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("got %d messages", len(messages))
	}
	if messages[0].kind != incomingReply || messages[0].id != 1 || messages[0].err != nil {
		t.Fatalf("unexpected ack: %#v", messages[0])
	}
	if messages[1].kind != incomingPing {
		t.Fatalf("unexpected ping: %#v", messages[1])
	}
	if messages[2].kind != incomingPublication || !bytes.Equal(messages[2].data, []byte{1, 2, 3}) {
		t.Fatalf("unexpected publication: %#v", messages[2])
	}
}

func TestDecodeRepliesRejectsTruncatedFrame(t *testing.T) {
	if _, err := decodeReplies([]byte{5, 8, 1}); err == nil {
		t.Fatal("expected truncated frame error")
	}
}
