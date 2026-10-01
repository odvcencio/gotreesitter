# Shared skipped-token recovery receipt

Focus item 8 measures exact fresh-C trees for C-error files, with no lost
previous exact matches. This receipt compares main `f9828512c` with the shared
skipped-token recovery change. It does not use recovery equivalence as its
success measure.

## Cause and scope

The fallback lexer can skip a valid anonymous terminal that the current parse
state cannot accept. The old gap materializer substituted a childless ERROR
covering the skipped bytes and preceding padding. The C error-mode lexer keeps
the terminal beneath an extra ERROR and excludes its preceding padding.

The shared runtime now tries error-mode lexing over the complete source. It
retains one visible anonymous non-extra terminal only when that terminal ends
exactly at the existing lookahead. It preserves the current scanner, lexer,
stack state, included-range behavior, and span-only fallback. Using the complete
source prevents a truncated gap from being mistaken for a complete token.

Error wrappers use the existing arena child-slice allocator. The first
correct-tree candidate used a heap slice for each retained token; a 1 MiB
allocation probe exposed 58,704 allocations versus main's 443. Arena storage
brings that probe back to 443 allocations and 297,745,936 bytes, both exactly
equal to main, without changing any of the 1,904 corpus results.

The smallest regression is `%O`. Main's digest is
`935bb176e126f8a6f87c6a5685030dc7bc252eb2ea4d33f1f220c8acb68fb701`;
the changed runtime and locked C both produce
`d4252e86530250e52bf0f1dff025f0d7e0989449ca0e17079f7f3d8c0c7670c8`.
The C-parity regression exercises JavaScript's production fallback and the same
fallback in TypeScript, with both admission routes. It also checks 72 edit
steps per grammar and route, fresh/incremental equality, root coverage, ERROR
flags, and zero allocations for a reparse without edits.

## Exact-tree measurement

