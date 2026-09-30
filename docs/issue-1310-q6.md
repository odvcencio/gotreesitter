# Q6 early stops: partial receipt

Refs [#1310](https://github.com/odvcencio/gotreesitter/issues/1310).
The Go false error is fixed, and the merged Python stack-cap fix is verified.
Q6 remains open: C++, HTML, and SQL still differ from fresh locked C; the Go
legacy route also remains above 10× C on the original witness.

Baseline engine: `8a78a7673e502b9d3eb1ae7b7f99328b951b3976` (origin/main when work began).
Initial fix: `80ddbba240847bfafd257b1b811026a3bdf6be63`.
The skip-token representation is `c5b3cf4fcc0060eb2cdecb061dd0c69cfd68b281`;
final bounds validation is `011eac73c1ba609ba57061062f766b79096733cc`.
Measurement tools: `68e97c29e05ff6aee506fdd18294530e9511739e` and
`53eec596d243edf94c3f07ac7fd80149658e3408`.
C reference: tree-sitter 0.25.1, commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`; grammars use `languages.lock`.
The oracle upgrade is separate work. All oracle comparisons here use fresh C
with no old tree. Incremental checks compare against fresh Go (D8).

## Go mechanism and regression

This 38-byte witness reproduces the false error on the baseline:

```go
package p
func f(){switch x{case
Y:}}
```

The shared lexer emits an external automatic semicolon for a competing
composite-literal interpretation. The expression-switch version needs the same
newline as internal-lexer padding. Previously that version had no action for
this shared token and was paused or discarded.

The fix keeps that version alive only after proving that its DFA skips the
whole external token, the scanner is stateless, the span is whitespace with a
newline, its state has no external-token action, and a reachable internal
continuation has an action with a whitespace-only gap. Horizontal whitespace
is excluded because it can represent an external concatenation token. The
proof lives in `internal/lexpadding`; there are no new language comparisons in
engine code. The stack's parse state and byte offset remain unchanged while
it waits for the next shared token.

The LF, CRLF, and multiple-expression witnesses pass on both routes. The
original 75,688-byte Go 1.25.1 `go/parser/parser.go` now matches fresh C,
including every canonical tree field, with no error. Its canonical digest is
`9ed06ed630d8e18a6f191271d9da9339765ad1bd8ca15df2829d9b58cf3cd0f1`.
The baseline legacy digest was
`d68083e7d0b99b493f4c78faceb11c191a5edf6bdba34a379df99eb67e10816e`.

Both routes passed the full original file's 72-step edit session. Each step
matched an independent fresh Go parse, each ERROR root carried HasError, and
any incomplete root needed an explanatory stop. Reparses without edits
allocated zero times. This run took 464.36 seconds and peaked at 2,613,728 KiB
RSS; it is correctness evidence, not timing comparison evidence.

## Deterministic counters

These are the legacy winning attempt's runtime counters. Whole-operation
allocation measurements include all retries. A low token count on SQL or C++
therefore does not establish correctness.

| Witness | Bytes | Tokens before → after | Nodes before → after | Peak stacks before → after | Result |
| --- | ---: | ---: | ---: | ---: | --- |
| Go, shrunk switch | 38 | 19 → 19 | 70 → 32 | 2 → 2 | Clean, exact C |
| Go, original parser | 75,688 | 17,462 → 17,462 | 399,206 → 408,090 | 72 → 72 | False error removed, exact C |
| C++, GCC 13 vector header | 70,530 | 318 → 318 | 774 → 774 | 7 → 7 | Stops at byte 4,532 |
| HTML, original memory model | 27,268 | 3,107 → 3,107 | 4,958 → 4,958 | 1 → 1 | Accepts full span; wrong tree |
| SQL, information schema | 128,179 | 715 → 715 | 1,933 → 1,933 | 9 → 9 | Accepts full span; wrong tree |
| Python, issue #454 generator | 140,363 | 51,717 → 51,717 | 371,146 → 371,146 | 8 → 8 | Clean, exact C |

The original Go winning attempt allocates 8,884 more nodes (+2.2254%) while
producing the exact C tree. The correctness change preserves a valid branch
that was previously lost. The compact full parse remains at 38,538 work nodes
and 17,461 tokens, with the same exact C digest.

The diagnostic whole-operation counters include every retry. They confirm
that the higher winning-attempt node count does not represent more total
work. All nonzero aggregate work fields fall; zero fields stay zero.
The tagged observer preserves the untagged tree digest on both revisions.
See [the complete attempt receipt](benchmarks/q6/whole-work-receipt.json).
The baseline tagged test deliberately fails on its original false error; its
recorded digest matches the untagged baseline. The fixed tagged test passes.

| Whole operation | Before | After | Change |
| --- | ---: | ---: | ---: |
| Parse attempts | 6 | 2 | −4 |
| Reductions / pop requests | 823,889 | 279,471 | −66.08% |
| Lexer front-door calls (proxy) | 104,766 | 34,922 | −66.67% |
| Leaf constructions (proxy) | 749,522 | 253,839 | −66.13% |
| Parent constructions (proxy) | 449,391 | 151,809 | −66.22% |
| Table lookups (proxy) | 4,118,329 | 1,352,335 | −67.16% |
| Merge attempts (proxy) | 3,668,394 | 1,216,918 | −66.83% |

The deterministic ledger passed before and after. Its collected JSON digest
is `cc32935db65906abdae75112c6df065a7607a25a3606f21de368497ad69104ce`
on both revisions. All 412 collected rows
across 206 languages are identical, including work and reuse. No existing
ledger, tree digest, census, expectation, threshold, or allowlist was changed.

The proof helper also rejects out-of-source unsigned offsets on 32-bit Go.
An end offset of `^uint32(0)` previously wrapped during conversion to int and
panicked while slicing a two-byte input. Comparing unsigned bounds before
slicing fixes it. The regression passes on both amd64 and 386; the change
has identical span-validation semantics on amd64.

## Python scope

[PR #1290](https://github.com/odvcencio/gotreesitter/pull/1290) is already merged.
Its legacy work reduction was 623,912 → 371,146 nodes. The exact inline
[issue #454](https://github.com/odvcencio/gotreesitter/issues/454) generator
produces 1,847 functions and 140,363 bytes, source digest
`3bbfe083cbe4b958ec90ddb88a91c44fbe35a22e9c33df374e533830ec23bd8b`.
It uses a distinct `xN` in each function. The general issue454bench generator
uses `x0` and is not the same fixture.

The current baseline and fixed engine both match fresh C's 59,105 visible
nodes and digest
`7f153bd61afacc587d071c9a759485fa3816b8dd003eb7907eeeee8d26df9db5`.
Legacy work is 371,146 nodes; compact work is 123,749 nodes. The default stack
configuration is used; the receipt clears `GOT_GLR_MAX_STACKS`.
The existing list-splat and Python scheduler oracle regressions also passed.
No additional Python engine change is needed for the stack-cap witness.

## Timing and memory

The standard Go controls are:

| Benchmark | Before | After | Change | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| Full DFA | 8.775 ms | 8.762 ms | No significant change (p=0.341) | 1,277 → 1,276.5 | 8 → 8 |
| One-byte edit DFA | 185.5 µs | 185.9 µs | +0.25% (p=0.001) | 388 → 388 | 5 → 5 |
| No-edit DFA | 9.512 ns | 8.005 ns | −15.84% (p<0.001) | 0 → 0 | 0 → 0 |

The 0.25% edit increase is a directional regression. It stays below the 10%
ratchet; the original-file correctness and total-work improvements are the
tradeoff. The earlier 7.61% edit increase was rejected and reduced by restoring
the three-result re-lex return.

| Witness / route | Go ms before → after | C ms before → after | Go/C time before → after | Go B/op before → after | Go allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go parser, legacy | 1,287.582 → 384.749 | 26.002 → 25.837 | 49.535× → 14.888× | 166,820,984.0 → 45,465,087.0 | 17,954.00 → 3,774.50 |
| Go parser, compact | 72.347 → 72.431 | 25.986 → 25.840 | 2.781× → 2.803× | 1,338,786.0 → 1,338,786.0 | 452.75 → 452.75 |
| HTML memory model, legacy | 18.851 → 18.741 | 2.876 → 2.861 | 6.550× → 6.549× | 641,418.5 → 641,418.8 | 5,616.00 → 5,616.00 |
| HTML memory model, compact | 25.890 → 25.724 | 2.878 → 2.863 | 8.915× → 8.971× | 5,285,265.2 → 5,285,374.5 | 9,358.00 → 9,358.00 |
| Python #454, legacy | 289.783 → 290.359 | 87.712 → 85.771 | 3.323× → 3.391× | 8,408,418.0 → 8,408,418.0 | 20.00 → 20.00 |
| Python #454, compact | 229.077 → 229.525 | 87.073 → 85.774 | 2.634× → 2.670× | 27,067,577.0 → 27,067,580.0 | 105,370.25 → 105,370.25 |

The compact Go first sample increases 0.51% (p=0.004); its second sample
has no significant change. The combined per-seed Go time increases 0.12%.
That small directional increase and the root edit increase are recorded
rather than hidden by the large legacy improvement. The witness medians and
all named sample statistics are in the JSON and benchstat receipts. Python
Go times have no significant change, but its median Go/C ratios increase
3.323× → 3.391× (legacy) and 2.634× → 2.670× (compact), mainly because the
matched C samples become faster. These directional ratio increases remain
below the ratchet.


Use the raw benchmark receipts in [benchmarks/q6](benchmarks/q6) for individual samples.
The protocol uses 20 seeds (1–20), one process per seed, GOMAXPROCS=1,
count=1, benchtime=750ms, and benchmem. Baseline and candidate process order
alternates. The Q6 benchmark shuffles the legacy/compact cycle order and
preserves Go-C-C-Go within each cycle. The baseline and candidate use identical
measurement sources; [their SHA-256 manifest](benchmarks/q6/harness-source-hashes.json)
records all six overlaid harness files. Table entries are medians of per-seed
sample means; Go/C is the median per-seed ratio. Benchstat retains each named
sample separately.

B/op and allocs/op report Go allocations, including Go wrappers in the C
samples. They exclude native C allocations. The Go/C memory comparison uses
peak RSS from separate processes, each parsing one witness ten times and
releasing every tree. RSS includes runtime and grammar loading, so it is a
whole-process measure. Native C allocated-byte and allocation-count ratios
were not measured.

| Witness / route | Go peak KiB before → after | C peak KiB before → after | Go/C RSS before → after |
| --- | ---: | ---: | ---: |
| Go parser, legacy | 398,848 → 352,656 | 18,208 → 18,192 | 21.905× → 19.385× |
| Go parser, compact | 45,548 → 45,300 | 18,208 → 18,192 | 2.502× → 2.490× |
| HTML memory model, legacy | 27,404 → 27,740 | 15,072 → 14,816 | 1.818× → 1.872× |
| HTML memory model, compact | 42,076 → 42,296 | 15,072 → 14,816 | 2.792× → 2.855× |
| Python #454, legacy | 191,680 → 191,448 | 25,104 → 25,064 | 7.635× → 7.638× |
| Python #454, compact | 109,524 → 109,420 | 25,104 → 25,064 | 4.363× → 4.366× |

A final EOF contract check confirms that stack re-lex rejects EOF before
probing, leaving the lookahead, parse state, and node count unchanged for
existing recovery. A proposed additional caller span guard was unnecessary
and was removed. The engine therefore remains exactly the measured
`011eac73` revision; only this additional contract test was added afterward.

RSS is directional evidence from three processes: HTML legacy increases
27,404 → 27,740 KiB (+1.23%), and compact 42,076 → 42,296 KiB (+0.52%).
Go legacy falls 398,848 → 352,656 KiB (−11.58%). All samples and ratios are
published; no run hit the 8 GiB container limit.

The Docker image was `gotreesitter/cgo-harness:go1.25-local`, with Go 1.25.14,
Linux amd64, and an Intel Xeon Platinum 8481C. Two containers were created and
reused; each had an 8 GiB memory limit. Correctness finished before timing.

## Gates and limits

[Per-language gate results](benchmarks/q6/gate-results.csv) and
[cliff frontier counters](benchmarks/q6/cliff-frontiers.json) preserve the scope
and unresolved baseline failures.

- Focused lexer-read-span, padding-proof, switch-case, incremental, and
  zero-allocation regression tests passed.
- Original Go fresh-C canonical parity and both full-file edit sessions passed.
- Existing fresh/incremental C parity passed before and after for all 41
  stateless-scanner grammars, with exhaustive mode and known skips disabled.
  Each grammar ran in its own process with an anchored subtest filter.
- R4 invariant sessions passed for 38 affected grammars. AWK failed at step 1
  and JavaScript at step 2 with the same digests and counters on origin/main.
  PowerShell timed out after five minutes in Tree.Edit on both revisions.
  These failures were not suppressed or repinned.
- The root Docker command `GOWORK=off go test ./... -parallel=1 -count=1
  -timeout=45m` completed in 30:48.32, peaking at 4,110,780 KiB RSS. 
  This sweep ran the frozen c5b3cf4f engine. The subsequent bounds-only
  patch has identical amd64 semantics and passed focused amd64/386 checks,
  the counter gate, and original Go C parity. All packages
  passed except grammargen. Its four failures reproduce on the baseline:
  `TestImportedKotlinSwiftGrammarConstructors` (306 Swift rules, expected 299),
  `TestMarkdownGrammarCommonMarkParity`, `TestMarkdownFencedCodeBlockContentParity`,
  and `TestShippedBlobsNonTerminalAliasMapInventory` (10 uncovered grammars).
- The Go cliff gate fails before and after: legacy peaks at 72 versions
  versus C's 8 (ceiling 18); multi-stack share is 96.31% → 98.36% versus
  C's 27.53%. This increase keeps the previously lost valid branch alive.
  Compact stays at 5 headers and 27.72% multi-header share. Elixir passes
  unchanged at 2 versions and 10.53% share versus C's 2 and 5.26%; compact
  declines that witness. Only frontier counters from the cliff report are
  used here; its one-seed diagnostic timings are discarded.
- No race sweep or graduation census was run. These receipts do not graduate
  a grammar, change the default engine, or satisfy the full D13 size matrix.

Affected grammars: awk, comment, css, cue, d, dhall, dtd, editorconfig, elixir,
erlang, fennel, fish, foam, gleam, gn, go, godot_resource, janet, javascript,
julia, kconfig, kdl, less, liquid, move, nix, odin, pkl, powershell, racket,
ron, scss, squirrel, tablegen, tcl, toml, tsx, typescript, uxntal, wgsl, yuck.

## Rejected candidates and remaining recovery work

[Baseline recovery witnesses](benchmarks/q6/baseline-recovery-witnesses.json)
record the canonical C and Go digests, source spans, and stops for C++, HTML,
and SQL. [Final engine diagnostics](benchmarks/q6/final-recovery-witnesses.json)
reproduce all three root differences with the same work counters. These are
diagnostic receipts, not replacement expectations.

The initial four-result re-lex return passed correctness, but the first
20-seed root control showed full parsing +0.85%, edits +7.61%, and no-edit
reparses −11.29%, with unchanged allocations. The final code keeps the
original three-result return and uses the lexer's existing positive-width
symbol-zero skip token. The common no-match path stays unchanged. Final
matched-harness measurements are reported above. Preliminary C-harness
timings had different Go regression files in the baseline; they are not used
as final comparison evidence.

The existing `GOT_FAITHFUL_CONDENSE=1` experiment now gives exact fresh-C Go
parity after this fix, at 47,458 nodes and 6 stacks. Before the fix it still
reported a false error at 48,142 nodes and 8 stacks. This is a counter-only
follow-on finding for Q0, not a default policy change or timing claim. D8 and
performance under that experimental policy were not gated here.

An earlier padding candidate also admitted horizontal whitespace. AWK's first
ledger edit went from 114 to 128 tokens, 407 to 460 nodes, 314 to 279 reused
bytes, and 85 to 72 splices. Requiring a newline removed that regression;
all ledger rows then matched the baseline.

A C++ resync candidate re-lexed from a dropped non-padding gap after popping a
recovery state. Coverage increased from 4,532 to 70,308 of 70,530 bytes,
tokens from 318 to 6,534, and nodes from 774 to 18,607. It still stopped before
EOF and did not match C. Worse, `(n))#i` became an empty, error-free
translation_unit where C has an ERROR child. The candidate was rejected.
The current engine still loses the vector header at a preprocessor directive:
recovery pops the parse state but retains lookahead scanned in the old mode.
Re-lexing alone does not preserve the required error payload or tree shape.

