# JavaScript malformed quote: issue 1335

The recovery fix restores the complete fresh locked-C tree for
[#1335](https://github.com/odvcencio/gotreesitter/issues/1335), including the
`program` root, declarations, fields, anonymous tokens, error flags, byte spans,
and points. Inserting and removing the quote also matches fresh Go and fresh C
on both parser routes. This change does not graduate JavaScript.

## Reference and reproduction

The before revision is origin/main at
`2e93c61b4ed5ef32d82eb07561e709960e2b478f`.
The engine fix is `a3521c6cf8c31181b307b4829c90e4e28a9ec203`.
No competing open PR for 1335 was present when this work began or when checked
again before publishing the branch.

The locked reference uses C runtime 0.25.1 at
`f5afe475deb7c0bae6407fb776c76824f717bb61`, JavaScript grammar at
`58404d8cf191d69f2674a8fd507bd5776f46cb11`, and go-tree-sitter v0.25.0 at
`adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3`. The native memory probe uses the
same locked runtime and grammar, compiled with `-O2`.

The reduced witness is 12 bytes, including its final newline:

```javascript
var "x = a;
```

The original witness contains 40 repetitions of
`function fn%d(a, b) {\n\tvar x%d = a + b;\n\treturn x%d;\n}\n\n`, with a quote
inserted before `x0`. It is 2,211 bytes. Repeating the same construction with
16,000 functions produces the 1,006,671-byte safety case. The test helper
`issue1335Source` reconstructs both inputs.

| Input | Before root | After and C root | Fresh C deep-tree SHA-256 |
| --- | --- | --- | --- |
| 12 bytes | ERROR | program | `b41240f9a8b1fc46e76c20afc94a42327c1a89529c4bf610dd0446129da785f9` |
| 2,211 bytes | ERROR | program | `f5b06003a7595bde54d6949e26a8c45ef7908cff1ab318e16edd676db959abe0` |
| 1,006,671 bytes | ERROR | program | `ca40046681be97aba71e26f5bbd53eff1eaa83dc742026db71d51f9bcc4fb244` |

On current main, the original witness's Go digest is
`89c4fa267e9c3b283adcc21c52e14f4e69a517cae9728f19c383b30ae4ea64c0`.
The older Go digest in the issue describes an earlier revision.

## Cause and fix

Legacy recovery unwound the declaration at the unexpected quote and replayed
it in a top-level string context. The permissive lexer then skipped a newline
that the string context could not lex and consumed later declarations as string
fragments. C kept the declaration context and skipped the quote locally.

The new policy in `internal/recover` selects a local skip only when grammar
actions prove that the next token resumes the current production and a bounded
raw lexical probe proves that the alternate context is unterminated. The
adapters preserve and restore lexer and scanner state and record lexical read
frontiers. A clean production bounded by matching anonymous terminals can
complete before legacy recovery wraps its unreduced children. This preserves
the separate closed-quote behavior as well. The engine adds no language-name
comparison and changes no admission, memory-budget, or gate threshold.

Three broader candidates were rejected:

- Enabling C cost competition returned `program` but the original witness's
  tree digest was
  `1b5719eec43c9d3c3da368b324f39c92259a544df8dec7d8ec12a12299bd14f6`,
  rather than the locked C digest `f5b060...`.
- An unconditional local skip fixed the open quote but produced one declaration
  for `var "x" = a;\n`, where C has two root children.
- Completing ordinary productions changed Godot's first-edit allocation count
  from 98 to 99 and changed its tree digest. Restricting completion to matching
  anonymous terminal endpoints restored 98 nodes and the unchanged digest
  `4dad58de3c6e60724396cb171416d181f16629834615a12e3ce904c499f4285d`.

## Counters and memory

The standard counter gate passed all 412 route rows for 206 languages. Every
collected row equals the before revision's row, including work and reuse.
JavaScript's committed real sample first edit remains at 168 tokens, 685 new
nodes, two live versions, 5,952 ppm multi-version token share, 937 reused bytes,
and 213 block splices. Existing pins were not changed. Both collected ledgers
have SHA-256 `488e04891145d103e38d1df2355d86f1258a32588e312a473ea3ab5a666941b2`.

The malformed witness now constructs the complete tree, so its own work grows:

| Input | Metric | Before | After |
| --- | --- | ---: | ---: |
| 2,211 bytes | Tokens | 170 | 842 |
| 2,211 bytes | New nodes | 333 | 1,842 |
| 2,211 bytes | Arena bytes | 7,536,544 | 7,657,504 |
| 2,211 bytes | Scratch bytes | 5,160,640 | 2,148,928 |
| 1,006,671 bytes | Tokens | 64,010 | 336,002 |
| 1,006,671 bytes | New nodes | 128,013 | 736,002 |
| 1,006,671 bytes | Arena bytes | 76,283,920 | 118,699,248 |
| 1,006,671 bytes | Scratch bytes | 7,032,832 | 2,772,992 |

Both inputs retain one stack, an accepted stop, full root coverage, and
`HasError=true`. The after trees equal C; the smaller before counters describe
incorrect trees.

Standalone `/usr/bin/time -v` processes each warmed once, then parsed and
released three fresh trees. At 1,006,671 bytes, peak RSS is 172,904 KiB before,
346,440 KiB after, and 44,640 KiB for native locked C: **3.873x C before and
7.761x C after**. After RSS is 352.4 bytes per source byte, below the 400-byte
gate. Scratch allocation falls 60.6%. No crash, OOM, truncation, or budget
override occurred. The small process's RSS is dominated by startup and is not
used as a memory ratchet.

## Timing

All comparisons use `scripts/run_randomized_benchmarks.sh`, seeds 1 through
20, `GOMAXPROCS=1`, one process per seed, `-count=1`, `750ms`, and `-benchmem`.
Baseline and candidate process order alternates by seed. Correctness completed
before timing started. The containers use Go 1.25.14 on linux/amd64.

The standard Go controls have these 20-sample medians:

| Benchmark | Before ns/op | After ns/op | Before/after B/op | Before/after allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Full DFA | 15,880,048.5 | 16,410,480 | 1,301 / 1,307 | 8 / 8 |
| Single-byte edit DFA | 322,422 | 321,296.5 | 391 / 391 | 5 / 5 |
| No-edit DFA | 15.185 | 15.015 | 0 / 0 | 0 / 0 |

`benchstat` finds no significant time change (full `p=0.142`, edit `p=0.758`,
no-edit `p=0.815`, all `n=20`). The nominal full-parse time rises 3.34%; the
three-case time geomean rises 0.61%. Full-parse B/op rises 0.46% (`p=0.036`).
This benchmark does not warm before its timed loop. Its adaptive median
iteration count falls from 57 to 52, amortizing the same initial allocation
over fewer parses: fitting `B/op = steady + startup/b.N` gives steady costs of
1,238.59 and 1,238.55 bytes, startup costs of approximately 3,560 bytes, and
residuals below 1.15 B/op. This is an inference from the samples and benchmark
setup. Allocation counts and deterministic work remain unchanged.

The malformed JavaScript timing uses Go-C-C-Go slots for both sizes. The
binding measurement is retained as supplemental evidence. Publication ratios
use the existing fully static, `-O2 -DNDEBUG` locked C oracle's parse-only
timer, excluding process startup, source setup, parser setup, and tree deletion.
Go times include `Parse` and `Release`, so the publication ratio also includes
Go's release cost. Only native C-slot ns/op uses the C timer. Other C-slot
metrics, including throughput, B/op, and allocs/op, describe the Go sidecar
transport; native C heap use is measured by standalone RSS.

The native results aggregate the two Go slots and two C slots across all 20
seeds (40 samples per parser). Each seed's ratio is
`sqrt(Go_first * Go_second / (C_first * C_second))`; the table reports the
median of those 20 paired ratios.

| Input | Go time before / after | C time before / after | Go/C before / after | Go B/op before / after | Go allocs/op before / after |
| --- | ---: | ---: | ---: | ---: | ---: |
| 2,211 bytes | 0.691 / 2.235 ms | 0.678 / 0.683 ms | 1.028x / 3.270x | 53,931 / 1,736 | 83 / 18 |
| 1,006,671 bytes | 261.5 / 1,036.4 ms | 316.0 / 320.8 ms | 0.824x / 3.229x | 87,352,000 / 95,115,960 | 1,181 / 38 |

C time is stable. Go time rises approximately 3.2x on the small input and 4.0x
on the large input because it now parses the complete C-matching tree. These
before timings describe incorrect output. Small-input allocation bytes fall
96.78% and allocation counts fall 78.31%; large-input allocation bytes rise
8.89% while allocation counts fall 96.78%. The maximum per-seed after Go/C
ratio is 3.895x, below the 10x hard gate. This is a correctness trade; the
standard ledger has no increases, and no existing performance pin is reset.

The supplemental binding ratios are 0.430x to 1.465x on the small input and
0.401x to 1.595x on the large input. They include binding input callbacks and
are not the publication ratios.

[Raw samples, benchstat output, and verification data](receipts/issue-1335/)
are checked in. The benchmark container's Git headers report `unavailable`
because it did not mount the common Git directory. The engine commits were
verified on the host, and the same benchmark helpers were copied into the
baseline checkout. Their SHA-256 values are in `verification.json`.
The standalone C artifact has SHA-256
`3003c712c663563426cd3894ce062741a9e6849a7fe237dd51f10bce917ee0b9`.

## Correctness and limits

- The new cgo regression passed 26 complete fresh-C comparisons: nine fresh
  cases on both routes plus quote insertion and repair for the reduced and
  original witnesses on both routes. Cases include single quotes, `let`,
  `const`, CRLF, closed quotes, and the large input.
- The root regression checks fresh-C digests, incremental-equals-fresh Go,
  root coverage, error flags, quote repair, and zero allocations across 100
  unchanged-input reparses on both routes.
- Standard JavaScript fresh, incremental, no-error, and highlight parity
  passed. The focused JavaScript sibling-selection, class-body, closing-paren,
  block, comment, and compact-certification cases passed.
- The authenticated corpus receipt matched all four clean files before and
  after. All 72 edit-session invariant steps passed, including coverage,
  error-root flags, D8, and zero unchanged-input allocations. All 72 edited
  Go tree digests are unchanged from main. The same 72 C parity mismatches
  remain on both revisions; this is not a general JavaScript recovery fix.
- A one-language-per-process, 206-language diagnostic invariant sweep yielded
  193 passes, six existing mismatches, and seven existing 60-second timeouts.
  Every failure or timeout was reproduced with identical output on main.
  Mismatches: awk, djot, html, javascript, meson, twig. Timeouts: elsa, fsharp,
  godot_resource, haskell, kotlin, nickel, powershell. This sweep preceded the
  final terminal-only completion guard; the final standard ledger was rerun.
- JavaScript W5 passed the 20 KiB and 137 KiB cases. The nine optional 1 MiB
  cells hit the same existing work ceiling before and after; for example,
  insertion at the start consumed 287,653 tokens against a 262,400 limit.
- The JavaScript repetition-fold control fails identically on main because
  it does not exercise the global C repetition-skip fold. It was not disabled.
- After fetching the pinned JavaScript grammar, the generator corpus-snippet
  suite passed 16 of 18 cases. Its two JSX failures (`jsx_corpus_block_exact`
  and `jsx_self_closing_multiple_attributes`) reproduce on main with identical
  generated and reference trees.

The final Docker root `GOWORK=off go test ./... -count=1 -timeout=60m`
completed: 41 packages passed; only `grammargen` failed, in four existing test
groups. Each failure was reproduced individually with identical details on
main: Swift has 306 rules against an expectation of 299; the alias inventory
has ten uncovered languages; and the same 41 CommonMark cases plus the fenced
code-content test disagree with the reference Markdown tree. None of these
expectations were changed. The root parser package passed in 446.904 seconds.
Optional suites that require additional imported grammars or the separate
`corpus_real` store retained their normal self-skips; the affected JavaScript
generator suite was seeded and run separately as described above.

The corpus lock was fetched through `GTS_CORPUS_LOCK_URL` and verified as
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
It remains outside the repository. All heavy runs used the
`gotreesitter/cgo-harness:go1.25-local` image; two persistent containers were
created and reused. Correctness and timing were run separately.

## Reproduce

Run the following inside that Docker image with the source tree, Go caches,
locked C cache, and authenticated corpora mounted. Use one language at a time.

```sh
export GOWORK=off GOMAXPROCS=1 GOFLAGS=-p=1
go test ./... -count=1 -timeout=60m
go run ./cmd/perfcounterledger
go test . -run '^TestJavaScriptMalformedQuotePreservesDeclaration$' -count=1
cd cgo_harness
go test -tags 'treesitter_c_parity gts_parsercorephase0' . \
  -run '^TestJavaScriptMalformedQuote.*LockedC$' -count=1
go test -tags 'treesitter_c_parity gts_parsercorephase0' . \
  -run '^TestParity(FreshParse|IncrementalParse|HasNoErrors|Highlight)$/^javascript$' -count=1
```

For before/after timing, check out the before revision separately and copy the
new benchmark test helpers there. Run `scripts/run_randomized_benchmarks.sh`
with `--baseline-root`, `--baseline-output`, `--output`, `--runs 20`, and
`--benchtime 750ms`. The wrapper supplies Go `-count=1` and `-benchmem`.
Select the standard Go trio in the root module and
`^BenchmarkJavaScriptMalformedQuote(Large)?LockedC$` in `cgo_harness`, with
`--tags 'treesitter_c_parity gts_parsercorephase0'`. Compare the raw samples with
`benchstat`. Each JavaScript input measures warm Go-C-C-Go fresh parses; each
tree is released within its iteration.

For the standalone C comparison, select
`^BenchmarkJavaScriptMalformedQuoteNative(Large)?LockedC$` with
`--tags 'treesitter_c_parity treesitter_c_perfscan gts_parsercorephase0'`.
Set `GTS_C_ORACLE_CACHE` to an external cache directory. The existing builder
fetches, verifies, and compiles the pinned runtime and grammar automatically.

To reproduce the JavaScript corpus invariant receipt, keep the lock outside
the repository and run from `cgo_harness`:

```sh
go run -tags 'treesitter_c_parity gts_parsercorephase0' ./cmd/gts_grammar_receipt \
  -grammar javascript -repo-root .. -out "$RECEIPT_OUTPUT" \
  -corpus-root "$CORPUS_ROOT" -corpus-lock "$CORPUS_LOCK" \
  -corpus-lock-sha256 41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea \
  -gotreesitter-commit "$ENGINE_COMMIT"
```
