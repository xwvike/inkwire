package gicisky

import (
	"encoding/binary"
	"fmt"

	"github.com/xwvike/inkwire/internal/display"
)

// EncodeOriented converts a logical page into the panel's physical payload.
// The profile is discovered from the advertisement before a real Gicisky write
// starts, so one renderer can serve every known panel size.
func EncodeOriented(frame *display.Frame, orientation display.Orientation, profile Profile) ([]byte, error) {
	if frame == nil {
		return nil, fmt.Errorf("frame must not be nil")
	}
	if profile.Width <= 0 || profile.Height <= 0 {
		return nil, fmt.Errorf("Gicisky profile has invalid size %dx%d", profile.Width, profile.Height)
	}
	landscape, err := landscapeFrame(frame, orientation, profile.Width, profile.Height)
	if err != nil {
		return nil, err
	}
	return Encode(landscape, profile)
}

// Encode packs a frame already rendered at profile.Width x profile.Height.
// Rotation and mirrors are panel-buffer transforms, not scene transforms.
func Encode(frame *display.Frame, profile Profile) ([]byte, error) {
	if frame == nil {
		return nil, fmt.Errorf("frame must not be nil")
	}
	if frame.Width() != profile.Width || frame.Height() != profile.Height {
		return nil, fmt.Errorf("Gicisky frame must be %dx%d, got %dx%d", profile.Width, profile.Height, frame.Width(), frame.Height())
	}
	if err := validatePalette(frame, profile); err != nil {
		return nil, err
	}

	physical := frame
	var err error
	if profile.TFT {
		physical, err = resizeFrame(physical, max(1, profile.Width/2), profile.Height*2)
		if err != nil {
			return nil, err
		}
	}
	if profile.Rotation != 0 {
		physical = rotateFrame(physical, profile.Rotation)
	}

	if profile.FourColor {
		return packFourColor(physical, profile)
	}
	black, red := packPlanes(physical, profile)
	if profile.Compression2 {
		return compress2(append(black, red...)), nil
	}
	if profile.Compression {
		return compressColumns(black, red, physical.Width(), physical.Height())
	}
	if profile.Palette == PaletteBW {
		return black, nil
	}
	return append(black, red...), nil
}

func validatePalette(frame *display.Frame, profile Profile) error {
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			switch profile.Palette {
			case PaletteBW:
				if ink == display.InkRed || ink == display.InkYellow {
					return fmt.Errorf("%s panel cannot show %s ink at (%d,%d)", profile.Palette, ink, x, y)
				}
			case PaletteBWR:
				if ink == display.InkYellow {
					return fmt.Errorf("%s panel cannot show yellow ink at (%d,%d)", profile.Palette, x, y)
				}
			case PaletteBWRY:
			default:
				return fmt.Errorf("unsupported Gicisky palette %s", profile.Palette)
			}
		}
	}
	return nil
}

func landscapeFrame(frame *display.Frame, orientation display.Orientation, width, height int) (*display.Frame, error) {
	switch orientation {
	case display.OrientationLandscape:
		if frame.Width() != width || frame.Height() != height {
			return nil, fmt.Errorf("landscape page must be %dx%d, got %dx%d", width, height, frame.Width(), frame.Height())
		}
		return frame, nil
	case display.OrientationPortraitClockwise, display.OrientationPortraitCounterClockwise:
		if frame.Width() != height || frame.Height() != width {
			return nil, fmt.Errorf("portrait page must be %dx%d, got %dx%d", height, width, frame.Width(), frame.Height())
		}
	default:
		return nil, fmt.Errorf("invalid orientation %d", orientation)
	}

	landscape, err := display.NewFrame(width, height, display.InkWhite)
	if err != nil {
		return nil, err
	}
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			switch orientation {
			case display.OrientationPortraitClockwise:
				landscape.Set(frame.Height()-1-y, x, ink)
			case display.OrientationPortraitCounterClockwise:
				landscape.Set(y, frame.Width()-1-x, ink)
			}
		}
	}
	return landscape, nil
}

func resizeFrame(source *display.Frame, width, height int) (*display.Frame, error) {
	out, err := display.NewFrame(width, height, display.InkWhite)
	if err != nil {
		return nil, err
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sourceX := min(source.Width()-1, x*source.Width()/width)
			sourceY := min(source.Height()-1, y*source.Height()/height)
			ink, _ := source.InkAt(sourceX, sourceY)
			out.Set(x, y, ink)
		}
	}
	return out, nil
}

