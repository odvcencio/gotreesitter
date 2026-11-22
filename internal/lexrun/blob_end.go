package lexrun

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// BuiltinEndBlobRuntimeVersion is the minimum reader version for the
// optional unconditional lexer end states.
const BuiltinEndBlobRuntimeVersion uint32 = 2

var lexEndMagic = [8]byte{'G', 'T', 'S', 'L', 'E', 'X', 1, 0}

const lexEndHeaderSize = len(lexEndMagic) + 4

// WrapBuiltinEndStates puts the marked main DFA state indices between the
// blob version header and the legacy payload (gzip or GTSBLOB envelope).
// The ordinary payload is returned unchanged when there are no markers.
func WrapBuiltinEndStates(payload []byte, states []uint32) []byte {
	if len(states) == 0 {
		return payload
	}
	out := make([]byte, lexEndHeaderSize+4*len(states)+len(payload))
	copy(out, lexEndMagic[:])
	binary.BigEndian.PutUint32(out[len(lexEndMagic):], uint32(len(states)))
	for i, state := range states {
		binary.BigEndian.PutUint32(out[lexEndHeaderSize+4*i:], state)
	}
	copy(out[lexEndHeaderSize+4*len(states):], payload)
	return out
}

// UnwrapBuiltinEndStates validates the extension framing before decoding the
// payload. The caller bounds-checks the indices against the decoded DFA.
func UnwrapBuiltinEndStates(blob []byte) (payload []byte, states []uint32, err error) {
	if !bytes.HasPrefix(blob, lexEndMagic[:]) {
		return blob, nil, nil
	}
	if len(blob) <= lexEndHeaderSize {
		return nil, nil, fmt.Errorf("invalid lexer end metadata: truncated header")
	}
	count := binary.BigEndian.Uint32(blob[len(lexEndMagic):])
	if count == 0 || uint64(count)*4 >= uint64(len(blob)-lexEndHeaderSize) {
		return nil, nil, fmt.Errorf("invalid lexer end metadata: state count %d exceeds payload", count)
	}
	for i := uint32(0); i < count; i++ {
		state := binary.BigEndian.Uint32(blob[lexEndHeaderSize+4*int(i):])
		if i != 0 && states[len(states)-1] >= state {
			return nil, nil, fmt.Errorf("invalid lexer end metadata: states must be strictly increasing")
		}
		states = append(states, state)
	}
	return blob[lexEndHeaderSize+4*int(count):], states, nil
}
