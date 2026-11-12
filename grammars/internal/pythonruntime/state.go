package pythonruntime

import "github.com/odvcencio/gotreesitter/internal/scanmode"

// This state and encoding serve Python-derived external scanners. Keep this
// file separate from Python registration so a subset does not publish Python
// metadata when it only includes a derivative grammar.
type Delimiter byte

const (
	DelimiterSingleQuote Delimiter = 1 << 0
	DelimiterDoubleQuote Delimiter = 1 << 1
	DelimiterBackQuote   Delimiter = 1 << 2
	DelimiterRaw         Delimiter = 1 << 3
	DelimiterFormat      Delimiter = 1 << 4
	DelimiterTriple      Delimiter = 1 << 5
	DelimiterBytes       Delimiter = 1 << 6
)

func (d Delimiter) IsFormat() bool { return d&DelimiterFormat != 0 }
func (d Delimiter) IsRaw() bool    { return d&DelimiterRaw != 0 }
func (d Delimiter) IsTriple() bool { return d&DelimiterTriple != 0 }
func (d Delimiter) IsBytes() bool  { return d&DelimiterBytes != 0 }

func (d Delimiter) EndChar() rune {
	switch {
	case d&DelimiterSingleQuote != 0:
		return '\''
	case d&DelimiterDoubleQuote != 0:
		return '"'
	case d&DelimiterBackQuote != 0:
		return '`'
	default:
		return 0
	}
}

type State struct {
	Indents                  []uint16
	Delimiters               []Delimiter
	InsideInterpolatedString bool

	// Flag holds the C-faithful mode (see internal/scanmode): the saved state
	// has the bytes of the pinned C scanner and the string flag is state of its
	// own. It lives in the payload because the scanner value is shared by every
	// parser of the language; only the new core turns it on.
	scanmode.Flag
}

// IsCFaithful reports whether the C-faithful mode is on.
func (s *State) IsCFaithful() bool { return s != nil && s.On() }

// SerializeState saves s in the layout of the scanners that write two-byte
// indents (python and mojo). With the mode off it writes the checkpoint layout
// of the legacy engine; with the mode on, the layout of the pinned C scanner.
func SerializeState(s *State, buf []byte) int {
	if s.IsCFaithful() {
		return serializeStateC2(s, buf)
	}
	return serializeStateLegacy(s, buf)
}

// DeserializeState is the reader of SerializeState.
func DeserializeState(s *State, buf []byte) {
	if s.IsCFaithful() {
		deserializeStateC2(s, buf)
		return
	}
	deserializeStateLegacy(s, buf)
}

// SerializeStateByteIndents is the writer of the starlark scanner: with the
// mode on, one byte per indent as tree-sitter-starlark does; with the mode off
// the legacy checkpoint layout.
func SerializeStateByteIndents(s *State, buf []byte) int {
	if s.IsCFaithful() {
		return serializeStateC1(s, buf)
	}
	return serializeStateLegacy(s, buf)
}

// DeserializeStateByteIndents is the reader of SerializeStateByteIndents.
func DeserializeStateByteIndents(s *State, buf []byte) {
	if s.IsCFaithful() {
		deserializeStateC1(s, buf)
		return
	}
	deserializeStateLegacy(s, buf)
}

func (s *State) SyncInsideInterpolatedString() {
	s.InsideInterpolatedString = false
	for _, d := range s.Delimiters {
		if d.IsFormat() {
			s.InsideInterpolatedString = true
			return
		}
	}
}

// Python scanner checkpoints use a compact, self-delimiting wire format:
// one flag byte, one delimiter-count byte, the delimiter stack, one
// little-endian indent-count word, and the indent stack as little-endian
// words. Keep the delimiter count byte as part of the current prefix shape,
// but treat the payload as ephemeral and version-local, not backward-compatible.
const (
	pythonScannerCheckpointHeaderBytes = 1 + 1 + 2
	maxPythonScannerDelimiterCount     = int(^uint8(0))
	maxPythonScannerIndentCount        = int(^uint16(0))
)