Enabling the existing C recovery port for HTML accepted the full input and
produced a document, but gave 137 root children versus C's 143, at 4,970 work
nodes versus the default's 4,958. It still absorbs the final malformed pre
block and following siblings into one ERROR. A literal `<` in the code sample
needs recovery that preserves the surrounding element and later siblings.
The override was rejected as a fix.

SQL's shared lex election picks a hidden quoted-identifier token
`[5884..6421]` over the correct semicolon `[5884..5885]`. The losing version
cannot accept a shorter token under the current shared frontier. A narrow
candidate preferring visible tokens over hidden EOF tails exactly fixed this
75-byte witness:

```sql
CREATE DOMAIN a AS character varying COLLATE "C";
CREATE DOMAIN b AS name;
```

It did not change the original file. Extending that preference to hidden
non-EOF tails increased the original to 778 tokens and 2,143 nodes but still
did not match C. Neither heuristic was shipped. SQL needs a safe way to retain
versions with different lexed token spans, followed by exact recovery and root
selection. Complete source spans alone do not prove that the file was parsed.

## Provenance and reproduction

The Go and HTML witnesses are the authenticated fixtures in
`internal/benchfixtures/testdata/cliffs`; their source digests are
`ad0a0a8fce883ab86ec3d2bf1d8d8dcc1072534bff46ab26c0796795f7b3087a` and
`561b50bca58e07455e993d68cf49281ed80df2150d09a38c77c906eabee36b7a`.
The original C++ header revision was not specified. The installed GCC 13
header reproduces the exact token-318 failure and has digest
`c4d4d95e5f60ea1b349e1a7568edef381621c38b430826e2d669ffeb2a2eb1b7`.
The SQL corpus checkout matches the verified external lock at PostgreSQL
`89eafad297a9b01ad77cfc1ab93a433e0af894b0`; information_schema.sql has digest
`cd7c97c2e15344d37de86161ea643d139a35d023eafe9b2e6e9ced19a507ce7d`.
The external corpus lock was authenticated and was not added to the repository.

