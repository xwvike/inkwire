package gicisky

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

type fakeTransport struct {
	notifications chan []byte
	stageOne      []byte
	payloadLength int
	dataWrites    [][]byte
	onData        func(*fakeTransport, []byte)
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		notifications: make(chan []byte, 16),
		stageOne:      []byte{0x01, 0x0c, 0x00},
	}
}

func (f *fakeTransport) Notifications() <-chan []byte {
	return f.notifications
}

func (f *fakeTransport) WriteControl(command []byte) error {
	switch {
	case bytes.Equal(command, []byte{0x01}):
		f.notify(f.stageOne)
	case len(command) == 8 && command[0] == 0x02:
		f.payloadLength = int(binary.LittleEndian.Uint32(command[1:5]))
		f.notify([]byte{0x02, 0x00, 0x00})
	case bytes.Equal(command, []byte{0x03}):
		f.notify(partACK(0))
	default:
		return errors.New("unexpected control command")
	}
	return nil
}

func (f *fakeTransport) WriteData(data []byte) error {
	copyOfData := append([]byte(nil), data...)
	f.dataWrites = append(f.dataWrites, copyOfData)
	if f.onData != nil {
		f.onData(f, copyOfData)
		return nil
	}

	sent := 0
	for _, write := range f.dataWrites {
		sent += len(write) - 4
	}
	if sent == f.payloadLength {
		f.notify([]byte{0x05, 0x08, 0x00, 0x00, 0x00, 0x00})
	} else {
		part := binary.LittleEndian.Uint32(data[:4])
		f.notify(partACK(part + 1))
	}
	return nil
}

func (f *fakeTransport) notify(data []byte) {
	f.notifications <- append([]byte(nil), data...)
}

func partACK(part uint32) []byte {
	ack := []byte{0x05, 0x00, 0x00, 0x00, 0x00, 0x00}
	binary.LittleEndian.PutUint32(ack[2:], part)
	return ack
}

func testUploader() Uploader {
	uploader := NewUploader(nil)
	uploader.NotifyReadyDelay = 0
	uploader.NotifyProcessDelay = 0
	uploader.ResponseTimeout = 100 * time.Millisecond
	return uploader
}

func TestUploadCompletes(t *testing.T) {
	payload := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}
	transport := newFakeTransport()

	if err := testUploader().Upload(context.Background(), transport, payload); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if transport.payloadLength != len(payload) {
		t.Fatalf("payload length = %d, want %d", transport.payloadLength, len(payload))
	}
	if len(transport.dataWrites) != 3 {
		t.Fatalf("data writes = %d, want 3", len(transport.dataWrites))
	}
	for index, write := range transport.dataWrites {
		if part := binary.LittleEndian.Uint32(write[:4]); part != uint32(index) {
			t.Errorf("write %d part = %d", index, part)
		}
		start := index * 8
		end := start + 8
		if end > len(payload) {
			end = len(payload)
		}
		if !bytes.Equal(write[4:], payload[start:end]) {
			t.Errorf("write %d payload = %x, want %x", index, write[4:], payload[start:end])
		}
	}
}

func TestUploadResendsLastPartOnACKMismatch(t *testing.T) {
	payload := []byte("0123456789")
	transport := newFakeTransport()
	transport.onData = func(fake *fakeTransport, data []byte) {
		part := binary.LittleEndian.Uint32(data[:4])
		switch len(fake.dataWrites) {
		case 1:
			fake.notify(partACK(0))
		case 2:
			fake.notify(partACK(1))
		default:
			if part != 1 {
				t.Fatalf("last part = %d, want 1", part)
			}
			fake.notify([]byte{0x05, 0x08})
		}
	}

	if err := testUploader().Upload(context.Background(), transport, payload); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if len(transport.dataWrites) != 3 {
		t.Fatalf("data writes = %d, want 3", len(transport.dataWrites))
	}
	if !bytes.Equal(transport.dataWrites[0], transport.dataWrites[1]) {
		t.Fatal("mismatched ACK did not resend the previous part")
	}
}

func TestUploadRejectsTruncatedStageOne(t *testing.T) {
	transport := newFakeTransport()
	transport.stageOne = []byte{0x01}

	err := testUploader().Upload(context.Background(), transport, []byte("payload"))
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("Upload() error = %v, want ProtocolError", err)
	}
}

func TestUploadRejectsUnexpectedFirstPart(t *testing.T) {
	transport := newFakeTransport()
	bad := &badFirstACKTransport{fakeTransport: transport}
	err := testUploader().Upload(context.Background(), bad, []byte("payload"))
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("Upload() error = %v, want ProtocolError", err)
	}
}

type badFirstACKTransport struct {
	*fakeTransport
}

func (b *badFirstACKTransport) WriteControl(command []byte) error {
	if bytes.Equal(command, []byte{0x03}) {
		b.notify(partACK(3))
		return nil
	}
	return b.fakeTransport.WriteControl(command)
}

func TestUploadRejectsEmptyPayload(t *testing.T) {
	err := testUploader().Upload(context.Background(), newFakeTransport(), nil)
	if err == nil || err.Error() != "payload must not be empty" {
		t.Fatalf("Upload() error = %v", err)
	}
}
