# BENCH — canonical performance claims and how to reproduce them

This is the single authoritative page for gotreesitter performance claims.
Every number here is either pinned to a release receipt in
[CHANGELOG.md](CHANGELOG.md) or derived from the ratcheted fleet ledger in
[`cgo_harness/perf_scan`](cgo_harness/perf_scan/). Anything not on this page
is not a claim.

## The one-paragraph story

gotreesitter trades some raw full-parse speed for portability: pure Go, no
cgo, cross-compiles anywhere Go does (including `wasip1`), fully visible to
`go test -race`. Editor-style incremental workloads are where it is fast
outright — a no-edit reparse is nanoseconds and a one-byte edit is
microsecond-scale on the historical control, both zero-allocation. Full parses
are ratcheted against the C runtime language-by-language with explicit
caveats instead of averaged marketing numbers.

## Canonical benchmark status

The generated 500-function Go source is a historical straight-LR control. It
contains no imports, selectors, methods, types, comments, strings, or control
flow and never forks under the current parser. It remains useful for tracking
the incremental fast paths and single-stack regressions, but it is not a
representative full-parse headline.

The former **1.895x C** full-parse headline and its **29% materialization**
decomposition are withdrawn in favor of the locked real-code replacement
below. The old comparison also used different Go grammar artifacts:
gotreesitter used the project-locked 1,425-state/214-symbol grammar while the C
benchmark used a 1,404-state/212-symbol grammar bundled by the old smacker
binding.

Historical control results, retained as workload-specific receipts:

| Lane | Benchmark | Historical result |
|---|---|---|
| Full parse (materialized, straight LR) | `BenchmarkGoParseFullDFA` | 10.907 ms on the pinned quiet host |
| One-byte incremental edit | `BenchmarkGoParseIncrementalSingleByteEditDFA` | 649 ns/op, 0 allocs |
| No-edit reparse | `BenchmarkGoParseIncrementalNoEditDFA` | 2.43 ns/op, 0 allocs |

Reproduce the historical control:

```sh
GOMAXPROCS=1 go test . -run '^$' \
  -bench 'BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA' \
  -benchmem -count=10 -benchtime=750ms
```

`BenchmarkGoParseCoreDFA` is a parser-loop diagnostic (no tree
materialization); its numbers are never quoted as full-parse numbers. See
the benchmark-integrity note below.

### Historical quiet-host receipt

The v0.24.1 audit withdrew the pre-correction full-parse headlines pending a
quiet-host rerun of the corrected public benchmark. First such receipt,
2026-07-12, main @ 04f75d15, Intel Xeon D-2141I @ 2.20 GHz (idle host,
`taskset -c 14`, `GOMAXPROCS=1`, `-count=10 -benchtime=750ms`), medians:

| Lane | ns/op | B/op | allocs/op |
|---|---|---|---|
| `BenchmarkGoParseFullDFA` | 12,245,000 | 1,527 | **9** |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 1,976 | 0 | **0** |
| `BenchmarkGoParseIncrementalNoEditDFA` | 9.85 | 0 | **0** |
| `BenchmarkGoParseCoreDFA` (diagnostic) | 8,737,000 | 996 | 6 |

Wall-clock numbers are host-specific (this is a low-clock server part; do not
compare against dev-box history). The allocation counts remain valid for this
fixture. The full-minus-core decomposition is not generalized beyond this
straight-LR control.

### v0.27.0 combined receipt

Same host and pinned core, the historical full parse measured 10.907 ms and
the mismatched-grammar C baseline measured 5.756 ms. Their former 1.895x ratio
is recorded only to explain earlier releases; it is not a current claim.

### Withdrawn same-host C calibration

The old C baselines were run on the same host and workload, but through the
smacker binding and its different Go grammar. Same-host scheduling does not
repair an oracle-identity mismatch. These rows are historical only:

| Lane | pure Go | cgo binding (C) | Go / C |
|---|---|---|---|
| Full parse (materialized) | 12.25 ms | 5.72 ms | **2.14x** |
| One-byte incremental edit | 1.98 µs | 331 µs | **0.006x — 167x faster** |
| No-edit reparse | 9.9 ns | 330 µs | **~33,000x faster** |

`BenchmarkGoParseFull` is also not the registry production route: built-in Go
uses the generated DFA path, while the hand-written Go token source remains an
explicit alternate. Its old 1.70x row is therefore withdrawn as both
oracle-mismatched and mislabeled.

## The one C oracle

Every new "versus C" claim names and fingerprints the same oracle inputs:

- upstream tree-sitter runtime v0.25.1, commit
  `f5afe475deb7c0bae6407fb776c76824f717bb61`;
- `github.com/tree-sitter/go-tree-sitter v0.25.0` wrapper commit
  `adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3` for in-process parity;
- tree-sitter-go commit
  `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`, from
  `grammars/languages.lock`;
- C runtime and grammar compiled with `-O2` into the static publication
  artifact, with compiler identity, flags, and artifact SHA-256 in the receipt.

The in-process cgo parity transport and the static throughput artifact must
consume those same runtime and grammar sources. The former
`treesitter_c_bench`/`treesitter_c_parity` binding split is a harness defect,
not an accepted source of two oracles.

## Canonical real-Go full-parse matrix

The replacement headline uses immutable snapshots of clean, human-authored Go
that exercise genuine GLR forking:

| Fixture | Bytes | SHA-256 |
|---|---:|---|
| `rewrite.go` | 5,116 | `74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097` |
| `query_compile.go` | 20,168 | `b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894` |
| `language.go` | 41,387 | `009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a` |
| `grammargen/lr.go` | 235,626 | `a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2` |

These reach 12-18 live stacks, thousands to tens of thousands of multi-stack
iterations, and constructed-to-selected-node ratios of 3.65-4.47 on the
admitting revision. Generated code is reported separately and never blended
into the human-code headline. A pinned external-project fixture will be added
to check repository self-reference.

Produce the complete publication receipt from a clean worktree on a quiet
Docker-capable host:

```sh
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh --core <idle-cpu>
```

The driver authenticates and materializes the four snapshots, admits exact Go,
cgo, and static-C trees, builds the locked `-O2` oracle, and collects ten
process-isolated samples per backend and fixture in five Go-C-C-Go cycles. It
fails closed on dirty source, parser or Go runtime overrides, noisy-host
admission, identity drift, and incomplete receipts. Shortened or skipped gates
require `--diagnostic` and are labeled `NONPUBLICATION_DIAGNOSTIC`.

### First locked-oracle publication receipt

The first complete publication receipt was collected on 2026-07-14 from
`3ffad7778199a17270efe6791d09036242667233` on the pinned quiet host, using
core 7 and Go 1.22.2. All four Go, cgo, and static-C deep trees matched before
timing. Medians from ten process-isolated samples per backend and fixture:

| Fixture | Go median | static C median | Go / C | Go max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|
| `query_compile.go` | 31.525 ms | 5.469 ms | **5.764366x** | 67,560 KiB | 2,816 KiB |
| `rewrite.go` | 5.556 ms | 1.197 ms | **4.639849x** | 56,344 KiB | 2,304 KiB |
| `language.go` | 30.106 ms | 5.809 ms | **5.182694x** | 69,136 KiB | 2,816 KiB |
| `grammargen/lr.go` | 376.938 ms | 57.867 ms | **6.513909x** | 192,560 KiB | 9,216 KiB |

