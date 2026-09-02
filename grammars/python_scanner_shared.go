//go:build !grammar_subset || grammar_subset_python || grammar_subset_bitbake || grammar_subset_mojo || grammar_subset_starlark

package grammars

// This state and encoding serve Python-derived external scanners. Keep this
// file separate from Python registration so a subset does not publish Python
// metadata when it only includes a derivative grammar.
type pyDelimiter byte

const (
	pyDelimSingleQuote pyDelimiter = 1 << 0
	pyDelimDoubleQuote pyDelimiter = 1 << 1
	pyDelimBackQuote   pyDelimiter = 1 << 2
	pyDelimRaw         pyDelimiter = 1 << 3
	pyDelimFormat      pyDelimiter = 1 << 4
	pyDelimTriple      pyDelimiter = 1 << 5
	pyDelimBytes       pyDelimiter = 1 << 6
)

func (d pyDelimiter) isFormat() bool { return d&pyDelimFormat != 0 }
func (d pyDelimiter) isRaw() bool    { return d&pyDelimRaw != 0 }
func (d pyDelimiter) isTriple() bool { return d&pyDelimTriple != 0 }
func (d pyDelimiter) isBytes() bool  { return d&pyDelimBytes != 0 }

func (d pyDelimiter) endChar() rune {
	switch {
	case d&pyDelimSingleQuote != 0:
		return '\''
	case d&pyDelimDoubleQuote != 0:
		return '"'
	case d&pyDelimBackQuote != 0:
		return '`'
	default:
		return 0
	}
}

type pythonScannerState struct {
	indents                  []uint16
	delimiters               []pyDelimiter
	insideInterpolatedString bool
}

func (s *pythonScannerState) syncInsideInterpolatedString() {
	s.insideInterpolatedString = false
	for _, d := range s.delimiters {
		if d.isFormat() {
			s.insideInterpolatedString = true
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

func serializePythonScannerState(s *pythonScannerState, buf []byte) int {
	if s == nil {
		return 0
	}
	s.syncInsideInterpolatedString()

	indentCount := 0
	if len(s.indents) > 0 {
		// indents[0] is the scanner's root sentinel and is not serialized.
		indentCount = len(s.indents) - 1
	}
	size, ok := pythonScannerCheckpointSize(len(s.delimiters), indentCount)
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
	if s.insideInterpolatedString {
		buf[write] = 1
	}
	write++
	buf[write] = byte(len(s.delimiters))
	write++
	for _, delimiter := range s.delimiters {
		buf[write] = byte(delimiter)
		write++
	}
	buf[write] = byte(indentCount)
	buf[write+1] = byte(indentCount >> 8)
	write += 2

	// Skip indents[0] (sentinel), serialize from index 1.
	for i := 1; i < len(s.indents); i++ {
		v := s.indents[i]
		buf[write] = byte(v)
		buf[write+1] = byte(v >> 8)
		write += 2
	}

	return write
}

func deserializePythonScannerState(s *pythonScannerState, buf []byte) {
	if s == nil {
		return
	}
	s.delimiters = s.delimiters[:0]
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	s.insideInterpolatedString = false

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
		delimiter := pyDelimiter(buf[2+i])
		s.delimiters = append(s.delimiters, delimiter)
		inside = inside || delimiter.isFormat()
	}
	if encodedInside != inside {
		// The flag is redundant, but an inconsistent checkpoint is not a
		// complete state representation. Fail closed instead of guessing.
		s.delimiters = s.delimiters[:0]
		return
	}
	size := indentCountOffset + 2
	for i := 0; i < indentCount; i++ {
		v := uint16(buf[size]) | uint16(buf[size+1])<<8
		s.indents = append(s.indents, v)
		size += 2
	}
	s.insideInterpolatedString = inside
}
