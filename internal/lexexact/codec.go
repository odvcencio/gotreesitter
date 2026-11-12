package lexexact

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
)

// Encode writes the record in the version 1 byte form:
// Magic, the version byte, BlobSHA256, SourceSHA256; then for the main and the
// keyword function: the state count, Accept, OnEOF and OnInvalid as unsigned
// varints, a flag byte for own ranges, and if it is 1 the Start list (its
// length, then its values) and, per state, the range count and, per range,
// lo, hi-lo, and target<<1|skip.
func (t *Tables) Encode() ([]byte, error) {
	if err := t.validate(); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(Magic)
	b.WriteByte(t.Version)
	b.Write(t.BlobSHA256[:])
	b.Write(t.SourceSHA256[:])
	for _, f := range []*Func{&t.Main, &t.Keyword} {
		putUvarint(&b, uint64(len(f.Accept)))
		for _, s := range [][]uint32{f.Accept, f.OnEOF, f.OnInvalid} {
			for _, v := range s {
				putUvarint(&b, uint64(v))
			}
		}
		if f.Trans == nil {
			b.WriteByte(0)
			continue
		}
		b.WriteByte(1)
		putUvarint(&b, uint64(len(f.Start)))
		for _, v := range f.Start {
			putUvarint(&b, uint64(v))
		}
		for _, rs := range f.Trans {
			putUvarint(&b, uint64(len(rs)))
			for _, r := range rs {
				if r.Hi < r.Lo {
					return nil, bad("range %d..%d is reversed", r.Lo, r.Hi)
				}
				putUvarint(&b, uint64(r.Lo))
				putUvarint(&b, uint64(r.Hi-r.Lo))
				v := uint64(r.Target) << 1
				if r.Skip {
					v |= 1
				}
				putUvarint(&b, v)
			}
		}
	}
	return b.Bytes(), nil
}

func putUvarint(b *bytes.Buffer, v uint64) {
	var tmp [binary.MaxVarintLen64]byte
	b.Write(tmp[:binary.PutUvarint(tmp[:], v)])
}

type reader struct {
	b   []byte
	err error
}

func (r *reader) uvarint() uint64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.err = bad("truncated or oversized number")
		return 0
	}
	r.b = r.b[n:]
	return v
}

func (r *reader) u32() uint32 {
	v := r.uvarint()
	if v > 1<<32-1 {
		r.err = bad("number %d does not fit 32 bits", v)
		return 0
	}
	return uint32(v)
}

// count reads a length and checks that the rest of the input can hold it,
// at one byte per item at least, so that a bad length cannot force a big allocation.
func (r *reader) count(perItem int) int {
	v := r.uvarint()
	if r.err == nil && (v > uint64(len(r.b)) || v*uint64(perItem) > uint64(len(r.b))) {
		r.err = bad("length %d exceeds the remaining %d bytes", v, len(r.b))
	}
	if r.err != nil {
		return 0
	}
	return int(v)
}

// Decode reads a record. It rejects an unknown version, a target outside the
// table, and trailing bytes. Call Validate to compare the state counts with
// the blob.
func Decode(data []byte) (*Tables, error) {
	if len(data) < len(Magic)+1+64 || string(data[:len(Magic)]) != Magic {
		return nil, bad("no %s header", Magic)
	}
	t := &Tables{Version: data[len(Magic)]}
	if t.Version != Version {
		return nil, bad("unknown version %d", t.Version)
	}
	rest := data[len(Magic)+1:]
	copy(t.BlobSHA256[:], rest[:32])
	copy(t.SourceSHA256[:], rest[32:64])
	r := &reader{b: rest[64:]}
	for _, f := range []*Func{&t.Main, &t.Keyword} {
		n := r.count(3)
		if n > 0 { // an empty function keeps nil slices, as Build leaves them
			f.Accept, f.OnEOF, f.OnInvalid = make([]uint32, n), make([]uint32, n), make([]uint32, n)
		}
		for _, s := range [][]uint32{f.Accept, f.OnEOF, f.OnInvalid} {
			for j := range s {
				s[j] = r.u32()
			}
		}
		if r.err != nil {
			return nil, r.err
		}
		if len(r.b) == 0 {
			return nil, bad("truncated before the own-ranges flag")
		}
		flag := r.b[0]
		r.b = r.b[1:]
		switch flag {
		case 0:
		case 1:
			ns := r.count(1)
			f.Start = make([]uint32, ns)
			for j := range f.Start {
				f.Start[j] = r.u32()
			}
			f.Trans = make([][]Range, n)
			for j := range f.Trans {
				m := r.count(3)
				if m == 0 {
					continue
				}
				f.Trans[j] = make([]Range, m)
				for k := range f.Trans[j] {
					lo := r.u32()
					d := r.u32()
					tv := r.uvarint()
					if r.err == nil && (uint64(lo)+uint64(d) > 1<<32-1 || tv>>1 >= uint64(n)) {
						r.err = bad("range or target out of bounds")
					}
					f.Trans[j][k] = Range{Lo: lo, Hi: lo + d, Target: uint32(tv >> 1), Skip: tv&1 != 0}
				}
			}
		default:
			return nil, bad("own-ranges flag %d", flag)
		}
		if r.err != nil {
			return nil, r.err
		}
	}
	if len(r.b) != 0 {
		return nil, bad("%d trailing bytes", len(r.b))
	}
	if err := t.validate(); err != nil {
		return nil, err
	}
	return t, nil
}

var (
	regMu sync.RWMutex
	reg   = map[[32]byte][]byte{}
)

// Register stores the encoded record for the blob with the given SHA-256. A
// second record for the same blob panics, because it would hide an error in
// the code that registers them.
func Register(blobSHA256 [32]byte, encoded []byte) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := reg[blobSHA256]; dup {
		panic(fmt.Sprintf("lexexact: record for blob %x registered twice", blobSHA256))
	}
	reg[blobSHA256] = encoded
}

// Lookup returns the encoded record registered for the blob, if any.
func Lookup(blobSHA256 [32]byte) ([]byte, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	b, ok := reg[blobSHA256]
	return b, ok
}

// MustHash decodes a 64-digit hexadecimal SHA-256. The generated files of
// cmd/ts2go use it to register a record; it panics on a malformed string.
func MustHash(hexSHA256 string) [32]byte {
	var h [32]byte
	b, err := hex.DecodeString(hexSHA256)
	if err != nil || len(b) != len(h) {
		panic("lexexact: bad hash " + hexSHA256)
	}
	copy(h[:], b)
	return h
}