func rotateFrame(source *display.Frame, degrees int) *display.Frame {
	degrees = ((degrees % 360) + 360) % 360
	if degrees == 0 {
		return source
	}
	width, height := source.Width(), source.Height()
	outWidth, outHeight := width, height
	if degrees == 90 || degrees == 270 {
		outWidth, outHeight = height, width
	}
	out, _ := display.NewFrame(outWidth, outHeight, display.InkWhite)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var outX, outY int
			switch degrees {
			case 90:
				outX, outY = y, width-1-x
			case 180:
				outX, outY = width-1-x, height-1-y
			case 270:
				outX, outY = height-1-y, x
			}
			ink, _ := source.InkAt(x, y)
			out.Set(outX, outY, ink)
		}
	}
	return out
}

func packPlanes(frame *display.Frame, profile Profile) ([]byte, []byte) {
	pixels := frame.Width() * frame.Height()
	black := make([]byte, (pixels+7)/8)
	red := make([]byte, (pixels+7)/8)
	index := 0
	for y := eachCoordinate(frame.Height(), profile.MirrorY); y >= 0 && y < frame.Height(); y = nextCoordinate(y, frame.Height(), profile.MirrorY) {
		for x := eachCoordinate(frame.Width(), profile.MirrorX); x >= 0 && x < frame.Width(); x = nextCoordinate(x, frame.Width(), profile.MirrorX) {
			ink, _ := frame.InkAt(x, y)
			white := ink == display.InkWhite || ink == display.InkYellow
			if profile.InvertLuminance {
				white = !white
			}
			if white {
				black[index/8] |= 1 << uint(7-index%8)
			}
			if ink == display.InkRed {
				red[index/8] |= 1 << uint(7-index%8)
			}
			index++
		}
	}
	if profile.Palette == PaletteBW {
		return black, nil
	}
	return black, red
}

func packFourColor(frame *display.Frame, profile Profile) ([]byte, error) {
	if frame.Width()*frame.Height()%4 != 0 {
		return nil, fmt.Errorf("four-color Gicisky frame has %d pixels, which is not divisible by four", frame.Width()*frame.Height())
	}
	packed := make([]byte, 0, frame.Width()*frame.Height()/4)
	var value byte
	count := 0
	for y := eachCoordinate(frame.Height(), profile.MirrorY); y >= 0 && y < frame.Height(); y = nextCoordinate(y, frame.Height(), profile.MirrorY) {
		for x := eachCoordinate(frame.Width(), profile.MirrorX); x >= 0 && x < frame.Width(); x = nextCoordinate(x, frame.Width(), profile.MirrorX) {
			ink, _ := frame.InkAt(x, y)
			colorValue := byte(0)
			switch ink {
			case display.InkWhite:
				colorValue = 1
			case display.InkYellow:
				colorValue = 2
			case display.InkRed:
				colorValue = 3
			}
			value = (value << 2) | colorValue
			count++
			if count == 4 {
				packed = append(packed, value)
				value, count = 0, 0
			}
		}
	}
	return packed, nil
}

func compressColumns(black, red []byte, width, height int) ([]byte, error) {
	if height%8 != 0 {
		return nil, fmt.Errorf("Gicisky compressed frame height must be divisible by 8, got %d", height)
	}
	bytesPerLine := height / 8
	if len(black) != width*bytesPerLine {
		return nil, fmt.Errorf("Gicisky compressed plane has %d bytes, want %d", len(black), width*bytesPerLine)
	}
	result := make([]byte, 4)
	appendChunks := func(plane []byte) {
		for offset := 0; offset < len(plane); offset += bytesPerLine {
			result = append(result, 0x75, byte(bytesPerLine+7), byte(bytesPerLine), 0, 0, 0, 0)
			result = append(result, plane[offset:offset+bytesPerLine]...)
		}
	}
	appendChunks(black)
	if red != nil {
		appendChunks(red)
	}
	binary.LittleEndian.PutUint32(result[:4], uint32(len(result)))
	return result, nil
}

func compress2(data []byte) []byte {
	split := len(data) / 2
	result := make([]byte, 4)
	binary.LittleEndian.PutUint32(result, uint32(len(data)-split))
	appendPart := func(part []byte) {
		result = append(result, compress2Chunks(part)...)
	}
	appendPart(data[:split])
	appendPart(data[split:])
	return result
}

func compress2Chunks(data []byte) []byte {
	const chunkSize = 64
	result := make([]byte, 0, len(data))
	for offset := 0; offset < len(data); {
		end := min(len(data), offset+chunkSize)
		chunk := data[offset:end]
		if compressed := qlzCompressCore(chunk); compressed != nil {
			result = append(result, 0x75, byte(len(compressed)+3), byte(len(chunk)))
			result = append(result, compressed...)
		} else {
			result = append(result, 0x74, byte(len(chunk)+3), byte(len(chunk)))
			result = append(result, chunk...)
		}
		offset = end
	}
	return result
}

