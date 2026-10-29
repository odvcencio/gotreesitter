//go:build cgo && (treesitter_c_parity || treesitter_c_bench)

package cgoharness

import (
	"unicode/utf8"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

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

type benchmarkEditSite struct {
	offset int
	start  gotreesitter.Point
	end    gotreesitter.Point
}
