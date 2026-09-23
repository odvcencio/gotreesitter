package gotreesitter

import (
	"encoding/gob"
	"io"
)

// init primes encoding/gob's wire type IDs for every type the language blob
// encoder writes, in a fixed order, before any other code in the process can
// gob-encode a value.
//
// gob assigns each type's wire ID from a process-global counter the first
// time the type crosses any Encoder. The IDs are written into the stream, so
// two processes that encode an identical Language produce different bytes
// when one of them encoded unrelated types first (a test binary running a
// neighboring test, for example), even though every decoded field is
// identical. That made blob SHA-256 pins depend on process history (task
// #37). Allocating the IDs here, from this package's init, fixes their
// values for every binary that imports the runtime: the packages this one
// depends on never gob-encode during init, and every package that could
// encode something else is initialized after this one.
//
// The order matters and must not change: the Language graph first, then the
// large-state-gotos trailer pair slice. A blob without LargeStateGotos was
// always encoded with the Language graph first, so shipped blobs of that kind
// keep their bytes; a blob with a trailer was encoded trailer-first by a
// fresh process before this priming existed, so its bytes change on the next
// regeneration only (every decoded field stays identical).
func init() {
	primeLanguageBlobGobTypes()
}

func primeLanguageBlobGobTypes() {
	enc := gob.NewEncoder(io.Discard)
	_ = enc.Encode(&Language{})
	_ = enc.Encode([]largeStateGotoPair{})
}