const (
	qlzCwordLen                      = 4
	qlzHashValues                    = 64
	qlzNoEntry                       = -1
	qlzMinOffset                     = 2
	qlzUnconditionalMatchLenCompress = 12
	qlzUncompressedEnd               = 4
)

func qlzCompressCore(source []byte) []byte {
	size := len(source)
	lastByteIndex := size - 1
	lastMatchStart := lastByteIndex - qlzUnconditionalMatchLenCompress - qlzUncompressedEnd
	if lastMatchStart < 0 {
		return nil
	}

	out := make([]byte, size*2+400)
	cwordPtr := 0
	dst := qlzCwordLen
	cwordValue := uint32(1 << 31)
	src := 0
	literals := 0
	hashOffset := [qlzHashValues]int{}
	hashCache := [qlzHashValues]uint32{}
	for i := range hashOffset {
		hashOffset[i] = qlzNoEntry
	}

	for src <= lastMatchStart {
		if cwordValue&1 == 1 {
			if src > size>>1 && dst > src-(src>>5) {
				return nil
			}
			binary.LittleEndian.PutUint32(out[cwordPtr:], (cwordValue>>1)|(1<<31))
			cwordPtr = dst
			dst += qlzCwordLen
			cwordValue = 1 << 31
		}

		fetch := qlzRead3(source, src)
		hash := qlzHash(fetch)
		cached := fetch ^ hashCache[hash]
		hashCache[hash] = fetch
		origin := hashOffset[hash]
		hashOffset[hash] = src

		distance := src - origin
		if cached&0xFFFFFF == 0 && origin != qlzNoEntry && (distance > qlzMinOffset || (src == origin+1 && literals >= 3 && src > 3 && qlzSame(source, src-3, 6))) {
			matchLen := 3
			remaining := min(255, lastByteIndex-qlzUncompressedEnd-src+1)
			for matchLen < remaining && source[src+matchLen] == source[origin+matchLen] {
				matchLen++
			}

			shiftedHash := hash << 4
			cwordValue = (cwordValue >> 1) | (1 << 31)
			if matchLen < 18 {
				value := (matchLen - 2) | shiftedHash
				out[dst] = byte(value)
				out[dst+1] = byte(value >> 8)
				dst += 2
			} else {
				out[dst] = byte(shiftedHash)
				out[dst+1] = byte(shiftedHash >> 8)
				out[dst+2] = byte(matchLen)
				dst += 3
			}
			src += matchLen
			literals = 0
			continue
		}

		literals++
		out[dst] = source[src]
		src++
		dst++
		cwordValue >>= 1
	}

	for src <= lastByteIndex {
		if cwordValue&1 == 1 {
			binary.LittleEndian.PutUint32(out[cwordPtr:], (cwordValue>>1)|(1<<31))
			cwordPtr = dst
			dst += qlzCwordLen
			cwordValue = 1 << 31
		}

		if src <= lastByteIndex-2 {
			fetch := qlzRead3(source, src)
			hash := qlzHash(fetch)
			hashCache[hash] = fetch
			hashOffset[hash] = src
		}

		out[dst] = source[src]
		src++
		dst++
		cwordValue >>= 1
	}

	for cwordValue&1 != 1 {
		cwordValue >>= 1
	}
	binary.LittleEndian.PutUint32(out[cwordPtr:], (cwordValue>>1)|(1<<31))

	if dst >= size {
		return nil
	}
	return append([]byte(nil), out[:dst]...)
}

func qlzHash(fetch uint32) int {
	return int(((fetch >> 12) ^ fetch) & (qlzHashValues - 1))
}

func qlzRead3(data []byte, pos int) uint32 {
	if pos+3 > len(data) {
		return 0
	}
	return uint32(data[pos]) | uint32(data[pos+1])<<8 | uint32(data[pos+2])<<16
}

func qlzSame(data []byte, pos, count int) bool {
	if pos < 0 || pos+count >= len(data) {
		return false
	}
	value := data[pos]
	for i := 1; i <= count; i++ {
		if data[pos+i] != value {
			return false
		}
	}
	return true
}

func eachCoordinate(size int, reverse bool) int {
	if reverse {
		return size - 1
	}
	return 0
}

func nextCoordinate(value, size int, reverse bool) int {
	if reverse {
		return value - 1
	}
	return value + 1
}
