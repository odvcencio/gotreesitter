package bench_test

// Copies of declarations from root test files that stay in the root package.
// Each entry names its source file in a "// from" comment. The root package
// keeps the originals for the tests that remain there.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// from benchmark_test.go
func makeGoBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 48)
	sb.WriteString("package main\n\n")
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "func f%d() int { v := %d; return v }\n", i, i)
	}
	return []byte(sb.String())
}

// from benchmark_test.go
func pointAtOffset(src []byte, offset int) gotreesitter.Point {
	var row uint32
	var col uint32
	for i := 0; i < offset && i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
		i += size
	}
	return gotreesitter.Point{Row: row, Column: col}
}

// from benchmark_test.go
func benchmarkFuncCount(b *testing.B) int {
	if raw := strings.TrimSpace(os.Getenv("GOT_BENCH_FUNC_COUNT")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			return n
		}
		b.Fatalf("invalid GOT_BENCH_FUNC_COUNT=%q", raw)
	}
	if testing.Short() {
		return 100
	}
	return 500
}

// from benchmark_test.go
func mustGoTokenSource(tb testing.TB, src []byte, lang *gotreesitter.Language) *grammars.GoTokenSource {
	tb.Helper()
	ts, err := grammars.NewGoTokenSource(src, lang)
	if err != nil {
		tb.Fatalf("NewGoTokenSource failed: %v", err)
	}
	return ts
}
