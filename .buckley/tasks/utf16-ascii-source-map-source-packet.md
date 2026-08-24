# Optimize the real UTF-16 source-map loop

Return one valid unified diff. The first byte must start `diff --git `. Do not add a Markdown fence, analysis, or alternatives.

Use the exact repository code below. The package is `gotreesitter`. The source map is not a flat integer slice.

Only change these files:

- `utf16.go`
- `utf16_test.go`
- `utf16_benchmark_test.go`, only when a mixed-input benchmark adds useful evidence

Do not change a public API. Do not change parser, compact replay, Generalized LR parsing, grammar, or campaign files.

Implement one bounded optimization. Scan each contiguous ASCII run inside `encodeUTF16ToUTF8WithMap`. Avoid `decodeUTF16Rune` and `utf8.EncodeRune` for that run. Preserve these exact facts for every ASCII unit:

- The output byte equals the unit.
- `unitToByte` maps both unit boundaries to their UTF-8 byte offsets.
- Both unit boundaries remain valid.
- `byteToUnit` gains the ending unit offset.
- The byte boundary remains valid.
- A newline adds its ending unit and byte offsets to both line-start slices.

Continue through the general path at each non-ASCII unit. Preserve Basic Multilingual Plane characters, valid surrogate pairs, and unpaired surrogate replacement.

This is the exact current type and function:

```go
type utf16SourceMap struct {
	source []uint16
	utf8 []byte
	byteToUnit []uint32
	byteBoundary []bool
	unitToByte []uint32
	unitBoundary []bool
	lineStartUnits []uint32
	lineStartBytes []uint32
}

func encodeUTF16ToUTF8WithMap(source []uint16) ([]byte, *utf16SourceMap) {
	lineStartCap := min(len(source)+1, 128)
	m := &utf16SourceMap{
		source:         source,
		unitToByte:     make([]uint32, len(source)+1),
		unitBoundary:   make([]bool, len(source)+1),
		byteToUnit:     make([]uint32, 1, len(source)+1),
		byteBoundary:   make([]bool, 1, len(source)+1),
		lineStartUnits: make([]uint32, 1, lineStartCap),
		lineStartBytes: make([]uint32, 1, lineStartCap),
	}
	m.byteBoundary[0] = true
	m.unitBoundary[0] = true
	if len(source) == 0 {
		return nil, m
	}

	out := make([]byte, 0, len(source))
	for unitPos := 0; unitPos < len(source); {
		unitStart := unitPos
		byteStart := len(out)
		r, unitSize := decodeUTF16Rune(source, unitPos)
		unitPos += unitSize

		var encoded [utf8.UTFMax]byte
		byteSize := utf8.EncodeRune(encoded[:], r)
		out = append(out, encoded[:byteSize]...)

		unitEnd := unitStart + unitSize
		byteEnd := byteStart + byteSize
		m.unitToByte[unitStart] = uint32(byteStart)
		for u := unitStart + 1; u < unitEnd; u++ {
			m.unitToByte[u] = uint32(byteStart)
		}
		m.unitToByte[unitEnd] = uint32(byteEnd)
		m.unitBoundary[unitStart] = true
		m.unitBoundary[unitEnd] = true

		for i := 1; i <= byteSize; i++ {
			unit := uint32(unitStart)
			if i == byteSize {
				unit = uint32(unitEnd)
			}
			m.byteToUnit = append(m.byteToUnit, unit)
			m.byteBoundary = append(m.byteBoundary, i == byteSize)
		}
		if r == '\n' {
			m.lineStartUnits = append(m.lineStartUnits, uint32(unitEnd))
			m.lineStartBytes = append(m.lineStartBytes, uint32(byteEnd))
		}
	}
	m.utf8 = out
	return out, m
}
```

Add focused tests that prove these cases:

- An all-ASCII source has several newlines. Every offset round-trips with identity mapping.
- A mixed source contains ASCII runs before, between, and after a Basic Multilingual Plane rune, a surrogate pair, and an unpaired surrogate.
- Internal UTF-8 bytes and the middle surrogate unit remain invalid boundaries.
- Line starts retain the correct UTF-16 units and UTF-8 bytes across mixed-width text.

The current ASCII conversion benchmark reports approximately 4.2 to 5.1 microseconds, 7,488 bytes, and eight allocations per operation on this machine. Keep allocations at or below eight. A useful patch must materially improve repeated ASCII timing and avoid a material mixed-input regression.

Validate with:

```text
go test . -run '^(TestDecodeUTF16Bytes|TestEncodeUTF16ToUTF8WithMap|TestUTF16Points|TestIncludedRangesForUTF16)' -count=20
GOMAXPROCS=1 go test . -run '^$' -bench '^BenchmarkUTF16SourceMapConversion' -benchmem -count=10 -benchtime=500ms
go test . -run '^TestEncodeUTF16ToUTF8WithMap' -race -count=1
go vet ./...
```

Return only the unified diff.