The canonical equal-fixture geomean is **5.481673x C**. The fixed-suite sum of
medians is **6.313799x C**; it is reported separately because the largest file
dominates aggregate wall time. This is the first locked-oracle historical
baseline, not a retrospective adjustment to the withdrawn straight-LR ratio;
the exact-revision production and candidate receipt is recorded below.

Receipt identities:

- manifest SHA-256:
  `a850d60ec93c2ff3a49b3ccf9266f32421c04a87c5cb5ce807f2f4fe3f70c1cb`;
- report SHA-256:
  `36fded16ad1ffa34eab8d4f3e61d0a634bcc30e70237101825f73394d797c1b8`;
- complete receipt bundle SHA-256:
  `aecb7f76e3df4832877f4526280849257826f3d84f2f104379a57748f4f6f310`;
- static C artifact SHA-256:
  `dfbed45811491be8d81e32b293ed5577222445dae47b67d876cedae09679a871`.

### v0.39.0 production-code baseline (historical)

An authenticated production receipt was collected on 2026-07-17 from the same
quiet host, core 6, and Go 1.22.2. The worktree was clean production code at
`2c7026563f3827da87e637bcb246d4a8f287c022`; its `PUBLICATION` receipt measures
the public `Parser.Parse` lifecycle, completeness check, and `Tree.Release`.

| Fixture | Go median | static C median | Go / C | B/op | allocs/op | Go max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 29.313 ms | 5.591 ms | **5.242927x** | 157,657 | 13 | 68,528 KiB | 2,816 KiB |
| `rewrite.go` | 5.110 ms | 1.260 ms | **4.055251x** | 2,040 | 14 | 54,052 KiB | 2,304 KiB |
| `language.go` | 28.383 ms | 5.980 ms | **4.746044x** | 163,060 | 32 | 67,620 KiB | 2,816 KiB |
| `grammargen/lr.go` | 345.658 ms | 61.198 ms | **5.648204x** | 13,165,973 | 561.5 | 205,716 KiB | 9,216 KiB |

The v0.39.0 public-parser baseline is **4.886056x C** by equal-fixture
geomean. Its fixed-suite sum of medians is **5.517602x C** (408.464 ms Go
versus 74.029 ms static C), and its worst fixture is `grammargen/lr.go` at
**5.648204x C**.

Receipt identities:

- manifest SHA-256:
  `8933650e446b14db263e165bbfed0bd68cebeb15898b7b818c57040e10b31b46`;
- report SHA-256:
  `19a68af15fe1f413e628d3f5a7aed0b7fb975652028aee685d5b30769151d2f3`;
- complete receipt bundle SHA-256:
  `3c529e002cee1ab28e7ec11edc826d3b31b298dec616e14aa0fd405e11c84a09`;
- static C artifact SHA-256:
  `dfbed45811491be8d81e32b293ed5577222445dae47b67d876cedae09679a871`.

### v0.40.0 production-code baseline (current)

The current authenticated production receipt was collected on 2026-07-18 from
the clean v0.40.0 tag target
`1935a42c1ecc68a40147f0e13dc90bcd4b23f1b7` on `ns1007492`, CPU 14, with Go
1.22.2 and `GOMAXPROCS=1`. The strict quiet-host and cgo/static-C deep-tree
admission gates passed before five Go-C-C-Go cycles produced ten samples per
backend and fixture. It uses the same locked static `-O2` C artifact and
measures the public fresh, materialized parser lifecycle.

| Fixture | Go median | static C median | Go / C | B/op | allocs/op | Go max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 27.9760085 ms | 5.439447625 ms | **5.143171x** | 154,878.5 | 13 | 68,900 KiB | 2,816 KiB |
| `rewrite.go` | 4.9456765 ms | 1.206203075 ms | **4.100202x** | 2,040 | 14 | 54,196 KiB | 2,304 KiB |
| `language.go` | 27.1806765 ms | 5.80478855 ms | **4.682458x** | 153,665 | 32 | 68,432 KiB | 2,816 KiB |
| `grammargen/lr.go` | 331.409954 ms | 59.0925574 ms | **5.608320x** | 13,165,874.5 | 560.5 | 206,868 KiB | 9,216 KiB |

The v0.40.0 public-parser baseline is **4.851050x C** by equal-fixture
geomean. Its fixed-suite sum of medians is **5.472406x C** (391.5123155 ms Go
versus 71.54299665 ms static C), and its worst fixture is `grammargen/lr.go` at
**5.608320x C**. The geomean is only **0.716%** better than v0.39.0's
4.886056x result, so this receipt does not satisfy the project's reproducible
2% performance-win threshold and is a baseline refresh, not a banked win.

Receipt identities:

- manifest SHA-256:
  `c5eaf477072d9a89a592b1f983b58ac6d733ea3a48c50622fb2a3201c892b600`;
- report SHA-256:
  `4b92a5fc576f9253984c4ecbceb4ca9c280c994cab8d1d0716931ad07788e915`;
- complete receipt bundle SHA-256:
  `7dfc311ad9f3e2f098d02752f5b515d41fc6988e40e54b1ab365f88640d3e5cf`;
- static C artifact SHA-256:
  `dfbed45811491be8d81e32b293ed5577222445dae47b67d876cedae09679a871`.

