# Bounded WASM tree walks, 2026-09-30

The valid 146,293-byte Go reproduction and two pinned 1 MiB Go inputs now
complete in Node, headless Chromium and headless Firefox. The baseline crashes
in all three hosts on both 1 MiB inputs. The bridge exercises opening a document,
a same-length one-byte edit, an unchanged update, highlights, queries, structured
tree output and document close. It checks clean roots that cover the full input,
UTF-16 coordinates and the existing output limits.

METRIC: crash-free 1 MiB execution | 0/3 -> 3/3 | f9828512c / 839bf44aa | functions-1m and nested-1m
METRIC: deterministic counter rows changed | 0 -> 0 | f9828512c / 839bf44aa | 412 rows, 206 grammars

The engine increments replace input-depth recursion in hidden flattening,
field-presence detection, edits, pending-parent materialization, error ranking,
column invalidation, missing-node dependency propagation, comparisons, S-expression
output and WASM JSON construction/encoding. Explicit frames retain traversal,
field, padding, cache ownership and output order. The existing 16,384-frame
retention ceiling and parser memory budgets stay unchanged. Frame reservation
counts the next hidden descent only when a buffer fills, avoiding repeated
geometric allocations on long lists.

The first increment, `9dc222caf`, independently changes the exact 1 MiB
function-list case from 0/3 to 3/3 passing hosts. Its matched source snapshot
contains the same changed engine files as that commit; `first-increment-*.json`
records its immutable artifact and workload identities. Its 20-seed native trio
keeps full parse at 8.858 ms, improves edit 181.3 -> 162.3 us (-10.43%), and
improves no-edit 8.149 -> 7.501 ns (-7.96%), with unchanged allocation counts.
Its fresh, edited and independently reparsed 1 MiB trees match locked C 0.27.
This is the smallest measured safety increment. Later increments also make the
12,000-level nesting case complete in every host.

METRIC: crash-free 1 MiB function-list execution | 0/3 -> 3/3 | f9828512c / 9dc222caf | functions-1m, Node/Chromium/Firefox
METRIC: native single-byte edit us | 181.3 -> 162.3 | f9828512c / 9dc222caf | BenchmarkGoParseIncrementalSingleByteEditDFA, 20 alternating seeds

## Revisions and workload identities

Baseline: `f9828512c`. Engine increments: `9dc222caf`, `6127d35d3` and measurement tooling `1482de17f`;
frame reservation is `839bf44aa`. Final measurements are recorded below.
Go WASM toolchain: 1.26.8. Native Docker toolchain: 1.25.
Node: 22.23.3. Playwright: 1.63.0. Chromium: 153.0.8010.12. Firefox: 155.0.

| Workload | Bytes | Shape | Source SHA-256 |
| --- | ---: | --- | --- |
| functions-64k | 65,536 | 2,052 function declarations | `787a5f02172d7d89be5f339e9653c3785e28d4004520489a5320dccb54323da5` |
| functions-4500 | 146,293 | Original 4,500-function reproduction | `a3e6f63a616dfb7c9ee1b739c9082e49066fd00d494fc6691cc4b713efdbe833` |
| functions-1m | 1,048,576 | 30,593 function declarations | `66e6e2d95fc2b124fe7b5050dc3228d1ea01c79d10c47e47118a230a6146861d` |
| nested-1m | 1,048,576 | 12,000 parenthesis levels, trailing comment padding | `91f2a2bbd6706b84f381429d4ffa3c71a84170ef00e489f5e2d3d45b816e2717` |

