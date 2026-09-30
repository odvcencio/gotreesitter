//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

var verifyMemoSizes = []struct {
	name  string
	bytes int
}{
	{"32KiB", 32 * 1024},
	{"137KiB", 137 * 1024},
	{"1MiB", 1024 * 1024},
}

func TestIncrementalVerifyMemoSizesLockedC(t *testing.T) {
	for _, size := range verifyMemoSizes {
		t.Run(size.name, func(t *testing.T) {
			verifyMemoLockedC(t, size.bytes)
		})
	}
}

func BenchmarkIncrementalVerifyMemoSizes(b *testing.B) {
	for _, index := range verifyCostBenchOrder(b.Name(), len(verifyMemoSizes)) {
		size := verifyMemoSizes[index]
		b.Run(size.name, func(b *testing.B) {
			benchmarkIncrementalVerifyMemo(b, size.bytes)
		})
	}
}
