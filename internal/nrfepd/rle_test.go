package nrfepd

import (
	"bytes"
	"math/rand"
	"testing"
)

// firmwareBufferSize is the destination the firmware decompresses into before
// each write to panel RAM: uint8_t rle_out[UINT8_MAX]. It matters here because
// the decoder is written to stop early and be called again, and a codec that
// only works when handed an unbounded buffer would fail on the device.
const firmwareBufferSize = 255

// decompressFrom is a transcription of rle_decompress_from from EPD_service.c.
// It is deliberately a translation rather than an implementation: the question
// these tests answer is whether the firmware can read what this package writes,
// so the firmware's own reading is the only thing worth asking.
func decompressFrom(src []byte, srcPos *int, dst []byte) int {
	dstPos := 0
	for *srcPos < len(src) && dstPos < len(dst) {
		control := src[*srcPos]
		if control&0x80 != 0 {
			count := int(control&0x7f) + 3
			if *srcPos+1 >= len(src) {
				break
			}
			if dstPos+count > len(dst) {
				break
			}
			*srcPos += 2
			value := src[*srcPos-1]
			for range count {
				dst[dstPos] = value
				dstPos++
			}
			continue
		}
		count := int(control) + 1
		if *srcPos+1+count > len(src) {
			break
		}
		if dstPos+count > len(dst) {
			break
		}
		*srcPos++
		for range count {
			dst[dstPos] = src[*srcPos]
			dstPos++
			*srcPos++
		}
	}
	return dstPos
}

// decode drives the transcribed decoder the way the firmware drives it: into a
// fixed buffer, repeatedly, until the stream is used up.
func decode(t *testing.T, stream []byte) []byte {
	t.Helper()
	var out []byte
	buffer := make([]byte, firmwareBufferSize)
	position := 0
	for position < len(stream) {
		written := decompressFrom(stream, &position, buffer)
		if written == 0 {
			t.Fatalf("the firmware decoder stopped with %d bytes left unread", len(stream)-position)
		}
		out = append(out, buffer[:written]...)
	}
	return out
}

func samples() map[string][]byte {
	random := make([]byte, 4096)
	rand.New(rand.NewSource(1)).Read(random)

	// A plane is mostly paper with ink in runs, which is the shape the codec
	// actually meets and the one where a bug would still look like it works on
	// random data.
	plane := bytes.Repeat([]byte{0xff}, 4000)
	for index := 300; index < 340; index++ {
		plane[index] = 0x00
	}
	for index := 1000; index < 1003; index++ {
		plane[index] = 0x81
	}
	plane[2000], plane[2001] = 0x0f, 0xf0

	return map[string][]byte{
		"empty":              {},
		"one byte":           {0x5a},
		"two alike":          {0xff, 0xff},
		"three alike":        {0xff, 0xff, 0xff},
		"all alike":          bytes.Repeat([]byte{0xff}, 5000),
		"longer than a code": bytes.Repeat([]byte{0x00}, maxRepeat+7),
		"alternating":        bytes.Repeat([]byte{0xaa, 0x55}, 500),
		"random":             random,
		"a plane":            plane,
	}
}

// The whole point: whatever this package sends, the firmware's decoder has to
// give back exactly what was drawn.
func TestTheFirmwareDecoderRecoversWhatWasCompressed(t *testing.T) {
	for name, data := range samples() {
		t.Run(name, func(t *testing.T) {
			if len(data) == 0 {
				return
			}
			if got := decode(t, compress(data, maxLiteral)); !bytes.Equal(got, data) {
				t.Errorf("recovered %d bytes, sent %d", len(got), len(data))
			}
		})
	}
}

// Each piece is handed over as its own write and read from its own beginning,
// so a piece has to be a complete stream. A split through the middle of a
// literal would have its first data byte read as a control byte, which does not
// fail: it unpacks into plausible noise.
func TestEveryChunkIsAStreamOfItsOwn(t *testing.T) {
	for name, data := range samples() {
		for _, limit := range []int{2, 3, 20, 128, 244} {
			t.Run(name, func(t *testing.T) {
				chunks := chunkRLE(data, limit)
				var rebuilt []byte
				for index, chunk := range chunks {
					if len(chunk) > limit {
						t.Fatalf("chunk %d is %d bytes, over the %d limit", index, len(chunk), limit)
					}
					// Decoded on its own, exactly as the firmware will.
					rebuilt = append(rebuilt, decode(t, chunk)...)
				}
				if !bytes.Equal(rebuilt, data) {
					t.Errorf("limit %d: rebuilt %d bytes from %d chunks, sent %d",
						limit, len(rebuilt), len(chunks), len(data))
				}
			})
		}
	}
}

// A run of paper is the case this exists for, so it is worth stating what it
// buys rather than only that it round trips.
//
// The number to hold it to is the format's own ceiling rather than a fraction
// somebody liked the look of. One repeat code carries at most 130 bytes and
// costs two, so a blank panel cannot come out smaller than two bytes per 130
// however good the encoder is, and it should not come out larger either.
func TestAPlaneOfPaperCompressesToTheFormatsCeiling(t *testing.T) {
	plane := bytes.Repeat([]byte{0xff}, 400*300/8)
	encoded := compress(plane, maxLiteral)
	best := 2 * ((len(plane) + maxRepeat - 1) / maxRepeat)
	if len(encoded) != best {
		t.Errorf("%d bytes of paper compressed to %d, and the format's best is %d",
			len(plane), len(encoded), best)
	}
	// Worth saying out loud, because it is the whole reason a 400x300 page is
	// sendable over a link that moves a couple of hundred bytes at a time.
	t.Logf("a blank 400x300 plane: %d bytes down to %d", len(plane), len(encoded))
}

// Compression is not a promise, and the caller has to be able to find out. A
// stream of unlike bytes costs a control byte for every 128 of them, so it
// comes out larger, and sending it would be slower than sending the picture.
func TestUnlikeBytesGrow(t *testing.T) {
	data := make([]byte, 1000)
	for index := range data {
		data[index] = byte(index % 251)
	}
	if len(compress(data, maxLiteral)) <= len(data) {
		t.Error("a stream with nothing to repeat did not grow, which cannot be right")
	}
}