Run heavy checks inside the specified Docker image, with GOWORK=off and the
locked C reference cache mounted. Use one grammar per process. From the root:

```sh
go run ./cmd/perfcounterledger
GOTREESITTER_V1_INVARIANT_LANGUAGE=go go test . \
  -run '^TestV1InvariantGateR4EditSession$' -count=1
bash scripts/run_randomized_benchmarks.sh \
  --baseline-root /baseline --baseline-output /artifacts/trio-before.txt \
  --output /artifacts/trio-after.txt \
  --bench-regex '^(BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$' \
  --require-benchmarks BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
```

The baseline needs the same measurement-only Q6 harness files and the
loadCliffSource testing.TB signature. No baseline engine file is changed.
From `cgo_harness`:

```sh
go test -tags treesitter_c_parity . \
  -run '^TestQ6GoFalseErrorLockedC$' -count=1
go test -tags treesitter_c_parity . \
  -run '^TestQ6GoOriginalIncrementalInvariant$' -count=1 -timeout=15m
go test -tags treesitter_c_parity . \
  -run '^TestQ6PythonStackCapLockedCReceipt$' -count=1
GTS_PARITY_MODE=exhaustive GTS_PARITY_IGNORE_KNOWN_SKIPS=1 \
  go test -tags treesitter_c_parity . \
  -run '^(TestParityFreshParse|TestParityIncrementalParse)$/^go$' -count=1
```

Set GTS_Q6_BENCH_LANGUAGE to go, html, or python_issue454 and run
`../scripts/run_randomized_benchmarks.sh` with tags treesitter_c_parity,
package `.`, and bench regex `^BenchmarkQ6Parse$`. Require all eight sample
names: `BenchmarkQ6Parse/{legacy,compact}/{GoFirst,CFirst,CSecond,GoSecond}`.
Supply paired baseline and output paths as above. For peak RSS, compile the
harness once, then run separate processes:

```sh
go test -tags treesitter_c_parity -c -o /artifacts/q6.test
GTS_Q6_BENCH_LANGUAGE=go GTS_Q6_MEMORY_ROUTE=legacy \
  /usr/bin/time -v /artifacts/q6.test \
  -test.run '^TestQ6ParsePeakRSSProbe$' -test.count=1
```

Repeat for C and compact, three processes per route and revision. The full
remaining witnesses can be inspected with the existing TestFirstDiffDiag,
REPRO_LANG and REPRO_FILE, using the same source bytes on both revisions.

NEEDS-APPROVAL: none; no existing pins or thresholds changed. No PR was opened,
following the owner's autonomy instruction. The lead can integrate these
commits with Refs #1310; Closes #1310 would be incorrect.
