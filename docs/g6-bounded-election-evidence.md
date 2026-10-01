# Bounded merge-election investigation

No tested runtime repair passed every gate. The final branch preserves main's engine,
all existing pins, and all gate thresholds. It adds seven portable locked-C
witnesses, an oracle authentication test, benchmark tools, and measurements of
the rejected repair.

The baseline is `f9828512cc58f1bc77b856c2b3451e85266c38b1`. The C oracle uses
runtime 0.25.1 at `f5afe475deb7c0bae6407fb776c76824f717bb61`, with the repository's
v0.25.0 Go binding and grammar lock. This repository pin governs the comparison;
no runtime upgrade was made. The external corpus lock was authenticated with
SHA-256 `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
The lock itself is not included.

Main accepts all seven small witnesses without errors but disagrees with the
locked C tree. Its GSS link union can erase a different completed subtree while
retaining an incumbent at equal dynamic precedence. Separately, some convergent
reductions need an election before the terminal frontier. Extracting only the
mixed flat/GSS unit-parent election fixes four witnesses; a bounded subtree
choice inside the existing link union fixes the remaining three. Combining them
matches C on all seven, but also performs subtree comparisons during ordinary
Meson conflicts and fails the performance gate.

The rejected combined repair adds 104 lines and removes two across `glr.go`,
`parser.go`, and `parser_recover_c.go`. Its diff has SHA-256
`664e088a9895fee903f87d17f5bed76abf45661f9f80abb9e0ed7fe197935ebe`.
It extracted the engine changes from `c1445bd7b` without the completed-subtree
refusal from `ec94ee46a`, then added a tie election in `gssMainAddLinkSeenMutate`.
The tie election required equal dynamic precedence, captured raw topology,
active-arena ownership for both payloads, and legacy version order. It replaced
an incumbent only after a complete bounded C subtree comparison preferred the
candidate. It applied both when the predecessor was identical and after
predecessors merged successfully. It added no parser or reuse-cursor fields,
exported runtime identifiers, root Go files, or grammar-name conditions.

| Rejected candidate | Witness matches / 7 | Reason for rejection |
| --- | ---: | --- |
| Mixed unit-parent election only | 4 | Bash, C#, and the 55-byte D witness remain wrong. |
| Widen mixed parent count | 4 | Still misses those three witnesses. Meson tokens 365→364; reused bytes 309→322. |
| Elect all same-pop pairs | 7 | Meson reused bytes 309→0. |
| Replace only when candidate wins | 7 | Less tokens 327→1090 and reuse 1118→0; Move tokens 193→201 and reuse 778→712; Dart block splices 103→75. |
| Require equal production/symbol, clean owned topology, single links, exact bounded comparison and equal shifted status | 7 | Move reuse still falls; Meson edit invariant remains red. |
| Merge-boundary election only | 4 | C#, D declaration, and Dart remain wrong. |
| Frontier election only | 7 | Move reuse falls; Meson edit invariant remains red. |
| Terminal-frontier election only | 6 | Bash remains wrong; Move reuse still falls. Meson tokens/reuse return to baseline. |
| Existing link-union tie election only | 2 | Fixes Bash and C#; misses the other five. |
| Unit-parent election plus link-union tie election | 7 | Meson edit time +12.92%, C# edit allocations +18.60%; counter and census pins also move. |

Each candidate was evaluated against authenticated portable sources. Requests
for the compact route currently decline to legacy on these sources, both before
and after the combined repair. A successful requested-route comparison therefore
does not prove compact-engine graduation. The JSON witnesses include the exact
source, source digest, C deep-tree digest, and baseline divergence, so a future
compact repair can reuse them without an external corpus.

On a four-file receipt subset per grammar, main→combined fresh exact matches were
Bash 3→4, C# 3→4, D 1→2, Dart 3→4, Solidity 3→3, and Meson 0→0. No previously
matching file in this subset lost its match. This is a narrow diagnostic subset,
not a full corpus parity or graduation receipt.

The deterministic ledger evaluated all 412 rows across 206 grammars. Main
passed. Combined failed first on Move edit tokens 193→199 (+3.11%). Move new
nodes rose 1092→1158, multi-version token share 487046→547738 ppm, reused bytes
fell 778→716, and block splices fell 113→107. Move's authenticated full and edited
samples now matched the locked C fresh tree, whereas main differed in expression
type. This could justify a separately reviewed pin refresh if the repair had
passed its other gates. No pins were changed because the repair was rejected.

C# full/edit deterministic counters were unchanged: tokens 273/273, new nodes
1903/2029, peak live versions 21/21. Meson full/edit tokens remained 487/365;
edit reused bytes remained 309 and block splices 122. Meson new nodes fell
4242→4231 full and 4033→4022 edit. These unchanged work/reuse counts did not
capture the added comparator allocation cost.

All other changed default-route rows were Circom full nodes 1044→1008 and
multi-version share 393782→370466 ppm; F# full nodes 9726→9777, edit nodes
9786→9837, and full/edit multi-version share 859683→863636 ppm; Verilog full
nodes 3541→3533. The F# increases are below the ledger's 2% threshold but still
move in the wrong direction. The counter receipt records all changed rows,
including unchanged fields and fixture/session digests. Candidate-route ledger
rows were unchanged.

The constructed G6 census passed on main: 108 sources, 150 C merges, 5 Go
merges, and zero sources where Go over-merges. Combined retained zero over-merge
sources but reduced Go merges 5→4, failing the existing exact census pin. The
pin was not refreshed. The changed case is
`apex/bind_variable_soql_with_generic_local`, with counted Go merges 1→0
against C's 10. Both Go versions produce the same incorrect deep tree digest
`bc754c2991e0c9a1ef1fd5bd109e56c2f2cb37ce1b9fa7463e5406ae6f847af1`;
C produces `aafdcb926b91a7ae390604b4711906cb9d0736cca9efed49021baa8e4c36cfcf`.
This does not prove the census move comes from a more-correct tree. A census pin
refresh is therefore not justified by this witness.

Meson's R4 edit-session invariant already fails on main at step 1. Both parses
accept EOF, cover the input, and report no errors, but their deep digests differ.
Main minimizes to insertion of `x` at byte zero:

```text
before: t(f(g())=='')
after:  xt(f(g())=='')
```

The result is 13 bytes; main's incremental result matches C, while its fresh
result does not. Combined minimizes from the authenticated 2050-byte sample to:

```text
before: t(f(g[1].h())=='')
after:  xt(f(g[1].h())=='')
```

The result is 18 bytes; combined's fresh result matches C and its incremental
result does not. This inherited failure prevents claiming the edit invariant is
green. Neither variant crashes, truncates, or reports an error on these two
minimal sources.

| Go operation | Median time main→rejected | Median B/op main→rejected | Median allocs/op main→rejected |
| --- | ---: | ---: | ---: |
| C# full | 8.295→8.555 ms | 1,100,754→1,115,981 | 667→789 |
| C# one edit | 8.432→8.634 ms | 1,101,099→1,116,903 | 672→797 |
| C# no edit | 13.635→13.205 ns | 0→0 | 0→0 |
| Meson full | 4.907→5.814 ms | 6,343→129,006 | 22→739 |
| Meson one edit | 5.924→6.689 ms | 856,659→976,471 | 32→731 |
| Meson no edit | 13.310→13.340 ns | 0→0 | 0→0 |

Both timing methods use `scripts/run_randomized_benchmarks.sh`, paired main
and candidate runs, seeds 1–20, one process per seed, `GOMAXPROCS=1`,
`-count=1`, `-benchtime=750ms`, and `-benchmem`. No fixed-order benchmark run
serves as comparison evidence. Benchstat finds Meson full and edit timing
regressions significant (`p=0.000`); C# full and edit timing differences are not
significant (`p=0.221` and `p=0.201`). C# allocation regressions are significant
(`p=0.000`). Both no-edit benchmarks allocate zero bytes and zero objects.

| Complete operation | Median Go ms main→rejected | Median C ms main→rejected | Median paired Go/C main→rejected |
| --- | ---: | ---: | ---: |
| C# full | 18.041→19.117 | 2.470→2.634 | 7.32→7.39 |
| C# edit | 18.773→18.817 | 0.116→0.120 | 162.14→163.00 |
| Meson full | 6.833→9.119 | 1.463→1.380 | 5.01→5.90 |
| Meson edit | 7.530→9.691 | 0.715→0.655 | 11.72→13.21 |

For each seed, the paired ratio is `(Go_first + Go_second) / (C_first + C_second)`.
The table reports its median across 20 seeds; it is not the ratio of the two
separately reported medians.

The Go-only campaign uses Go 1.26.8; the C-paired campaign uses Go 1.25.14.
Each main/candidate pair uses the same compiler. These are separate builds and
campaigns; their absolute Go times should not be mixed.

The C comparison times complete public operations through the pinned C binding:
full parse plus result close, or Tree.Edit plus incremental parse plus result
close. Go times the corresponding public operations, including its edit
invalidation work and result release. Old-tree preparation and old-tree release
are outside both edit timers. C prepares old trees by cloning an unchanged base;
Go reparses them to preserve its first-edit dependency fold. Each process runs
Go-C-C-Go for each phase. C native heap allocations are not represented by Go's
`B/op` or `allocs/op` readings. The 2 KiB C# and Meson timing samples have known
fresh-tree mismatches in both versions, so these timings diagnose cost and do
not constitute parity admission evidence.

| Deterministic large source | Bytes | Peak RSS KiB main→rejected | Effect |
| --- | ---: | ---: | --- |
| C#: 30,000 `class CN { int x = N; }` lines | 937,780 | 834,960→961,712 | +15.18% (815.4→939.2 MiB) |
| Meson: 30,000 `vN = N` lines | 427,780 | 772,824→787,100 | +1.85% (754.7→768.7 MiB) |

Both sources use N=0…29999. Each probe runs in a separate process with Go
1.26.8, `GOMAXPROCS=1`, default parser budgets, and `/usr/bin/time -v`, inside
Docker. Both versions cover the full input but stop at `memory_budget` and
report an error on both workloads. They return without a crash or OOM; they do
not certify accepted completion. C#'s RSS increase exceeds the 10% ratchet.

A full-parse Meson allocation profile at `-memprofilerate=1` attributes 77,436
allocation objects (83.39% of all process allocations, including setup) to
`compareRawStackEntriesCExact` in the combined repair. The unprofiled full-parse
benchmark adds exactly 717 objects per operation (22→739). This points to
comparator slice growth as the allocator cost. The profile is attribution
evidence, not before/after timing evidence.

| Gate | Main/final branch | Rejected combined repair |
| --- | --- | --- |
| Seven portable Go/C witnesses | FAIL: 0/7 exact Go matches | PASS: 7/7 exact matches |
| C witness oracle authentication | PASS: 7/7 | C expectations unchanged |
| `TestMergeEventCensusBaseline` | PASS: Go 5, C 150, zero over-merges | FAIL: Go 4, C 150, zero over-merges |
| Deterministic counter ledger | PASS: 412 rows, 206 grammars | FAIL: Move tokens/reuse; other directional rises recorded above |
| R4 edit sessions and invariants | PASS for Bash, C#, D, Solidity, Dart, Circom, F#, Move, Verilog (72 steps each); FAIL Meson step 1 | Same nine passes; FAIL Meson step 1 |
| Pinned probe ledger | PASS: JavaScript, TOML, Python, Go, TypeScript, both routes | PASS: same five languages, both routes |
| Pinned reconstructed session ledger | PASS: all thirteen languages, both routes | PASS: all thirteen languages, both routes |
| Focused GSS, reduction, comparator, hot-layout and memory-budget tests | PASS: 43 tests | PASS: 43 tests |
| `TestBuiltinRuntimeProfilesStayNarrow` | PASS | PASS |
| Grammar name detection | PASS | Authenticated benchmark loaders also exercised |
| Root `go vet ./...` | PASS | PASS before restore |
| Tagged C-harness `go vet -tags treesitter_c_parity .` | FAIL: five existing lock-copy warnings, identical to main | No new diagnostic from added benchmark code |
| Root-file/API guardrails | PASS: 756/756 Go files, 12 API tag sets | PASS before restore |
| Twenty-seed Go and C-paired campaigns | COMPLETE | FAIL performance: Meson time, C# allocations and RSS |

The C-harness vet diagnostics refer to copies of `Language` (which contains
`sync.Once`) in five existing tests. Main and the final branch produce identical
diagnostics. No existing test, expectation, allowlist, threshold, or production
Go file was changed. Full multi-size cliff and release-graduation campaigns were
not completed for this rejected repair.

Reproduce the portable oracle check one grammar per container:

```sh
for language in bash c_sharp d solidity dart; do
  bash cgo_harness/docker/run_parity_in_docker.sh -- \
    "cd /workspace/cgo_harness && GOWORK=off GOMAXPROCS=1 GTS_MERGE_ELECTION_LANG=$language go test -tags treesitter_c_parity -run '^TestMergeElectionWitnessOracle$' -count=1 -v ."