All 1,844 pinned files and all 60 cluster-A files were checked. Their source
digests matched the pinned manifest and receipt digests. The corpus lock digest
was `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
The lock itself is not part of this change.

Each C-error file was measured with `gts_mismatch -mode check`, using the
supplied locked C reference cache. Equality means the complete canonical tree
digest equals a fresh C parse, including flags, terminal children, fields,
points, and ranges.

| Grammar | C-error files | Main exact | Changed exact |
| --- | ---: | ---: | ---: |
| c | 21 | 7 | 7 |
| go | 7 | 2 | 2 |
| java | 0 | 0 | 0 |
| javascript | 84 | 0 | 62 |
| python | 4 | 0 | 0 |
| rust | 26 | 19 | 19 |
| typescript | 5 | 2 | 2 |
| angular | 3 | 2 | 2 |
| asm | 4 | 2 | 2 |
| blade | 1 | 0 | 0 |
| caddy | 3 | 1 | 1 |
| cairo | 3 | 0 | 0 |
| crystal | 4 | 0 | 0 |
| ebnf | 4 | 0 | 0 |
| fsharp | 2 | 0 | 0 |
| glsl | 2 | 0 | 0 |
| haxe | 4 | 0 | 0 |
| hlsl | 1 | 0 | 0 |
| hurl | 4 | 3 | 3 |
| jinja2 | 4 | 1 | 1 |
| kdl | 3 | 0 | 0 |
| less | 2 | 0 | 0 |
| **Total** | **191** | **39** | **101** |

The pinned seven-grammar subset improves from 30/147 to 92/147. Cluster A
remains 9/44. No prior exact match is lost in either admission route, across
both C-error files and the entire sample. The entire sample improves from
1,713/1,904 to 1,775/1,904 on the default route and from 1,717/1,904 to
1,779/1,904 on the candidate route.

## Other recurring mechanisms

The largest first-divergence signature was a missing ERROR extra flag in 54
JavaScript files. Retaining the skipped terminal fixes 62 complete trees,
including files whose first reported divergence was a range or a later shape.

Other recurring classes remain: premature missing-token insertion after stale
recovery progress in C, TypeScript, ASM, Cairo, and Haxe; wrong root or reduction
grouping at EOF in several grammars; and error-mode keyword/identifier aliasing.
The smallest remaining JavaScript case is `t>`: Go returns an ERROR root where
C returns a program containing an ERROR and identifier.

The pause-progress reset candidate gained 17 exact matches and lost none,
including C improving from 7/21 to 19/21. It was rejected because Agda remained
different from C while fresh allocations rose from 3,223 to 5,706 nodes
(77.0%) and edit allocations from 3,657 to 6,541 (78.9%). Relative to the old
3,297-node pin, its fresh-node rise was 73.1%. The replacement retains the main
counts of 3,223 and 3,657. It does not include the reset.

## Deterministic checks

Both complete 412-row counter measurements, covering 206 languages, are
byte-identical. Their SHA-256 is
`cc32935db65906abdae75112c6df065a7607a25a3606f21de368497ad69104ce`.
Both `perfcounterledger` checks pass without changes to pins or thresholds.
The API/layout check retains 756 root Go files, 12 API tag sets, and no moves.

The skipped-gap unit tests, locked-C regression, edit-session regression,
merge-event census, hot-layout telemetry check, narrow runtime profiles,
focused incremental ledger probes, no-edit allocation checks, and memory
budget witnesses pass. Root `go vet ./...` passes.

The registry-wide edit-session sweep records 391 passes, 9 digest mismatches,
and 12 runs that reach the 90-second execution limit. All 21 failures reproduce
on main, and all nine digest diagnostics are identical. This is not a green
global invariant gate. Digest failures affect awk, JavaScript, meson, twig,
and candidate-route djot. Both routes reach the execution limit for elsa,
godot_resource, haskell, kotlin, nickel, and powershell.

Tagged harness vet also reports invalid fixture code and existing copy-lock
diagnostics, identical on main and the changed runtime. No gate, expectation,
test, or pin is weakened by this change.

## Timing protocol

`BenchmarkSharedRecoveryFile` measures the complete public parse and tree
release, including lexer creation, retries, and materialization. Both runtimes
reuse a parser; grammar loading and parser setup are outside the timer. C
allocation metrics count the Go bridge, so C's native memory is measured with
RSS separately.

Before/after runs use `scripts/run_randomized_benchmarks.sh`, alternating the
baseline and current order, with 20 seeds, `GOMAXPROCS=1`, `-count=1`,
`-benchtime=750ms`, and `-benchmem`. Each process measures one grammar in paired
Go-C-C-Go subbenchmarks. The baseline receives the identical benchmark source
but retains main's runtime. Its benchmark source digest is
`4a36f9282626c6ba9fb4e0e41108e0d89811712b540a46e21bea7a662fa2c0cb`.

To reproduce a paired fixture run, supply the same locked C cache and fixture
in both checkouts, then run from `cgo_harness`:

```sh
GTS_SHARED_RECOVERY_LANGUAGE=javascript \
GTS_SHARED_RECOVERY_FILE="$FIXTURE" \
bash ../scripts/run_randomized_benchmarks.sh \
  --output "$RESULTS/current.txt" \
  --baseline-root "$BASELINE/cgo_harness" \
  --baseline-output "$RESULTS/main.txt" \
  --package . --tags treesitter_c_parity \
  --bench-regex '^BenchmarkSharedRecoveryFile$' \
  --require-benchmarks 'BenchmarkSharedRecoveryFile/Go,BenchmarkSharedRecoveryFile/C,BenchmarkSharedRecoveryFile/C#01,BenchmarkSharedRecoveryFile/Go#01'