func pythonScannerCheckpointSize(delimiterCount, indentCount int) (int, bool) {
	if delimiterCount < 0 || delimiterCount > maxPythonScannerDelimiterCount ||
		indentCount < 0 || indentCount > maxPythonScannerIndentCount {
		return 0, false
	}
	size := pythonScannerCheckpointHeaderBytes + delimiterCount
	maxInt := int(^uint(0) >> 1)
	if indentCount > (maxInt-size)/2 {
		return 0, false
	}
	return size + indentCount*2, true
}

func serializeStateLegacy(s *State, buf []byte) int {
	if s == nil {
		return 0
	}
	s.SyncInsideInterpolatedString()

	indentCount := 0
	if len(s.Indents) > 0 {
		// Indents[0] is the scanner's root sentinel and is not serialized.
		indentCount = len(s.Indents) - 1
	}
	size, ok := pythonScannerCheckpointSize(len(s.Delimiters), indentCount)
	if !ok || len(buf) < size {
		// Never publish a prefix. A zero return tells the parser that this
		// boundary has no usable checkpoint and forces the safe fallback.
		return 0
	}

	write := 0
	// Always write the flag. Scanner checkpoint buffers are reused between
	// tokens, so leaving a false flag untouched would leak a prior f-string
	// state into the next checkpoint.
	buf[write] = 0
	if s.InsideInterpolatedString {
		buf[write] = 1
	}
	write++
	buf[write] = byte(len(s.Delimiters))
	write++
	for _, delimiter := range s.Delimiters {
		buf[write] = byte(delimiter)
		write++
	}
	buf[write] = byte(indentCount)
	buf[write+1] = byte(indentCount >> 8)
	write += 2

	// Skip Indents[0] (sentinel), serialize from index 1.
	for i := 1; i < len(s.Indents); i++ {
		v := s.Indents[i]
		buf[write] = byte(v)
		buf[write+1] = byte(v >> 8)
		write += 2
	}

	return write
}

func deserializeStateLegacy(s *State, buf []byte) {
	if s == nil {
		return
	}
	s.Delimiters = s.Delimiters[:0]
	s.Indents = s.Indents[:0]
	s.Indents = append(s.Indents, 0)
	s.InsideInterpolatedString = false

	if len(buf) == 0 {
		return
	}
	if len(buf) < pythonScannerCheckpointHeaderBytes {
		return
	}

	encodedInside := buf[0] != 0
	delimCount := int(buf[1])
	indentCountOffset := 2 + delimCount
	if indentCountOffset+2 > len(buf) {
		return
	}
	indentCount := int(buf[indentCountOffset]) | int(buf[indentCountOffset+1])<<8
	required, ok := pythonScannerCheckpointSize(delimCount, indentCount)
	if !ok || len(buf) != required {
		// Reject both truncated and trailing data. Leave the reset sentinel in
		// place so a malformed checkpoint cannot partially restore scanner state.
		return
	}

	inside := false
	for i := 0; i < delimCount; i++ {
		delimiter := Delimiter(buf[2+i])
		s.Delimiters = append(s.Delimiters, delimiter)
		inside = inside || delimiter.IsFormat()
	}
	if encodedInside != inside {
		// The flag is redundant, but an inconsistent checkpoint is not a
		// complete state representation. Fail closed instead of guessing.
		s.Delimiters = s.Delimiters[:0]
		return
	}
	size := indentCountOffset + 2
	for i := 0; i < indentCount; i++ {
		v := uint16(buf[size]) | uint16(buf[size+1])<<8
		s.Indents = append(s.Indents, v)
		size += 2
	}
	s.InsideInterpolatedString = inside
}

