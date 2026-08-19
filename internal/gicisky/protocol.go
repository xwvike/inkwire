package gicisky

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/xwvike/inkwire/internal/ble"
	"math"
	"time"
)

const (
	DefaultNotifyReadyDelay   = 2 * time.Second
	DefaultNotifyProcessDelay = 50 * time.Millisecond
	// Every measured notification came back in about 105 ms, including the
	// last one that reports the refresh has started, so this is two orders
	// of magnitude of headroom over a tag that is answering at all.
	DefaultResponseTimeout = 5 * time.Second
)

type ProtocolError struct {
	message string
}

func (e *ProtocolError) Error() string {
	return e.message
}

func newProtocolError(format string, args ...any) error {
	return &ProtocolError{message: fmt.Sprintf(format, args...)}
}

type Transport interface {
	Notifications() <-chan []byte
	WriteControl([]byte) error
	WriteData([]byte) error
}

type Uploader struct {
	NotifyReadyDelay   time.Duration
	NotifyProcessDelay time.Duration
	ResponseTimeout    time.Duration
	Logf               func(string, ...any)
}

type UploadOptions struct {
	Compression2 bool
}

func NewUploader(logf func(string, ...any)) Uploader {
	return Uploader{
		NotifyReadyDelay:   DefaultNotifyReadyDelay,
		NotifyProcessDelay: DefaultNotifyProcessDelay,
		ResponseTimeout:    DefaultResponseTimeout,
		Logf:               logf,
	}
}

func ValidatePayload(payload []byte) error {
	if len(payload) == 0 {
		return errors.New("payload must not be empty")
	}
	if uint64(len(payload)) > math.MaxUint32 {
		return errors.New("payload is too large for the uint32 length field")
	}
	return nil
}

func (u Uploader) Upload(ctx context.Context, transport Transport, payload []byte) error {
	return u.UploadWithOptions(ctx, transport, payload, UploadOptions{})
}

func (u Uploader) UploadWithOptions(ctx context.Context, transport Transport, payload []byte, options UploadOptions) error {
	if err := ValidatePayload(payload); err != nil {
		return err
	}
	if err := ble.Wait(ctx, u.NotifyReadyDelay); err != nil {
		return err
	}

	if err := transport.WriteControl([]byte{0x01}); err != nil {
		return fmt.Errorf("stage 1 write: %w", err)
	}
	response, err := u.receive(ctx, transport.Notifications())
	if err != nil {
		return fmt.Errorf("stage 1: %w", err)
	}
	if len(response) < 3 || response[0] != 0x01 {
		return newProtocolError("stage 1: unexpected %x", response)
	}
	messageSize := int(binary.LittleEndian.Uint16(response[1:3]))
	blockSize := messageSize - 4
	if blockSize <= 0 {
		return newProtocolError("stage 1: invalid message size %d", messageSize)
	}
	u.logf("tag requested %d byte messages -> %d byte blocks", messageSize, blockSize)

	lengthCommand := make([]byte, 8)
	if options.Compression2 {
		lengthCommand = make([]byte, 6)
		lengthCommand[5] = 0x01
	}
	lengthCommand[0] = 0x02
	binary.LittleEndian.PutUint32(lengthCommand[1:5], uint32(len(payload)))
	if err := transport.WriteControl(lengthCommand); err != nil {
		return fmt.Errorf("stage 2 write: %w", err)
	}
	response, err = u.receive(ctx, transport.Notifications())
	if err != nil {
		return fmt.Errorf("stage 2: %w", err)
	}
	if len(response) == 0 || response[0] != 0x02 {
		return newProtocolError("stage 2: unexpected %x", response)
	}

	if err := transport.WriteControl([]byte{0x03}); err != nil {
		return fmt.Errorf("stage 3 write: %w", err)
	}

	expected := uint32(0)
	var last []byte
	total := (len(payload)-1)/blockSize + 1

	for {
		response, err = u.receive(ctx, transport.Notifications())
		if err != nil {
			return fmt.Errorf("transfer: %w", err)
		}
		if len(response) < 2 || response[0] != 0x05 {
			return newProtocolError("transfer: unexpected %x", response)
		}
		if response[1] == 0x08 {
			u.logf("upload complete, tag is refreshing")
			return nil
		}
		if response[1] != 0x00 {
			return newProtocolError("tag reported error %x", response)
		}
		if len(response) < 6 {
			return newProtocolError("transfer: truncated %x", response)
		}

		acknowledged := binary.LittleEndian.Uint32(response[2:6])
		if acknowledged == expected {
			start := uint64(expected) * uint64(blockSize)
			if start >= uint64(len(payload)) {
				continue
			}
			end := start + uint64(blockSize)
			if end > uint64(len(payload)) {
				end = uint64(len(payload))
			}

			last = make([]byte, 4+int(end-start))
			binary.LittleEndian.PutUint32(last[:4], expected)
			copy(last[4:], payload[int(start):int(end)])
			u.logf("part %d/%d", uint64(expected)+1, total)
			expected++
		} else {
			if last == nil {
				return newProtocolError("tag requested unexpected first part %d", acknowledged)
			}
			u.logf("ack %d != expected %d, resending", acknowledged, expected)
		}

		if err := transport.WriteData(last); err != nil {
			return fmt.Errorf("data write: %w", err)
		}
	}
}

func (u Uploader) receive(ctx context.Context, notifications <-chan []byte) ([]byte, error) {
	timeout := u.ResponseTimeout
	if timeout <= 0 {
		timeout = DefaultResponseTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var data []byte
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("notification timeout after %s", timeout)
	case received, ok := <-notifications:
		if !ok {
			return nil, errors.New("notification stream closed")
		}
		data = received
	}

	if err := ble.Wait(ctx, u.NotifyProcessDelay); err != nil {
		return nil, err
	}
	return data, nil
}

func (u Uploader) logf(format string, args ...any) {
	if u.Logf != nil {
		u.Logf(format, args...)
	}
}