For incremental parsing, the versioned, hash-locked admission matrix documented
in [`cgo_harness/README.md`](cgo_harness/README.md#run-locked-canonical-incremental-benchmarks)
validates exact correctness and classifies identity, leaf validation, real-code
GLR, recovery, and scanner-state work. It publishes no general comparative
incremental speed headline.

### Exact-revision production and post-fusion compact-candidate receipt

A paired strict receipt was collected on 2026-07-17 from quiet host
`ns1007492` and Go 1.22.2. Both worktrees were clean at exact post-fusion
revision `708c665f762f85ea07a72e3ffb31581f8d622a93`. The `PUBLICATION` control
measures the public `Parser.Parse` lifecycle, completeness check, and
`Tree.Release`.

| Fixture | Go median | static C median | Go / C | B/op | allocs/op | Go max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 27.890 ms | 5.402 ms | **5.163424x** | 152,098 | 13 | 65,728 KiB | 2,816 KiB |
| `rewrite.go` | 4.887 ms | 1.202 ms | **4.065670x** | 2,040 | 14 | 54,084 KiB | 2,304 KiB |
| `language.go` | 26.745 ms | 5.804 ms | **4.607774x** | 149,581 | 32 | 71,396 KiB | 2,816 KiB |
| `grammargen/lr.go` | 331.145 ms | 59.675 ms | **5.549179x** | 13,166,466 | 564.5 | 205,724 KiB | 9,216 KiB |

The same-revision public-parser control is **4.813350x C** by equal-fixture
geomean. Its fixed-suite sum of medians is **5.419730x C** (390.667 ms Go
versus 72.082 ms static C), and its worst fixture is `grammargen/lr.go` at
**5.549179x C**.

Production receipt identities:

- manifest SHA-256:
  `575f1eab1ed29eeae680baab33686c5a43bb7636a873c6ee23d32d41cc0a6363`;
- report SHA-256:
  `f44a77007142c24ad5b1ec6d386fb19b417f2d1ba4f1b32c9058e39bfd87b80d`;
- complete receipt archive SHA-256:
  `937a83b9551ec7c2a1e65c36fcc8c5e13cada8f412434b2db87a3a7eca3d862f`.

The same revision adds a separate build-tagged compact backend:

```sh
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh \
  --go-backend candidate --core <idle-cpu>
```

Its receipt class is `AUTHENTICATED_CANDIDATE`, its build tag is
`gts_parsercorephase0`, and its measured lifecycle is
`parserCoreFreshFullRunner.parse + shallow completeness + Tree.Release`.
Fallback policy is `none_fail_closed`; all 40 timed Go samples reported zero
fallback and repeat-identical per-fixture work counts.

| Fixture | Candidate median | static C median | Candidate / C | B/op | allocs/op | Candidate max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 21.495 ms | 5.436 ms | **3.954119x** | 231,446 | 9,032 | 50,972 KiB | 2,816 KiB |
| `rewrite.go` | 4.359 ms | 1.200 ms | **3.633407x** | 56,193 | 1,935 | 52,972 KiB | 2,304 KiB |
| `language.go` | 22.016 ms | 5.801 ms | **3.794892x** | 275,011 | 9,279 | 56,108 KiB | 2,816 KiB |
| `grammargen/lr.go` | 236.025 ms | 58.739 ms | **4.018193x** | 2,415,402 | 91,202 | 94,400 KiB | 9,216 KiB |

The candidate equal-fixture geomean is **3.847233x C**. Its fixed-suite sum of
medians is **3.988613x C** (283.895 ms candidate versus 71.176 ms static C), and
its worst fixture is `grammargen/lr.go` at **4.018193x C**. Comparing the paired
ratios, the candidate improves the equal-fixture geomean by **20.07%** and the
fixed-suite sum by **26.41%**. The per-fixture ratio reductions are 23.42%,
10.63%, 17.64%, and 27.59% in table order. Candidate allocation count is not a
win; its lower elapsed time and lower large-fixture RSS arise despite many
small allocations.

Candidate receipt identities:

- manifest SHA-256:
  `513a1e0dcec8c0214c0abcc57e3563ec4ed43a5ab369aa5092867746f0aec50a`;
- report SHA-256:
  `4ff459035520685c6d01a38df738105aae1121edbe33248d66a7b7603e4e3813`;
- complete receipt archive SHA-256:
  `b8e543de18769059faf7be0f2adc84009898e48e5e44d8063c8bc3c2fb10940e`;
- static C artifact SHA-256:
  `dfbed45811491be8d81e32b293ed5577222445dae47b67d876cedae09679a871`.

“Exact” for this candidate means `gts-deep-tree-v1` visible structural equality:
selected syntax, spans, points, fields, aliases/extras, EOF, and selected-node
census match the locked oracle on the four clean fresh-UTF-8 fixtures. The only
admitted grammar/scanner identity is the embedded Go blob with SHA-256
`9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08`
and the exact `github.com/odvcencio/gotreesitter/grammars.GoExternalScanner`
type. The route declines recovery or retry, incremental parsing, included
ranges, closed-prefix operation, missing or no-lookahead tokens, repetition
shifts, extra chains, and any EOF frontier other than one accepted head with
one exact derivation. Other unsupported semantics also fail closed. The digest
does not cover `ParseState` or `PreGotoState`, and the compact materializer
currently writes zero for both. The candidate has not admitted query/cursor
behavior or multiple grammars. Therefore **3.847233x C is a branch-only
diagnostic compact-candidate result, not a replacement public `Parser.Parse`
claim**; the same-revision production control is **4.813350x C**.

### Compact candidate action-row dispatch receipt

A clean authenticated candidate receipt was collected on 2026-07-18 from
quiet host `ns1007492`, pinned to CPU 12 with Go 1.22.2, at exact revision
`24f96df21155a77625a5031fa02ff64a58d4a128`. It uses the same static `-O2`
oracle contract and artifact described above, passed cgo/static deep admission,
and reported zero fallback with repeat-identical parser work on every timed
sample.

| Fixture | Candidate median | static C median | Candidate / C | B/op | allocs/op | Candidate max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 20.734 ms | 5.492 ms | **3.775635x** | 231,222 | 9,032 | 48,888 KiB | 2,816 KiB |
| `rewrite.go` | 4.261 ms | 1.218 ms | **3.499159x** | 55,969 | 1,935 | 53,232 KiB | 2,304 KiB |
| `language.go` | 21.433 ms | 5.806 ms | **3.691434x** | 274,803 | 9,279.5 | 56,800 KiB | 2,816 KiB |
| `grammargen/lr.go` | 226.958 ms | 58.572 ms | **3.874884x** | 2,415,107 | 91,204 | 92,948 KiB | 9,216 KiB |

The candidate equal-fixture geomean is **3.707677x C**. Its fixed-suite sum of
medians is **3.845796x C** (273.387 ms candidate versus 71.087 ms static C),
and every fixture is below 3.90x C.

Before banking the change, two serialized reverse-order quiet-host A/B boards
compared the exact pre-change revision `6354d0e357481de1cb13e69ebd76373fc336b325`
with the action-row descriptor change. Each board used ten 750 ms samples per
fixture on CPU 12. The rows below compare Go medians directly, so static-C
sample jitter cannot create the reported improvement.

| Fixture | Board 1 base | Board 1 candidate | Improvement | Board 2 base | Board 2 candidate | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 21.538 ms | 20.759 ms | **3.62%** | 21.440 ms | 20.600 ms | **3.92%** |
| `rewrite.go` | 4.389 ms | 4.182 ms | **4.71%** | 4.363 ms | 4.214 ms | **3.42%** |
| `language.go` | 22.156 ms | 21.047 ms | **5.00%** | 22.196 ms | 21.105 ms | **4.91%** |
| `grammargen/lr.go` | 235.346 ms | 226.635 ms | **3.70%** | 236.572 ms | 226.548 ms | **4.24%** |

The equal-fixture Go-time improvements are **4.2603%** and **4.1258%**. Exact
tree, parser-work, and fallback receipts are unchanged. The A/B runs are
labeled diagnostic because the candidate side was
an uncommitted worktree; the clean authenticated receipt above independently
binds the final code and oracle identities.

Receipt identities:

- clean revision manifest SHA-256:
  `670d415e4fd17ac11de1ab0438701b0ba1f0337312d104877ff9d2da99091085`;
- clean revision report SHA-256:
  `eba3e7e5926c7d7c6d61b37066f45036892593cfcc4ba8bf9b16ab2a6b3d6d26`;
- clean revision archive SHA-256:
  `a050e7f12a8842a9dcb30729e6f69c23c8078bae44b989c1318b20276d4b91f2`;
- Board 1 base manifest/report/archive SHA-256:
  `d101f941ff880e4d42ee1d287c93f93b3e792eea78588b393079c53c880502e5`,
  `949e2b09530289f2b82bde91a924ea721d7f62020e5e6dd471de794c065ea123`,
  `c07237ae59d22637bd52015c19c07c9abb9bb427a99f3e95645b06d307388944`;
- Board 1 candidate manifest/report/archive SHA-256:
  `f727877c0e193e49e23afe96a67790b089e750dbb9820b4dcd70b7fc3d169b9c`,
  `4b6d99d6fbd98ce81b8bb8ee8833d8810acc55a9380762483e03e990e22dd412`,
  `d633fe5d3f995c43a351d5a74c143eceda0b4c9af0686c8630cdfcacaa92b821`;
- Board 2 candidate manifest/report/archive SHA-256:
  `ebf4cc932a076fef5a4a25a05cc4bb64c0d3540ee1b679033b0dcad2830e72bd`,
  `f6dfd42ca59425458c4b8bad4f3f5b6cc3462475bf4dab9b026ba505bc36fd2b`,
  `ce4192edfc80e4db912f5c305b7dbc9efed45b3050ec8bf2a797be3cdfba9614`;
- Board 2 base manifest/report/archive SHA-256:
  `61b499f24838ea0ce55a29b3f4b44f6dc375c5bf66d9b6427c82ff97ceffc87a`,
  `6360db575c24459b1f41bef3826b87ea9ddc166bde6773eb511041671dc3873e`,
  `77edb152f746bed600b4c1c779409efad3c8dd1f1036ae443cc93aba2283c4b9`.

The support boundary remains unchanged: this is a diagnostic clean fresh-full
Go candidate, not a public `Parser.Parse`, recovery, incremental, included-range,
or multi-grammar claim.

### Compact candidate point-cache receipt

A clean authenticated candidate receipt was collected on 2026-07-18 from
quiet host `ns1007492`, pinned to CPU 12 with Go 1.22.2, at exact revision
`15a04d7bfac7f75dbbfd4a5c199c7fb731c0031c`. It uses the same locked static
`-O2` oracle contract, passed cgo/static deep admission, remained clean, and
reported zero fallback with repeat-identical parser work on every timed sample.

| Fixture | Candidate median | static C median | Candidate / C | B/op | allocs/op | Candidate max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 20.249 ms | 5.460 ms | **3.708633x** | 231,222 | 9,032 | 55,376 KiB | 2,816 KiB |
| `rewrite.go` | 4.102 ms | 1.197 ms | **3.425931x** | 55,969 | 1,935 | 53,084 KiB | 2,304 KiB |
| `language.go` | 20.609 ms | 5.772 ms | **3.570688x** | 274,796 | 9,279.5 | 52,820 KiB | 2,816 KiB |
| `grammargen/lr.go` | 220.607 ms | 58.490 ms | **3.771715x** | 2,415,065 | 91,202.5 | 92,868 KiB | 9,216 KiB |

The candidate equal-fixture geomean is **3.616769x C**. Its fixed-suite sum of
medians is **3.744659x C** (265.567 ms candidate versus 70.919 ms static C),
and every fixture is below 3.78x C.

The change adds a 16-entry, materialization-local exact-offset cache. Its
authenticated selected-tree census reuses **59.02-62.17%** of point lookups
across the four fixtures. Before banking it, two serialized reverse-order
quiet-host A/B boards compared the exact pre-change revision `24f96df2` with
the point cache. Each board used ten 750 ms samples per fixture on CPU 12. The
table compares Go medians directly.

| Fixture | Board 1 base | Board 1 candidate | Improvement | Board 2 base | Board 2 candidate | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 20.658 ms | 20.087 ms | **2.76%** | 20.641 ms | 20.271 ms | **1.79%** |
| `rewrite.go` | 4.220 ms | 4.098 ms | **2.87%** | 4.165 ms | 4.126 ms | **0.92%** |
| `language.go` | 21.153 ms | 20.725 ms | **2.03%** | 21.226 ms | 20.616 ms | **2.87%** |
| `grammargen/lr.go` | 225.862 ms | 221.912 ms | **1.75%** | 225.623 ms | 218.970 ms | **2.95%** |

The equal-fixture Go-time improvements are **2.3539%** and **2.1380%**. Every
fixture improves in both orders; parser work and fallback counts are unchanged.
The A/B runs are diagnostic because the candidate side was an uncommitted
worktree. The clean authenticated receipt above independently binds the final
code and oracle identities.

Receipt identities:

- clean revision manifest/report/archive SHA-256:
  `88f2bdd29295708a800d260c5dd7f719a0bf2c865ed37ccbf22e436d2e021deb`,
  `bc4c2d77bbbffc0e1bbb11f27949ba3ee44c366be3ab687d12f1fdb7ad0ce605`,
  `d1dbb349ae77c6bf57d950c679fca0e11c35a8289007366aef8219015f0aa663`;
- Board 1 candidate manifest/report/archive SHA-256:
  `c7ad2f554c8253f7d3c5617be1151847472653bfc00737ddde05de980105f111`,
  `61aa489261bc0f8bfdd138cd8c49675fd70e9ad86e6e5691bf5721d5557e30c2`,
  `49b722898d181d5c0cdc4e2224a8ffebf3cb100d02f27fa4192da242329d7e6b`;
- Board 1 base manifest/report/archive SHA-256:
  `263186ff0e8cfd725f8a4ebe32be250c9f45921d2fb439dbe4661353d94c21d7`,
  `4051bff7928928ca6a51d3dcb24ac5896a3e861432e550160fe8ba7e5414e4dc`,
  `556d0f822398c29f06c545be37bdfa08d086cf9668f86b072aef8d8f38346d4e`;
- Board 2 base manifest/report/archive SHA-256:
  `bc30841887f5baf310eb31a25dceef28f7ae48efefba97404b161bf0e41aede4`,
  `6e06ab725a81ce1fea55cb9907b705e561e20a9a821fad210575b5dc9d13dc89`,
  `ba3107380af25c80d985469ae4d9003ecea80bb9cd917f4ca36c789d4fba6c5f`;
- Board 2 candidate manifest/report/archive SHA-256:
  `50cf482dede0e48c995097e24d3013e0365af2e5286dae95b0e543bff62b1ec3`,
  `7af49e386ae656d3651cdb5c85079a36445cb37480a295ac07941f2c680128f6`,
  `a70d6e92441c599578d1c5d85ff30ff64ad763cb027c37d42f05323a58505f8f`.

The support boundary remains unchanged: this is a diagnostic clean fresh-full
Go candidate, not a public `Parser.Parse`, recovery, incremental, included-range,
or multi-grammar claim.

### Current exact-revision production and construction-authenticated compact receipt

A paired clean publication was collected on 2026-07-18 from quiet host
`ns1007492`, pinned to CPU 3 with Go 1.22.2, at exact revision
`34c567bbacab367d01e03b16bad0a4914cbc5a24`. Both lanes use the same locked
static `-O2` oracle. They passed cgo/static deep admission, bounded quiet-host
admission, and clean initial/final worktree checks.

The public `Parser.Parse` control reports:

| Fixture | Go median | static C median | Go / C | B/op | allocs/op | Go max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 28.006 ms | 5.447 ms | **5.141956x** | 152,098 | 13 | 68,428 KiB | 2,816 KiB |
| `rewrite.go` | 4.901 ms | 1.203 ms | **4.073988x** | 2,040 | 14 | 54,232 KiB | 2,304 KiB |
| `language.go` | 26.883 ms | 5.833 ms | **4.609063x** | 153,678 | 32 | 68,672 KiB | 2,816 KiB |
| `grammargen/lr.go` | 330.813 ms | 58.976 ms | **5.609304x** | 13,166,901 | 567.5 | 205,516 KiB | 9,216 KiB |

The production equal-fixture geomean is **4.824113x C**. Its fixed-suite sum
of medians is **5.466192x C**, and its worst fixture is `grammargen/lr.go` at
**5.609304x C**.

The separately build-tagged `AUTHENTICATED_CANDIDATE` reports:

| Fixture | Candidate median | static C median | Candidate / C | B/op | allocs/op | Candidate max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 19.488 ms | 5.554 ms | **3.508968x** | 231,222 | 9,032 | 49,672 KiB | 2,816 KiB |
| `rewrite.go` | 3.951 ms | 1.204 ms | **3.280552x** | 55,969 | 1,935 | 53,292 KiB | 2,304 KiB |
| `language.go` | 20.164 ms | 5.770 ms | **3.494748x** | 274,801 | 9,279.5 | 58,052 KiB | 2,816 KiB |
| `grammargen/lr.go` | 214.731 ms | 60.551 ms | **3.546268x** | 2,415,065 | 91,202.5 | 96,272 KiB | 9,216 KiB |

The candidate equal-fixture geomean is **3.456037x C**. Its fixed-suite sum
of medians is **3.534987x C**, and every fixture is below 3.55x C. All 40
timed candidate samples reported zero fallback, and each fixture retained one
repeat-identical compact work signature across its ten samples. Relative to
the preceding `15a04d7b` receipt, the geomean improves by **4.44%**, the suite
sum by **5.60%**, and the worst ratio by **5.98%**. Relative to the same-revision
production control, the candidate improves the geomean by **28.36%** and the
suite sum by **35.33%**.

Before banking construction-authenticated materialization, a balanced
two-order quiet-host A/B compared exact base `e24d56ee` with the final change.
The pooled fresh-full geomean improved by **3.44%**, every fixture improved by
2.36-4.50%, materialization-only improved by **13.20%**, warm total improved
by **3.90%**, and compact work plus fallback counts were unchanged. Both A/B
artifacts used only `gts_parsercorephase0`; an earlier mismatched-tag diagnostic
was rejected and is not part of this receipt.

Receipt identities:

- production manifest/report/archive SHA-256:
  `23cc2354b86b8fc31ad6f4fad98205cebc87a5bd8de34bcbd4ff1e111b2a0c3c`,
  `bf23ed1a4c87ed6bdd8828890d6a41cf8746646a115076b8920d166d46516d93`,
  `cdb0d89d5d42ed0240d263b1a6d60144caa66abff3d8db6e33164abed3a5d6c6`;
- candidate manifest/report/archive SHA-256:
  `02d0b998de5c8a4d202dc95f3a9a7c93afc5545d46df3fb297977ec3f1688800`,
  `fee39e64fb6efbf5fbc92f9f962f06f160873ff7c31678d4d0ef83db8ea41e58`,
  `9e94bb1c539bb6da2cf51b945ea77acc5f6ac961239cf4532e35ea97cad2bfe7`;
- locked static C artifact SHA-256:
  `dfbed45811491be8d81e32b293ed5577222445dae47b67d876cedae09679a871`.

The support boundary remains unchanged: the compact result authenticates the
four clean fresh-full Go fixtures and visible deep-tree structure, not public
`Parser.Parse`, recovery, incremental reuse, included ranges, parser-state
metadata, query/cursor behavior, or multiple grammars.

### Direct selected-store boundary receipt

The next diagnostic boundary removes public-node materialization from the
opt-in compact consumer without changing the public-node control. Acceptance
carries only the exact selected payload IDs. After a successful fresh
scheduler session, the direct route seals a pointer-light occurrence store,
walks it through `SelectedCursor`, and calls `SelectedStore.Release`; the
public route never constructs that store. The store is readable after
`Core.Reset`, retains no compact payload handle, and becomes unreadable after
release. Cancellation, occurrence count, retained bytes, unsupported policy,
precedence overflow, and root identity all fail closed.

The reviewed lifecycle creates unary policy metadata lazily, polls cancellation
before that width-squared construction, pools record and child backing as one
synchronized capped pair, preflights root-extra growth, rejects non-extra
sibling roots, and releases the store on every post-build validation failure.
The record deliberately omits parser-state and pre-goto-state metadata.

The exact reviewed revision `ba1ed1bf5e2e50e6125fbf1d66ea9fdbb8320bed`
first passed a balanced A-B-B-A boundary gate on 2026-07-18 on `ns1007492`,
CPU 2, with Go 1.22.2, `GOMAXPROCS=1`, 20 samples per lane and fixture, and
750 ms samples. The control is the same-revision public-node lifecycle; the
candidate includes selected-store sealing, complete cursor traversal, and
release.

| Fixture | Time delta | B/op delta | Retained selected bytes |
|---|---:|---:|---:|
| `rewrite.go` | **-12.68%** | **-55.45%** | 89,476 |
| `query_compile.go` | **-13.57%** | **-58.40%** | 453,236 |
| `language.go` | **-11.18%** | **-55.93%** | 430,416 |
| `grammargen/lr.go` | **-15.58%** | **-55.83%** | 4,384,556 |

The equal-fixture time geomean improves by **13.27%** and total B/op by
**56.42%**; allocation count improves by 0.47%. Both direct-store orders peak
below the public control (87,084/88,028 KiB versus 99,864/100,724 KiB). Every
timed sample reports zero fallback and the exact standing shifts, reductions,
pop requests, pop paths, pop payloads, link additions, leaf constructions, and
parent constructions for its fixture. Exact admissions additionally compare
all visible symbols, spans, fields, aliases/extras, terminal/external flags,
production IDs, checked precedence, selected-node counts, and deep-tree
digests against the public compact materializer. Parser-state and
pre-goto-state metadata are intentionally absent from the selected-store
record and are not part of this admission.

Board identities:

- combined benchstat SHA-256:
  `48f2c660f96a081db6ecd6b6c353de516fc80813bf82cff4d46fa8524ec0c355`;
- public samples SHA-256:
  `3ca87a44ca37cb5cc962d1edf238567d6ad3a7fa1340f203ec4b68c3e41d9477`;
- direct selected-store samples SHA-256:
  `f8bd8a724235e3053b523d07a7b36b365d6f1d9dc1295bb1216d9765ed1ca857`;
- test binary SHA-256:
  `ab042ed4030a608ed5f527cfef1941152f4ae36b8b0838bfadeb4e323b133d8e`.

The same clean revision then completed the strict locked-static-C publication
driver with cgo/static deep admission, quiet-host admission, five Go-C-C-Go
cycles, ten process-isolated samples per backend and fixture, exact work, and
zero fallback:

| Fixture | Selected store median | static C median | Selected / C | B/op | allocs/op | Selected max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `rewrite.go` | 3.140 ms | 1.208 ms | **2.599748x** | 24,824 | 1,909 | 39,888 KiB | 2,304 KiB |
| `query_compile.go` | 15.235 ms | 5.457 ms | **2.791974x** | 95,978 | 9,000 | 39,788 KiB | 2,816 KiB |
| `language.go` | 15.670 ms | 5.835 ms | **2.685641x** | 120,682 | 9,220 | 40,184 KiB | 2,816 KiB |
| `grammargen/lr.go` | 157.935 ms | 59.221 ms | **2.666881x** | 1,066,512.5 | 91,156.5 | 84,008 KiB | 9,216 KiB |

The selected-store equal-fixture geomean is **2.685181x C**. Its fixed-suite
sum is **2.676794x C** (191.980 ms selected store versus 71.720 ms static C),
and its worst fixture is `query_compile.go` at **2.791974x C**.

Publication identities:

- manifest SHA-256:
  `dcbe8586283f16e7da1122b3991277383f5ce0494e23bce653cf91757673042f`;
- report SHA-256:
  `c3b96d8713566833367f893eb598b4bd912407b55e31d6c23ba04418d523b78f`;
- complete receipt archive SHA-256:
  `9600063ebabd54e73a83d9b4af0fb3b2cb79d582a3803a61fa7f90f58d726c51`;
- locked static-C artifact SHA-256:
  `dfbed45811491be8d81e32b293ed5577222445dae47b67d876cedae09679a871`.

This is a diagnostic clean fresh-full Go consumer boundary. It is not a
public `Parser.Parse`, recovery, incremental, included-range, multi-grammar,
or public query API claim.

### Single-link compact reduction publication

A clean candidate publication was collected on 2026-07-18 from quiet host
`ns1007492`, pinned to CPU 3 with Go 1.22.2, at exact revision
`0062fe35880f18879a801cda58cbff249f9f8f32`. It uses the same locked static
`-O2` oracle as the preceding board and passed cgo/static deep admission,
bounded quiet-host admission, and clean initial/final worktree checks.

The build-tagged `AUTHENTICATED_CANDIDATE` reports:

| Fixture | Candidate median | static C median | Candidate / C | B/op | allocs/op | Candidate max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 19.001 ms | 5.426 ms | **3.501799x** | 231,221.5 | 9,032 | 52,896 KiB | 2,816 KiB |
| `rewrite.go` | 3.904 ms | 1.195 ms | **3.266439x** | 55,969 | 1,935 | 49,708 KiB | 2,304 KiB |
| `language.go` | 19.402 ms | 5.770 ms | **3.362799x** | 274,798 | 9,279 | 53,056 KiB | 2,816 KiB |
| `grammargen/lr.go` | 200.186 ms | 59.091 ms | **3.387751x** | 2,415,085 | 91,202.5 | 93,812 KiB | 9,216 KiB |

The equal-fixture geomean is **3.378660x C**. The fixed-suite sum of medians
is **3.392365x C**, and every fixture is below 3.51x C. All 40 timed candidate
samples reported zero fallback and retained repeat-identical compact work
signatures. Relative to the preceding `34c567bb` candidate receipt, the
geomean improves by **2.24%**, the suite sum by **4.03%**, and the worst ratio
by **1.25%**.

Before publication, a fixed quiet B/C/C/B comparison on the same host measured
the `query_compile` scheduler at 15.25 ms versus 14.95 ms (-1.97%) and warm
total at 18.89 ms versus 18.42 ms (-2.51%), with unchanged bytes and
allocations. A 10-second CPU profile reduced cumulative `popPaths` time from
6.79% to 3.72%; exact compact work, deep trees, and fallback counts were
unchanged.

Receipt identities:

- manifest/report/archive SHA-256:
  `59b887314c0b02847201dac3e541e6794abf113ae3e4394ca9594349239839b1`,
  `e45961d4dcb88b9e3904e62ed467b699df62b75dd34e2e7d911088fad0f2f5b3`,
  `c2ffa0c720911bb08b6015c05e53a2aaa4f06782c31af9140abd4338f26b8d01`;
- locked static C artifact SHA-256:
  `dfbed45811491be8d81e32b293ed5577222445dae47b67d876cedae09679a871`.

The support boundary remains unchanged: this is a diagnostic clean fresh-full
Go candidate, not a public `Parser.Parse`, recovery, incremental,
included-range, query/cursor, or multi-grammar claim.

### Fresh compact scheduler session publication

A clean candidate publication was collected on 2026-07-18 from quiet host
`ns1007492`, pinned to its least-busy admitted CPU with Go 1.22.2, at exact
revision `19a8f5265d158703ba1465687178a2adf527a372`. It uses the same locked
static `-O2` oracle and passed cgo/static deep admission before five Go-C-C-Go
cycles produced ten samples per backend and fixture.

The build-tagged `AUTHENTICATED_CANDIDATE` reports:

| Fixture | Candidate median | static C median | Candidate / C | B/op | allocs/op | Candidate max RSS | C max RSS |
|---|---:|---:|---:|---:|---:|---:|---:|
| `query_compile.go` | 17.573 ms | 5.516 ms | **3.185522x** | 230,909 | 9,030 | 46,948 KiB | 2,816 KiB |
| `rewrite.go` | 3.621 ms | 1.203 ms | **3.010150x** | 55,897 | 1,935 | 50,644 KiB | 2,304 KiB |
| `language.go` | 18.053 ms | 5.820 ms | **3.101863x** | 274,543 | 9,279 | 57,820 KiB | 2,816 KiB |
| `grammargen/lr.go` | 187.140 ms | 58.882 ms | **3.178231x** | 2,414,818 | 91,201.5 | 96,336 KiB | 9,216 KiB |

The equal-fixture geomean is **3.118130x C**, the fixed-suite sum of medians
is **3.169740x C**, and every fixture is below 3.19x C. Exact static-C
admission, compact work signatures, and zero fallback remained unchanged. This
publication does not claim a paired per-change speedup: its diagnostic control
used a different CPU, cycle count, and admission mode.

Receipt identities:

- manifest/report/archive SHA-256:
  `66242c222f60cafbbd4b1cf3b4deebd571e772f5c9f9981ef2aa17bbca404ed5`,
  `3eb7accb7f227d426f7ecb38b7480ebadda6561a264ee0e013f66833dbd29f4e`,
  `97c44e56600d60e6d8665c74e75f2da3dedf0347a009ed388634d79f16a938df`;
- locked static C artifact SHA-256:
  `dfbed45811491be8d81e32b293ed5577222445dae47b67d876cedae09679a871`.

The support boundary remains diagnostic and unchanged: clean fresh-full Go,
visible deep-tree structure, and exact work counts—not public `Parser.Parse`,
recovery, incremental reuse, included ranges, query/cursor behavior, or
multiple grammars.

### Diagnostic workload-regime receipt

Before the static publication artifact was built, a strict-admitted quiet run
used the exact locked Go grammar through the existing dynamic parity loader.
On `a5df0aa5`, ten 750 ms samples measured the synthetic at 1.0437x locked C
and the nearly size-matched `query_compile.go` at 2.8890x. The synthetic had one
stack and no forks or merges; the real file reached 12 stacks, 1,765 forks,
5,216 merges, and constructed 31,847 nodes for 7,524 selected. This proves the
workload-regime defect, not the final C ratio. Report SHA-256:
`c6de42e12724f72393162a0a50ecb8247f97312eaaff6cb5b093746b1206b4ab`.

### Authenticated work-count diagnostic harness

Wall time alone cannot distinguish excess GLR work from higher cost per event.
The diagnostic-only `gts-work-count/v2` contract therefore runs one fresh
public `query_compile.go` parse invocation through a tagged diagnostic Go test
binary and a fully static C binary built from the same locked runtime and
grammar as the publication oracle. The tagged parse is diagnostic, not a
production-path claim. It is not a timing lane, does not modify the production
static timing artifact, and emits no elapsed time. Every internal Go retry or
recovery reparse triggered by that single public invocation remains inside the
counter window.

Admission happens before instrumentation: a separate ordinary untagged Go
binary and the unmodified static C oracle must both reproduce the frozen
`gts-deep-tree-v1` digest. The tagged Go child independently authenticates the
fixture hash, grammar commit/blob, GLR workload regime, full clean span, and
deep tree before reporting counters. The C child must reproduce the same tree.
The complete unmodified-C cold build and admission runs in a dedicated,
wall-bounded process group, so repository acquisition, compiler/linker,
identity, `nm`, and `readelf` descendants are killed together on timeout.

The Go schema attributes counter deltas to every `parseInternal` attempt at
entry, after cap resolution, and during finalization. A logical retry rung is
reported separately from the resolved `retry_pass` mode. The aggregate must
equal the sum of attempt counters plus the explicitly reported outside-attempt
residual. On the frozen canonical fixture, `accept_actions=3` means three GLR
accept actions inside one `initial_full` attempt; it does not mean three parse
attempts. A content-addressed valid-Go retry witness pins the two-attempt
`initial_full` accepted-error to `initial_merge` accepted-clean sequence, and a
separate content-addressed straight-LR control pins the one-stack case.

`accept_actions` is a terminal-event counter, not a convergence counter. C can
accept once and then emit multiple packed roots during `pop_all`, while Go can
apply multiple accept actions inside one attempt. The historical v2 contract
therefore remains the shared Go/static-C counter vocabulary. The diagnostic
v3 supplement implements a Go-only convergence frontier; the static C child
does not fabricate equivalent events.

The v3 frontier retains at most the first 256 events plus the first rejection
for each closed-vocabulary reason. Events are keyed by attempt-local decision
IDs for a target/candidate pair, lookahead, and exact phase: reduce-window selection, post-reduce primary
and pending packing, boundary GSS/equivalence/cull, pending work, final path
expansion and selection, and terminal acceptance. Records contain values only,
never Go pointers. External-scanner comparability and checkpoint identity are
captured for the current token election, not inferred from either stack head.
Link and packed-path counts saturate independently from event truncation.
After chronological retention fills, scalar aggregates and bounded shapes
continue to update, while decision correlation, formatted detail, and head
snapshots are omitted. Packed final-path observation is bounded to the production
expansion limit plus one path (currently seven) and by a parse-global traversal
budget; an exhausted observation reports unknown snapshot evidence rather
than claiming that the production cap fired. This lane remains diagnostic and
untimed.

An authoritative `gts-work-count-receipt/v4` requires a clean Git checkout and
records `HEAD`, `HEAD^{tree}`, and `git_clean=true`. Both Go binaries compile
from one sealed content-addressed Git source snapshot. C compiles from private
snapshots of the runtime, grammar, patch, and driver. Ambient `GOT_*`,
`GOFLAGS`, `GOAMD64`, `GOEXPERIMENT`, and `GODEBUG` values are removed and the
chosen build/runtime environments are serialized. Build, patch, link, and
child process groups are wall-bounded. A stale receipt is removed before work
starts, and a successful receipt is published atomically in its destination
directory.
The ordinary untagged Go admission child remains on
`gts-work-count-go-child/v3`; the tagged diagnostic child is v4, while the
static-C child retains its historical schema.

Directly comparable counters are limited to applied shift/reduce/recover
actions, reduction pop requests and emitted paths/payloads, and the selected
tree census. Accept actions remain terminal evidence rather than packed-root
counts. The Go-only v3 frontier supplies the missing interpretation without
changing shared counter semantics. Table lookups, lexer calls, stack versions,
merges, graph links, and transient payload construction remain explicitly
labeled representation-specific proxies.

Run the focused Docker gate from a linked worktree by mounting its absolute
Git common directory and selecting the worktree-specific Git directory inside
the container:

```sh
git_common=$(git rev-parse --path-format=absolute --git-common-dir)
git_dir_name=$(basename "$(git rev-parse --path-format=absolute --git-dir)")
bash cgo_harness/docker/run_parity_in_docker.sh \
  --label work-count-query-compile --memory 8g --cpus 2 --timeout 12m \
  --mount "$git_common:/git-common:ro" -- \
  "export GIT_DIR=/git-common/worktrees/$git_dir_name GIT_WORK_TREE=/workspace; \
   cd /workspace/cgo_harness && env \
   GTS_WORK_COUNT_ORACLE=1 \
   GTS_WORK_COUNT_RECEIPT=/workspace/harness_out/work_count/query_compile.json \
   go test . -tags 'treesitter_c_parity treesitter_c_perfscan' \
   -run '^TestAuthenticatedWorkCountOracle$|^TestWorkCountSanitizedEnvDropsParserAndGoOverrides$|^TestWorkCountRunCapturedKillsDescendantProcessGroup$|^TestStaticCLanguageSymbol' \
   -count=1 -parallel 1 -timeout 12m -v"
```

The checkout must expose usable Git metadata inside the container. Dirty
source fails closed. `GTS_WORK_COUNT_ALLOW_DIRTY=1` exists only for focused
pre-commit harness tests; its receipt records `authoritative=false` and cannot
enter the evidence ledger.
The checked-in shared semantic contract remains
[`cgo_harness/work_count/contract_v2.json`](cgo_harness/work_count/contract_v2.json).
The independently validated Go-only frontier vocabulary is
[`cgo_harness/work_count/contract_v3.json`](cgo_harness/work_count/contract_v3.json).
`construction_surplus` means representation-specific leaf plus parent
constructions minus selected nodes; it must never be relabeled as a count of
discarded nodes.

### Four-fixture work-count board backends

The authenticated four-fixture board emits `gts-work-count-board/v3` and keeps
timing eligibility false. Production remains the default Go backend. Select the
build-tagged compact backend explicitly:

```sh
cd cgo_harness
GTS_WORK_COUNT_BOARD=1 \
GTS_WORK_COUNT_GO_BACKEND=parsercore_phase0 \
GTS_WORK_COUNT_BOARD_RECEIPT="$PWD/../harness_out/work_count_parsercore_board.json" \
go test -tags 'treesitter_c_parity treesitter_c_perfscan' . \
  -run '^TestAuthenticatedFourFixtureWorkCountBoard$' -count=1 -v
```

The compact backend builds a separate tagged artifact, authenticates the
ordinary untagged Go tree and locked static-C tree first, then runs the compact
child twice and requires repeat-identical counts. Directly comparable rows use
the shared `gts-work-count/v2` counter contract. Candidate-only scheduling
fields and mandatory events without paired semantic hooks are marked
incomparable or instrumentation-blocked; they are never filled with production
proxies or zeros. The checked-in board contract is
[`cgo_harness/work_count/board_contract_v1.json`](cgo_harness/work_count/board_contract_v1.json).

## Go-vs-C fleet scoreboard (full parse, real corpora)

Source of truth: [`cgo_harness/perf_scan/perf_ratio_budgets.json`](cgo_harness/perf_scan/perf_ratio_budgets.json)
and the wave-3 ledger. 204 of 206 languages carry ratcheted budgets
(D and F# are held out pending memory RCA; every exclusion is named in the
[known-gap ledger](cgo_harness/perf_scan/wave3_sweep_status.md)).

Distribution of observed full-parse ratios (Go time / C time, largest-file
basis, as of the 2026-07-11 ledger):

| Bucket | Languages |
|---|---|
| ≤ 1x (Go at or faster than C) | 10 |
| 1–2x | 64 |
| 2–3x | 29 |
| > 3x | 101 |

Median observed ratio: **~3x**. The long tail (up to ~650x on `uxntal`) is
dominated by small-file DSL grammars where C finishes in microseconds and
the ratio is mostly fixed per-parse overhead, plus a small set of named
GLR-ambiguity cliffs. Ratios are budgets with caveats, not endorsements:
`green_with_caveat` rows record exactly what was measured and what was not.

Named large-file witnesses (tracked, not hidden): JavaScript
`poppler.js` (3.4 MB — exact parity inside a hard 2 GiB container at
1,708,712 KiB max RSS; full parse 3.50x C), TypeScript
`webworker.generated.d.ts`, Groovy `pleac11_15.groovy`, and the
generated-table class (Go `opGen.go` / `rewriteAMD64.go` and in-repo
witnesses).

Fleet report reduction can preserve a terminal shard only when it carries the
closed status `no_static_c_oracle`, `no_corpus`, or `no_corpus_files` and no
measurement or oracle payload. These rows remain fatal closure findings and
are omitted from the combined oracle-language manifest. Certification still
rejects them; both modes reject missing, generic, contradictory, or mixed
identity evidence.

### Forest-routing screen and confirmation

The authenticated forest audit separates discovery from promotion. A
`forest-audit-result-v2` production shard is a directional screen: it can show
that the routed parser was faster on that run, but it cannot by itself promote
a language. Promotion requires fresh, isolated confirmation trials with the
same hashed host fingerprint, image, and resource configuration. Confirmation
uses `--cpus 1` and one numeric `--cpuset-cpus`; `--host-label` is an operator
label and is recorded separately from the automatically hashed host
fingerprint:

1. run one complete production-first trial (`pair-a`);
2. run one complete routed-first trial (`pair-b`);
3. pool the two orders only when both preserve exact production identity;
4. confirm only when the pooled routed speedup is at least 1.05x, each timed
   side lasts at least 750 ms, and order speedups stay within 10%;
5. escalate short, marginal, sign-changing, or order-sensitive results to an
   A-B-B-A sequence, increasing complete-corpus repetitions up to 20 for the
   duration floor.

The runner writes into an attempt directory, mounts the source tree read-only,
and publishes only after a successful container exit and a same-HEAD clean
worktree postcheck. Trial, run-config, and cohort receipts are immutable and
content-addressed. The reducer admits a cohort only through an explicitly
selected, content-addressed confirmation index; an unindexed or failed attempt
cannot complete a trial. `plan-confirmations` output is a planning artifact,
not evidence, and should live outside the results bundle (the reducer also
ignores its schema if it is copied there).

The reducer emits a win-only confirmation plan, keeps a screen win `incomplete`,
and records screening eligibility separately from promotion eligibility. Trees
are released after every repetition, including error, decline, nil-tree, and
divergence paths, and authoritative shards run exactly one language per
container. Every routed path must have accepted coverage in the locked C
shard. The locked C oracle remains the correctness authority: if forest and
routed results appear to correct a production-parser divergence, the reducer
records `oracle_correction_review_required` for direct three-way review instead
of auto-promoting or forcing a second four-runtime benchmark lane.

## Memory receipts (the v0.24 → v0.26.1 campaign)

On the exact Poppler witness under a hard 2 GiB container:

- Retained post-GC heap: 862,803,056 → **409,862,040 bytes (−52.5%)**
  after final-tree arena compaction (v0.26.1).
- Node header: **144 → 104 bytes** via arena-backed field-metadata
  sidecars (v0.26.0).
- Bounded raw-shape reclamation: **−192 MiB** retained (v0.26.0).

Each claim shipped with exact Go/C S-expression and deep parity on the same
witness; memory wins that break the selected tree do not merge.

## Methodology (why these numbers can be trusted)

1. **Correctness and performance are separate gates.** Before timing, every
   fixture must be clean and full-span and must preserve exact symbol, byte and
   point ranges, named/extra/missing flags, field ownership, and child order
   against the locked C oracle. See `cgo_harness/`.
2. **Ratchets, not snapshots.** Perf budgets only tighten
   (`cmd/benchgate`, `perf_scan` hard zero-cliff gate); parity exemptions
   only shrink (currently zero).
3. **Benchmark identity fails closed.** Fixture bytes, runtime commit, grammar
   commit, compiler, flags, and C artifact hash are recorded. A mismatch aborts
   admission instead of silently producing another ratio.
4. **Lifecycle and warm state are symmetric.** Each backend receives an
   untimed warm parse; the timed lane includes parse, first root validation,
   and tree release/close. Cold construction/loading is measured separately.
5. **Benchmark integrity is audited.** A 2026-07-11 audit found the then-
   canonical full-parse benchmark silently ran a no-tree diagnostic path;
   the affected headline numbers (1.54 ms / 7 allocs and successors) were
   withdrawn. A 2026-07-14 audit then found that the replacement workload never
   forked and that its C lane used a different grammar. Those claims are also
   withdrawn rather than patched around.
6. **Quiet-host discipline.** Publication runs use `GOMAXPROCS=1`, a pinned
   core, ten process-isolated samples per backend and fixture in five paired
   Go-C-C-Go cycles, at least 750 ms of internal timing, and Go `benchmem`.
   Contended-box measurements are smoke evidence only.

## Multi-workload tracking

```sh
go run ./cmd/benchmatrix --count 10   # bench_out/matrix.{json,md} + raw logs
```

The default matrix includes a bounded, warmed language-family full-parse
group reported in MB/s, plus the Go/editor lanes above.
