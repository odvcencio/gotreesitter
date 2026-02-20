# gotreesitter

Pure-Go tree-sitter runtime and grammar pipeline.

## Current Status

`go run ./cmd/parity_report`

- coverage: `parseable=25 total=25 unsupported=0`
- all manifest languages now produce a tree
- partial-backend languages are reported honestly as `ok` or `degraded` (instead of hard-failing on nil-root)

## Supported Grammars (Current)

- `bash` (`dfa-partial`, `ok`)
- `c` (`token_source`, `ok`)
- `cpp` (`token_source`, `ok`)
- `css` (`dfa-partial`, `ok`)
- `elixir` (`dfa-partial`, `degraded`)
- `go` (`token_source`, `ok`)
- `html` (`token_source`, `ok`)
- `java` (`token_source`, `ok`)
- `javascript` (`token_source`, `ok`)
- `json` (`token_source`, `ok`)
- `kotlin` (`dfa-partial`, `ok`)
- `lua` (`token_source`, `ok`)
- `nix` (`dfa-partial`, `ok`)
- `php` (`dfa-partial`, `ok`)
- `python` (`dfa-partial`, `ok`)
- `ruby` (`dfa-partial`, `ok`)
- `rust` (`token_source`, `ok`)
- `scala` (`dfa-partial`, `ok`)
- `sql` (`dfa-partial`, `ok`)
- `swift` (`dfa`, `ok`)
- `toml` (`token_source`, `ok`)
- `tsx` (`dfa-partial`, `ok`)
- `typescript` (`token_source`, `ok`)
- `yaml` (`dfa-partial`, `degraded`)
- `zig` (`dfa`, `ok`)

## Benchmarks

Benchmarks were run on:

- `goos: linux`
- `goarch: amd64`
- `cpu: Intel(R) Core(TM) Ultra 9 285`

Command:

```bash
go test -run '^$' -tags treesitter_c_bench -bench 'Benchmark(GoParse|CTreeSitterGoParse)' -benchmem
```

Results:

| Benchmark | ns/op | MB/s | B/op | allocs/op |
| --- | ---:| ---:| ---:| ---:|
| `BenchmarkCTreeSitterGoParseFull` | 1,960,803 | 9.84 | 600 | 6 |
| `BenchmarkCTreeSitterGoParseIncrementalSingleByteEdit` | 116,598 | 165.47 | 648 | 7 |
| `BenchmarkCTreeSitterGoParseIncrementalNoEdit` | 114,872 | 167.96 | 600 | 6 |
| `BenchmarkGoParseFull` | 813,371 | 23.72 | 12,698 | 2,495 |
| `BenchmarkGoParseIncrementalSingleByteEdit` | 6,360 | 3,033.59 | 193 | 6 |
| `BenchmarkGoParseIncrementalNoEdit` | 8.126 | 2,374,311.41 | 0 | 0 |

## Query API Status

Implemented:

- query compile + execute (`NewQuery`, `Execute`, `ExecuteNode`)
- cursor-style streaming (`Exec`, `NextMatch`, `NextCapture`)
- predicates: `#eq?`, `#match?`
- structural quantifiers: `?`, `*`, `+`

Important API note:

- `ExecuteNode` requires `source []byte` for correct text-predicate evaluation:
  - `ExecuteNode(node, lang, source)`
  - passing `nil` source disables text-based predicate checks for that call

## Expansion Path (In Progress)

The manifest pipeline is now a repeatable expansion path:

1. Add a grammar repo to `grammars/languages.manifest`.
2. Generate grammar/runtime bindings:

```bash
go run ./cmd/ts2go -manifest grammars/languages.manifest -outdir ./grammars -package grammars
```

3. Add/adjust smoke samples in:
- `cmd/parity_report/main.go`
- `grammars/parse_support_test.go`

4. Evaluate support and status:

```bash
go run ./cmd/parity_report
```

Recent expansion in this repo:

- milestone: `21/21 parseable`
- current: `25/25 parseable`
- newly added and registered in this batch: `zig`, `scala`, `elixir`, `nix`
- generated but not yet registered (missing highlight query stub from source repos): `graphql`, `hcl`
