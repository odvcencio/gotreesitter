package parsercorephase0

import (
	"fmt"
	"testing"
)

func BenchmarkCheckpointIntern(b *testing.B) {
	for _, size := range []int{8, 64, 1024} {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			states := make([][]byte, 64)
			for index := range states {
				states[index] = make([]byte, size)
				states[index][0] = byte(index)
			}
			b.Run("repeat", func(b *testing.B) {
				interner := newCheckpointInterner(64, uint64(64*size))
				for _, state := range states {
					if _, err := interner.intern(state); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.SetBytes(int64(size))
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					if _, err := interner.intern(states[index%len(states)]); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("new", func(b *testing.B) {
				interner := newCheckpointInterner(64, uint64(64*size))
				b.ReportAllocs()
				b.SetBytes(int64(size * len(states)))
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					interner.reset()
					for _, state := range states {
						if _, err := interner.intern(state); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		})
	}
}
