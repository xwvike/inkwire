package nrfepd

// The firmware's compression, which is a run length encoding with two codes.
//
// A control byte with the top bit set is a repeat: the low seven bits plus
// three give the count, and the byte after it is the value. A control byte
// without it is a literal: the byte itself plus one gives how many of the
// bytes after it are copied out as they are. Three is the shortest run worth
// encoding because two bytes of run cost two bytes to say.
//
// The counts are what they are because of where the decoder lives. A repeat is
// at most 130 so that the control byte stays inside a byte, and a literal is at
// most 128 for the same reason on the other code.

const (
	// maxRepeat is the longest run one repeat code can say: 0x7F + 3.
	maxRepeat = 130
	// maxLiteral is the longest run of unlike bytes one literal code can
	// carry: 0x7F + 1.
	maxLiteral = 128
	// shortestRun is where a repeat starts paying for itself. Two equal bytes
	// cost two bytes either way, so the saving only begins at three.
	shortestRun = 3
)

// compress encodes data, keeping every literal run within limit so that no
// single code grows past what the caller can carry.
func compress(data []byte, limit int) []byte {
	if limit > maxLiteral {
		limit = maxLiteral
	}
	if limit < 1 {
		limit = 1
	}
	var out []byte
	for index := 0; index < len(data); {
		run := 1
		for index+run < len(data) && run < maxRepeat && data[index+run] == data[index] {
			run++
		}
		if run >= shortestRun {
			out = append(out, byte(0x80|(run-shortestRun)), data[index])
			index += run
			continue
		}
		// A literal run ends where a repeat worth encoding begins, otherwise
		// the run would swallow bytes that compress better on their own.
		start := index
		length := 0
		for index < len(data) && length < limit {
			if index+2 < len(data) && data[index] == data[index+1] && data[index] == data[index+2] {
				break
			}
			length++
			index++
		}
		out = append(out, byte(length-1))
		out = append(out, data[start:start+length]...)
	}
	return out
}

// chunkRLE compresses data and splits the result into pieces that each fit in
// limit bytes.
//
// The split has to fall between codes rather than anywhere. Each piece is
// handed to the firmware as its own write, and the decoder there starts from
// the beginning of whatever it is given: a piece cut through the middle of a
// literal would be read as a control byte and unpack into noise. So every
// piece is a complete, self-contained stream, and the pieces concatenate to
// exactly the original.
func chunkRLE(data []byte, limit int) [][]byte {
	if limit < 2 {
		return nil
	}
	// A literal code is a control byte and its bytes, so the run has to leave
	// room for the control byte or the code alone would not fit a piece.
	encoded := compress(data, limit-1)

	var chunks [][]byte
	start, index := 0, 0
	for index < len(encoded) {
		size := codeSize(encoded, index)
		if size == 0 {
			break
		}
		if index+size-start > limit {
			chunks = append(chunks, encoded[start:index])
			start = index
		}
		index += size
	}
	if start < len(encoded) {
		chunks = append(chunks, encoded[start:])
	}
	return chunks
}

// codeSize reports how many bytes the code at index occupies, or zero if the
// stream ends inside it.
func codeSize(encoded []byte, index int) int {
	control := encoded[index]
	if control&0x80 != 0 {
		if index+1 >= len(encoded) {
			return 0
		}
		return 2
	}
	size := 1 + int(control) + 1
	if index+size > len(encoded) {
		return 0
	}
	return size
}
