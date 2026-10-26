package parsercorephase0

import "math/bits"

// ScannerTrialMask owns a frozen large trial mask without mutable slice backing.
// Zero is not packed; short and oversized masks keep the existing representation.
type ScannerTrialMask struct {
	words  [2]uint64
	length uint8
}

func PackScannerTrialMask(src []bool) (ScannerTrialMask, bool) {
	if len(src) <= 64 || len(src) > 128 {
		return ScannerTrialMask{}, false
	}
	out := ScannerTrialMask{length: uint8(len(src))}
	for i, value := range src {
		if value {
			out.words[i>>6] |= uint64(1) << uint(i&63)
		}
	}
	return out, true
}

func (m ScannerTrialMask) Len() int { return int(m.length) }

// Restore reuses the destination's private backing and clears stale true flags.
func (m ScannerTrialMask) Restore(dst []bool) []bool {
	if cap(dst) < int(m.length) {
		dst = make([]bool, int(m.length))
	} else {
		dst = dst[:m.length]
		clear(dst)
	}
	for wordIndex, word := range m.words {
		for word != 0 {
			index := wordIndex*64 + bits.TrailingZeros64(word)
			dst[index] = true
			word &= word - 1
		}
	}
	return dst
}
