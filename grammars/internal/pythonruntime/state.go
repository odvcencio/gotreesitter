package pythonruntime

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

func SerializeState(s *State, buf []byte) int {
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

func DeserializeState(s *State, buf []byte) {
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
