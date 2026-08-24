# Optimize the UTF-16 ASCII source-map path

Work only in this repository. Return one valid unified diff. Do not add Markdown fences or explanatory text.

Implement a bounded fast path for ASCII runs in `encodeUTF16ToUTF8WithMap`.

The current loop calls the general UTF-16 decoder and UTF-8 encoder for every code unit. ASCII editor buffers are common. The existing benchmark reports approximately 4.2 to 5.1 microseconds, 7,488 bytes, and eight allocations per 512-byte conversion on this machine.

Requirements:

- Preserve the exact output and every map boundary.
- Preserve all newline line-start entries.
- Preserve surrogate-pair and unpaired-surrogate behavior.
- Preserve mixed ASCII and non-ASCII behavior.
- Keep the general path clear and bounded.
- Do not change public APIs.
- Do not change parser, compact replay, Generalized LR parsing, grammar, or campaign files.
- Limit production changes to `utf16.go`.
- Add focused tests in `utf16_test.go` for an ASCII run with multiple newlines and a mixed ASCII, Basic Multilingual Plane, surrogate-pair, and unpaired-surrogate source.
- Update `utf16_benchmark_test.go` only if needed to measure both ASCII and mixed inputs.
- Do not weaken or delete tests.
- Keep allocations at or below the current benchmark count.
- Keep the change only if repeated benchmark evidence shows a material ASCII speed improvement without a material mixed-input regression.

Validate with these commands:

```text
go test . -run '^(TestDecodeUTF16Bytes|TestEncodeUTF16ToUTF8WithMap|TestUTF16Points|TestIncludedRangesForUTF16)' -count=20
GOMAXPROCS=1 go test . -run '^$' -bench '^BenchmarkUTF16SourceMapConversion' -benchmem -count=10 -benchtime=500ms
go test . -run '^TestEncodeUTF16ToUTF8WithMap' -race -count=1
go vet ./...
```

Return only the unified diff.