`internal/wasmfixtures` pins the native sources; the JavaScript probe checks both
source and edited-source digests. No grammar, counter, allowlist or gate pin was
changed. The external corpus lock was fetched and verified as
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`;
the lock itself is not committed.

The C oracle is runtime `0.27.0` at
`6070dbfefd326bd735e5683eb128cc1b57dad0c0`, obtained from the owner's pin
commit `3a0e7df5`. Its Go grammar is
`2346a3ab1bb3857b48b29d779a1ef9799a248cd7`, with artifact SHA-256
`f17d674be6e3e7a0edb521defa31d3174c877b01414fd758a4988673423573f7`.
The pin's harness ran from a temporary copy with a module replacement to each
engine revision. This work does not change the repository's C-runtime pin.

## Correctness and invariants

The Docker tests match every fresh and edited fixture node against locked C:
type, named/missing/error flags, HasError, spans, points, child count and fields.
An independent fresh Go parse of each edited input also matches. Native UTF-16
complete-operation JSON payloads match for all four cases: 301,831; 669,388;
4,653,393; and 169 bytes respectively.

The deterministic ledger has 412 identical rows for 206 grammars, before any
timing comparison. The invariant fleet runs one grammar per test process. At
`6127d35d3`, 202 grammars pass; JavaScript, awk, meson and twig retain their
baseline failures. Five baseline timeouts now pass: Kotlin, Haskell, PowerShell,
elsa and godot_resource. Removing repeated recursive dependency walks resolves
those timeouts. No failed gate was disabled, no threshold changed, and no
language was graduated. See `invariant-fleet.tsv` for exit codes.

Focused Docker tests cover deep flattening, fields, edits, pending parents,
error ranking/cost, alias comparison, HasError, column dependencies and parser
memory budgets. The full Go incremental invariant race test passes in Docker
(1,087 seconds); focused deep traversal and WASM output race tests also pass.
WASM-compiled deep tree tests run under Node, alongside WASM JSON unit tests.
Existing unrelated external tests do not compile with the Go-only grammar
subset; the root WASM check therefore compiles the package's normal Go files
with the focused deep regression file explicitly.

## Performance boundary and earlier candidates

Benchmark receipts retain seed markers, all measurement rows and completion
status; repeated oracle log messages are omitted. Every comparison uses `scripts/run_randomized_benchmarks.sh`, 20 explicit shuffle
seeds, one process per seed, count 1, 750 ms, GOMAXPROCS=1 and benchmem. Baseline
and candidate order alternates. Native complete operations execute Go-C-C-Go
cycles and shuffle fixture/operation order. Their boundary includes UTF-16 input
copying, edit discovery, Tree.Edit, parsing, every identifier capture, common JSON
encoding and release; parser/query setup is outside the operation. These are
native frontends, not a C-WASM bridge comparison. C malloc is absent from Go
B/op, so those allocation columns are not total C memory measurements.

The actual Node benchmark measures complete JavaScript bridge calls. Its B/op
and allocs/op belong to the small Go subprocess driver, not to the WASM parser.
Its separate edit, open, unchanged-update and RSS metrics come from Node.

At `6127d35d3`, the default native benchmark trio measured full parse
14.82 -> 15.04 ms (+1.43%), edit 283.5 -> 282.4 us (no significant change),
and no-edit 15.02 -> 13.51 ns (-10.02%). Full parse retained 8 allocations;
edit retained 5; no-edit retained zero bytes and allocations. Production-tag
full parse measured 14.72 -> 15.03 ms (+2.12%). These small full-parse costs are
recorded as the tradeoff for replacing stack recursion; their full-parse cost stays below 10%; tail validation is discussed below.

That revision's actual 64 KiB Node edit measured 430.0 -> 456.8 ms (+6.22%),
unchanged update 181.4 -> 172.7 ms (-4.82%), and open 1.114 -> 1.105 s
(no significant change). The audit's 489 ms observation was not from the same
available artifact and cannot serve as an apples-to-apples baseline.

Rejected candidates include unpooled flatten frames (full-parse B/op
1,276 -> 121,602; allocations 8 -> 17), pooling only the reduce frames
(B/op 1,276 -> 31,741; allocations 8 -> 12; edit +10.41%), and removing
clean-node dependency fast paths (64 KiB Node edit 677.9 -> 737.7 ms, +8.82%).
The accepted pooling and clean-node fast paths address those regressions.
The later frame-reservation increment addresses the 1 MiB edit allocation
increase at `6127d35d3` (293,148,446 -> 325,478,152 B/op, +11.03%).

## Memory and limits

The native complete-operation 1 MiB process RSS probe measured
213,032 -> 212,220 KiB at `6127d35d3`, then 199,772 KiB with frame reservation.
The locked C frontend measured 96,416 -> 100,804 KiB. These are fixed-count diagnostic memory
probes, not timing comparisons. The native 400 bytes/source-byte budget passes.
Node's four-case run peaked at 894,304 KiB; Chromium's sampled browser process
tree peaked at 1,272,074,240 bytes and Firefox's at 1,541,726,208 bytes. Browser
RSS sums process resident sets and can count shared pages more than once.

A 15-operation Node retention probe with an event-loop yield between document
transactions passes without crashing, but WASM linear memory reaches 1,415 MiB
and Node RSS reaches about 1.7 GiB. A separate uncommitted diagnostic build
forces Go GC every five operations: live heap is stable at 297,112,920 bytes
at operations 10 and 15, while reserved linear memory continues growing.
This rules out growth in retained Go heap in that probe, not allocator
high-water growth. WASM memory reduction remains work; this receipt does not
claim all memory targets or all v1 graduation gates are green. Recursive mixed
payload copying in Tree.Copy and existing depth-limited legacy comparisons were
not part of the exercised bridge paths and remain unchanged.

## Final frame-reservation measurements

The final artifact is 19,891,882 bytes; gzip level 9 is 4,979,681 bytes, compared
with baseline 19,819,239 / 4,963,302 bytes (+0.33% gzip). The matching bootstrap
adds 4,348 gzip bytes and external Go blob adds 47,950; total shipped gzip assets
are 5,015,600 -> 5,031,979 bytes. The final WASM SHA-256 is
`2f946f44c771bac24cff12a3b9321b50077847730cd652d6e432984e8fd8b2ee`.
The native benchmark source fingerprint is in `reserve-engine-source-sha.json`.

METRIC: WASM gzip bytes | 4963302 -> 4979681 | f9828512c / 839bf44aa | Go-only external-blob build, Go 1.26.8, gzip level 9
METRIC: 64 KiB Node edit ms | 471.6 -> 489.8 | f9828512c / 839bf44aa | functions-64k, 20 alternating seeds, unrestricted Node

The unrestricted Node run measures open 719.3 -> 672.7 ms and unchanged update
211.0 -> 216.9 ms; none of these time differences is significant. Peak RSS is
259.3 -> 254.9 MiB (-1.70%). A separate single-CPU run under load measures edit
734.5 -> 771.8 ms (+5.08%) and unchanged update 298.5 -> 283.7 ms (-4.95%).
Both complete-process timings are unchanged. Both raw runs are retained; the
absolute latency depends on host scheduling and JavaScript worker availability.

The first final default-trio run reports full parse 17.22 -> 19.18 ms (+11.35%)
with wide intervals (baseline +/-15%, candidate +/-42%). Edits and no-edit timings
have no significant change; allocations remain 8, 5 and 0 respectively. A second
20-seed run on another CPU reports full parse 19.84 -> 27.83 ms with intervals
of +/-58% and +/-42%, and no significant difference. These default-trio timings
remain unresolved under concurrent host load. The stable production run reports
full parse 15.18 -> 15.38 ms (+1.35%), edit 315.8 -> 309.7 us (no significant
change), and no-edit 14.52 -> 14.12 ns (-2.72%); allocation counts stay 8/5/0. The complete
operation Go/C comparisons remain the performance-ratchet boundary, rather than
treating the noisy Go-only result as a C comparison.

The final complete-operation measurements average the two Go and two C results
within each process, then take medians across all 20 seeds. The ratio column is
the median of each seed's Go/C ratio, rather than the ratio of separate medians.

| Operation | Go/C before -> after | Go ms/op before -> after | Go B/op before -> after | Go allocs/op before -> after |
| --- | --- | --- | --- | --- |
| 64 KiB full | 1.143 -> 1.152 | 127.30 -> 128.89 | 2,534,709 -> 2,586,687 | 12,354.5 -> 12,354.75 |
| 64 KiB edit | 3.931 -> 3.944 | 161.49 -> 162.53 | 3,299,238 -> 3,299,238 | 12,397 -> 12,397 |
| 1 MiB full | 1.247 -> 1.249 | 2,027.54 -> 2,050.25 | 229,494,778 -> 232,435,964 | 184,195.5 -> 184,201 |
| 1 MiB edit | 3.153 -> 3.266 | 2,673.47 -> 2,717.69 | 296,943,212 -> 302,825,324 | 183,829.5 -> 183,839.5 |

METRIC: complete-operation 1 MiB edit Go/C | 3.153 -> 3.266 | f9828512c / 839bf44aa | functions-1m, native UTF-16 copy/edit/parse/all captures/JSON/release, 20 Go-C-C-Go cycles
METRIC: native 1 MiB peak RSS KiB | 213032 -> 199772 | f9828512c / 839bf44aa | functions-1m full complete-operation diagnostic

The frame reservation reduces the earlier 11.03% edit-byte increase to 1.98%
in its own paired comparison; full-operation bytes increase 1.28%. Go/C median
ratios increase 0.83%, 0.31%, 0.21% and 3.60% respectively. These changes and
allocation counts stay within the 10% ratchets; the deterministic ledger is
identical. Timing is measured only for Go; other grammars have counter and
invariant coverage but no before/after timing receipts in this lane.

Individual positions in the 1 MiB cycle differ: the first Go edit changes
2.596 -> 2.471 s (-4.83%), while the second changes 2.725 -> 3.070 s (+12.65%).
Across the 40 single-iteration Go edit samples, the interpolated 99th percentile
is 2.953 -> 3.286 s (+11.27%). The 64 KiB Node samples give 528.7 -> 553.6 ms
(+4.71%). These tail estimates use few samples and do not implement the design's
137 KiB/1 MiB, 16-site insertion/deletion/replacement session gate. The observed
1 MiB tail increase exceeds 10% and remains a performance concern; this receipt
does not claim that tail-latency graduation gate passed. A dedicated isolated
edit-session run is the smallest next check, followed by removing hot-path
frame-growth calls when capacity is already available if that cost persists.
No gate threshold or pin was relaxed to accept this observation.

The final four-case Node run peaks at 942,804 KiB. Sampled browser process-tree
RSS is 1,271,410,688 bytes for Chromium and 1,573,326,848 for Firefox. All three
hosts pass all four cases; structured output still stops at its existing node
limit and queries at their existing match limit. Complete parse and highlight
work is validated independently against C, so these output caps do not hide
input truncation.

## Reproduction

Build the Go-only external-blob WASM artifact with its matching Go bootstrap:

```sh
GOWORK=off GOOS=js GOARCH=wasm go build \
  -tags grammar_subset,grammar_subset_go,grammar_blobs_external \
  -o runtime.wasm ./wasm/runtime
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" wasm_exec.js
GOWORK=off GTS_WASM_PROGRAM_AUDIT=1 GTS_WASM_HOST=node \
  node scripts/measure_wasm_runtime.cjs \
  runtime.wasm wasm_exec.js grammars/grammar_blobs/go.bin
```

Install Playwright 1.63.0 and its Chromium and Firefox binaries in an external
scratch directory; expose that directory's node_modules through NODE_PATH.
Repeat the probe with GTS_WASM_HOST=chromium and GTS_WASM_HOST=firefox.
GTS_WASM_CASE selects a pinned workload; GTS_WASM_REPETITIONS controls retention
iterations. The Node comparison wrapper takes baseline WASM, candidate WASM,
bootstrap, Go blob, baseline output and candidate output as six arguments.
Run correctness in Docker before the randomized timing wrapper.

The stated audit directory was unavailable. Its script was reconstructed from
`392dc8e2` and extended with pinned exact-size workloads, browser execution and
checks. Gzip is measured at level 9 on freshly rebuilt artifacts, not inferred
from the old 4.82 MB report. Source padding is explicit, especially in the nested
case. The host had other independent lanes running; paired randomized processes
and CPU pinning reduce but do not eliminate that timing noise.

NEEDS-APPROVAL: none. No existing pins were refreshed. The branch is pushed for
lead integration; no PR was opened, following the owner's overriding autonomy
instruction.