```

The dense recovery fixture repeats `let x = 1;\n%F(x);\n` to 1,048,590 bytes,
with 58,255 skipped terminals. It is not exact on main and is exact on the
changed runtime in both admission routes. RSS probes use separate Go and C
benchmark subprocesses, one complete dense parse each, under
`/usr/bin/time -v`. Main Go peaks at 382,872 KiB; the changed runtime peaks at
380,624 KiB (371.7 bytes per source byte, below the 400-byte limit). Main C
peaks at 144,100 KiB and changed C at 143,992 KiB. These single RSS probes do
not establish a timing trend.

The 1 MiB timing fixture starts with `%F(x);\n` and repeats
`let x = 1;\nF(x);\n` to 1,048,584 bytes. It exercises one skipped-terminal
recovery during a complete large parse. The focused timing matrix selects this
input, three median affected locked JavaScript files, the largest affected locked
JavaScript file, the Agda ledger fixture, the C FSM fixture, and the standard Go trio.
The dense fixture's 20-seed timing, generated 32/137 KiB timing, edit latency
percentiles, and a full language-graduation performance matrix remain unverified.

The single-error 1 MiB timing run is incomplete. Its first baseline benchmark
process was still running after 175 seconds, at an observed 716,204 KiB
resident set, without yielding a valid timing row. The run was stopped before
the three-hour task limit to publish the verified work. Neither it nor a
partial seed is used as comparison evidence. The dense fixture's accepted
parse, exact-C equality, allocation counts, and RSS checks above are separate
completed measurements. The cause of the single-error baseline stall remains
unresolved.

## Timing results

The following are 20-seed medians. Within each seed, the two Go times and the
two C times in the Go-C-C-Go cycle are averaged separately. Paired Go/C is
the median of those per-seed ratios, rather than a quotient of the two
independently sorted time medians. Bytes and allocations use the same cycle
averaging; the displayed byte values are rounded. Every timed operation is
accepted and its root covers the complete input.

| Fixture (bytes) | Go ns/op: main → changed | C ns/op: main → changed | Paired Go/C | Go B/op | Go allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| medium__array-next-2.js (644) | 608,174 → 614,642 | 810,576 → 844,057 | 0.646× → 0.642× | 16,265 → 16,265 | 46 → 46 |
| medium__regress-1455517.js (644) | 1,392,610 → 1,269,519 | 426,841 → 478,607 | 3.067× → 2.697× | 23,386 → 23,386 | 351 → 351 |
| medium__regress-417169470-1.js (644) | 778,218 → 766,110 | 413,286 → 407,471 | 1.899× → 1.859× | 21,162 → 21,162 | 243 → 243 |
| large__typedarray-map.js (1022) | 1,125,653 → 1,095,146 | 560,269 → 544,562 | 2.056× → 2.008× | 22,450 → 22,450 | 331 → 331 |
| Agda ledger fixture (2008) | 5,086,233 → 5,149,506 | 3,263,648 → 3,300,712 | 1.565× → 1.548× | 31,478 → 31,511 | 340 → 340 |
| large__fsm-health-linux.c (775) | 463,301 → 475,242 | 469,113 → 459,192 | 0.970× → 1.039× | 5,738 → 5,742 | 95 → 95 |

The directional Go time rises are 1.1% for the first JavaScript file, 1.2%
for Agda, and 2.6% for C. The C paired ratio rises 7.2%, within the 10%
ratchet. JavaScript's other three cycle medians improve. Individual-series
`benchstat` comparisons find no significant timing change in these six
fixtures; JavaScript timing confidence intervals are wide. The three median files and
the largest file are selected from the 62 newly exact JavaScript files.
The largest affected file is 1,022 bytes; the overall locked corpus includes
1,024-byte files, whose timing is not measured here. These runs support
the ratchet checks, not a claim that the change improves speed.

All six Go allocation counts are unchanged. Agda bytes rise 0.11% and C bytes
rise 0.07%; the JavaScript byte medians are unchanged. No counter, allocation,
or RSS threshold is changed to accept these results. Agda still has the main
tree's error flag where C is clean, and its Go digests are unchanged in both
routes. This pre-existing correctness failure is not repaired by this change.

The standard Go trio also completes 20 paired seeds:

| Benchmark | ns/op: main → changed | B/op: main → changed | allocs/op: main → changed |
| --- | ---: | ---: | ---: |
| GoParseFullDFA | 24,029,186 → 24,616,134 | 1,325 → 1,338 | 8 → 8 |
| GoParseIncrementalSingleByteEditDFA | 529,440 → 424,568 | 392 → 390 | 5 → 5 |
| GoParseIncrementalNoEditDFA | 23.415 → 20.315 | 0 → 0 | 0 → 0 |

Full-parse time rises 2.4% and its bytes rise 1.0%; edit time falls 19.8%
and no-edit time falls 13.2%. These are directional medians. `benchstat`
finds no significant timing difference (p=0.883, 0.289, and 0.925,
respectively); timing confidence intervals are wide. All allocation counts
are unchanged, and no hard failure or 10% ratchet is introduced by these
comparisons.