done
```

The oracle authentication test checks the locked C runtime, input digests,
complete error-free C parses, and pinned C deep-tree digests. It does not assert
that unresolved Go witnesses are fixed. No failing Go regression test or engine
repair is shipped.

Run the Go benchmark in paired checkouts, with the same benchmark source present
on main and candidate:

```sh
GOWORK=off GTS_LEGACY_ELECTION_BENCH_LANG=meson \
  bash scripts/run_randomized_benchmarks.sh \
  --package ./grammars --tags '' --bench-regex '^BenchmarkLegacyMergeElection$' \
  --baseline-root "$BASELINE_ROOT" --baseline-output /tmp/meson-main.txt \
  --output /tmp/meson-candidate.txt --runs 20 --benchtime 750ms \
  --require-benchmarks 'BenchmarkLegacyMergeElection/meson/full,BenchmarkLegacyMergeElection/meson/edit,BenchmarkLegacyMergeElection/meson/no_edit'
```

For complete-operation C comparisons, run the same script inside Docker from
`cgo_harness`, with `--package . --tags treesitter_c_parity`, benchmark regex
`^BenchmarkMergeElectionComplete$`, and a baseline root pointing at the baseline
checkout's `cgo_harness` directory. Require each of the eight exact benchmark
names for `full`/`edit` and `go_first`/`c_first`/`c_second`/`go_second`.

The sanitized receipts retain every measured benchmark row and shuffle seed;
local checkout paths and machine-identifying metadata are omitted. They can be
passed directly to benchstat. The runtime repair, Meson edit-invariant repair,
and compact-route admission remain outstanding. NEEDS-APPROVAL: none; no pin or
threshold refresh is proposed.