// Python scanner state bytes are the bytes of the pinned C scanners
// (tree-sitter-python scanner.c:364-415, and the same layout in tree-sitter-mojo):
//
//	byte 0        inside_interpolated_string, as the scanner stored it
//	byte 1        delimiter count n (at most 255)
//	bytes 2..n+1  the delimiters, one byte each
//	then          the indent stack without the root sentinel, one little-endian
//	              16-bit word per entry; no count is written, the reader takes
//	              words until the bytes end
//
// The flag is state of its own, not derived from the delimiter stack: C sets
// it from the delimiter that a STRING_START pushes and clears it at the end of
// any string (scanner.c:180, :198, :353), so a plain string nested in an
// f-string interpolation leaves it false while an f-string delimiter is still
// on the stack. C's serialize stops adding indent words once the size reaches
// TREE_SITTER_SERIALIZATION_BUFFER_SIZE (scanner.c:383), which may leave the
// size at 1025.
const (
	serializationBufferSize = 1024
	maxDelimiterCount       = int(^uint8(0))
)

func serializeStateC2(s *State, buf []byte) int {
	return serializeState(s, buf, 2)
}

// serializeStateC1 writes the layout of SerializeState, but one byte
// per indent (the low byte), as tree-sitter-starlark does
// (scanner.c:363-387).
func serializeStateC1(s *State, buf []byte) int {
	return serializeState(s, buf, 1)
}

// serializeState writes the C layout and returns the size C's serialize returns
// (python scanner.c:364-390): the header, then indent words while the size is
// below the 1,024-byte serialization buffer size, which can end at 1,025 when an odd
// number of delimiters precedes two-byte indents. Bytes are written only inside buf.
// A size above len(buf) tells the core that the state does not fit; the core then
// stops the parse with a named reason (C writes past its buffer here; this does not).
func serializeState(s *State, buf []byte, indentWidth int) int {
	if s == nil {
		return 0
	}
	delimiterCount := len(s.Delimiters)
	if delimiterCount > maxDelimiterCount {
		delimiterCount = maxDelimiterCount
	}
	put := func(at int, b byte) {
		if at < len(buf) {
			buf[at] = b
		}
	}
	size := 0
	flag := byte(0)
	if s.InsideInterpolatedString {
		flag = 1
	}
	put(size, flag)
	size++
	put(size, byte(delimiterCount))
	size++
	for _, delimiter := range s.Delimiters[:delimiterCount] {
		put(size, byte(delimiter))
		size++
	}
	// Indents[0] is the scanner's root sentinel and is not serialized.
	for i := 1; i < len(s.Indents) && size < serializationBufferSize; i++ {
		v := s.Indents[i]
		put(size, byte(v))
		size++
		if indentWidth == 2 {
			put(size, byte(v>>8))
			size++
		}
	}
	return size
}

func deserializeStateC2(s *State, buf []byte) {
	deserializeState(s, buf, 2)
}

// deserializeStateC1 reads the layout of serializeStateC1
// (tree-sitter-starlark scanner.c:389-413).
func deserializeStateC1(s *State, buf []byte) {
	deserializeState(s, buf, 1)
}

func deserializeState(s *State, buf []byte, indentWidth int) {
	if s == nil {
		return
	}
	s.Delimiters = s.Delimiters[:0]
	s.Indents = append(s.Indents[:0], 0)
	// An empty snapshot clears the stacks but keeps the string flag, as C does
	// (python scanner.c:395-402, mojo :420-427, starlark :392-399).
	if len(buf) == 0 {
		return
	}
	s.InsideInterpolatedString = buf[0] != 0
	size := 1
	if len(buf) < 2 {
		return
	}
	delimiterCount := int(buf[1])
	size = 2
	// C copies delimiterCount bytes whatever the length; a shorter buffer
	// cannot be read past its end here, so only the bytes that exist are used.
	if delimiterCount > len(buf)-size {
		delimiterCount = len(buf) - size
	}
	for i := 0; i < delimiterCount; i++ {
		s.Delimiters = append(s.Delimiters, Delimiter(buf[size+i]))
	}
	size += delimiterCount
	for size+indentWidth <= len(buf) {
		v := uint16(buf[size])
		if indentWidth == 2 {
			v |= uint16(buf[size+1]) << 8
		}
		s.Indents = append(s.Indents, v)
		size += indentWidth
	}
}
