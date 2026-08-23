# Compact-lane perf attribution board

This document defines one boundary-exact attribution tree over the compact
parser core's fresh-full-parse central processing unit (CPU) time. It states
the disambiguation rule for
every function that more than one component could otherwise claim. It
records the noise floor of the local measurement host, and the first
published receipt.

This is measurement infrastructure only. It changes no parser code, no
routing, and no shipped behavior.

## 2026-08-23 P25a fresh-profile performance blocker receipt

Status: **KEEP LIVE / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`0448715e9a80305556b687b6ecaf041da42e9d9d`.

P25a used the fresh profile settings required by P24i. The artifacts prove the
recorded commands, but they do not prove a quiet host before or during the run.
The run used one central processing unit (CPU), `GOMAXPROCS=1`, one process per
benchmark, and sequential benchmark execution. The process was pinned to CPU
19 with `taskset`. The benchmark used one count, one test CPU, benchmark memory
reporting, and a five-second profile window. The benchmark invocation used
`GOFLAGS=-p=1`.

The artifact set records benchmark process logs only. It does not measure
unrelated host activity or prove the absence of a concurrent workload.

The exact benchmark command was:

```text
taskset -c 19 env GOMAXPROCS=1 GOFLAGS=-p=1 GOT_BENCH_FUNC_COUNT=500 GOT_GLR_MAX_STACKS=6 /tmp/p25a-profile-20260824-artifacts/gotreesitter.test -test.run '^$' -test.bench '^BenchmarkName$' -test.benchtime 5s -test.count 1 -test.cpu 1 -test.benchmem -test.cpuprofile /tmp/p25a-profile-20260824-artifacts/profiles/BenchmarkName.pprof
```

P25a substituted each name from the primary trio. It ran the benchmarks
sequentially. It did not run the 20-seed publication protocol. One profile
ran for each lane. This receipt does not establish a formal noise floor.

### P25a fresh profile results

The primary trio is a historical generated straight-LR control. It does not
prove generalized LR (GLR) behavior or a real-GLR performance improvement.

| Benchmark | Time | Bytes per operation (B/op) | Allocations per operation (allocs/op) | Maximum resident set size (RSS) |
|---|---:|---:|---:|---:|
| `BenchmarkGoParseFullDFA` | 9.763 ms | 12,453 | 4 | 49,872 KiB |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 882.5 ns | 0 | 0 | 50,668 KiB |
| `BenchmarkGoParseIncrementalNoEditDFA` | 3.212 ns | 0 | 0 | 49,868 KiB |

Full parsing had distributed cost. `runtime.duffcopy` used 9.99% flat CPU.
`Core.reduceOutputsClassifiedIntoActive` used 4.70% flat and 23.35%
cumulative CPU. The generic scheduler dispatch used 3.96% flat and 55.65%
cumulative CPU. Materialization used 2.35% flat and 10.43% cumulative CPU.
No single full-parse helper formed a safe local candidate.

The single-byte edit profile supplied the strongest target region:

- `runtime.memmove` used 21.78% flat CPU.
- `Tree.Edit` used 8.08% flat and 11.37% cumulative CPU.
- `tryTokenInvariantLeafEdit` used 3.56% flat and 47.95% cumulative CPU.
- `reuseTreeWithNewSource` used 2.05% flat and 13.15% cumulative CPU.
- `newTreeWithUniqueArenas` used 6.16% flat and 11.10% cumulative CPU.

The no-edit lane is an identity-reuse control. `Parser.ParseIncremental` used
32.98% flat and 63.03% cumulative CPU. Compatibility checks and root reuse
accounted for most remaining samples. It is not a useful local target.

### P25a candidate and proof boundary

P25a considered the `Tree.Edit` single-byte replacement and reuse path. The
path marks the affected tree path. When final child references exist, it uses
the general delta path. That path shifts child entries, stack entries, pending
parents, and points.

The token-invariant fast path checks the language, edit range, leaf range,
error and missing flags, scanner checkpoints, token symbol, and token span.
The reuse path retains arenas, clears dirty paths, and carries forest, compact,
reuse, result, and compatibility state into the new tree.

Any candidate in this region must prove all of these properties:

- Preserve linear and forked graph-structured stack (GSS) paths.
- Preserve pending-parent ownership, child references, and materialization.
- Preserve raw shape, hashes, precedence, and result selection.
- Preserve field identifiers, field sources, hidden flattening, and field order.
- Preserve fragility flags and fragile non-leaf reuse rejection.
- Preserve locked-C recovery, retries, missing tokens, and fallback decisions.
- Prove incremental trees equal fresh parses after clean and malformed edits.

The fresh profiles identify a region, but they do not identify one helper whose
edit can satisfy this proof boundary. The primary trio also lacks forked GLR
coverage. P25a therefore found no defensible candidate.

### P25a artifacts and decision

The isolated profile worktree was `/tmp/gts-p25a-profile-20260824`. The
benchmark binary SHA-256 is
`8f05f5c338ab95050dcb91c8944dd9c51995765ca12a727359b1de564fd40ac2`.

The profile artifacts are in
`/tmp/p25a-profile-20260824-artifacts/`. The `profiles` directory contains
these files:

- `profiles/BenchmarkGoParseFullDFA.pprof`
- `profiles/BenchmarkGoParseIncrementalSingleByteEditDFA.pprof`
- `profiles/BenchmarkGoParseIncrementalNoEditDFA.pprof`

The `top` directory contains the matching top reports. The same directory
contains benchmark output and `/usr/bin/time -v` logs.

P25a changed no parser or test code. It ran no Docker correctness or parity
test because no candidate survived the profile proof boundary. It ran no
20-seed, 40-process publication. The accepted decision is **KEEP LIVE / NO-GO**.

### P25a reopening condition

Reopen P25a only when a fresh quiet-host profile names one bounded helper
above a measured noise floor. Use one CPU, `GOMAXPROCS=1`, and no concurrent
CPU work. Make one local candidate edit only after the profile identifies it.
Prove linear and forked GSS, pending parents, raw shape, fields, fragility,
recovery, and incremental equality on production and compact routes. Run
focused Docker correctness and parity before any performance publication.
Then run the exact 20-seed, 40-process protocol with alternating order,
`GOMAXPROCS=1`, count one, 750 milliseconds, benchmark memory reporting, the
primary trio, and per-process RSS.

## 2026-08-22 P24i final performance blocker receipt

Status: **KEEP LIVE / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`603f64155651888d46937e6b5df461873283b9a1`.

P24i reviewed the bounded performance candidates from P24a through P24h. It
found no safe bounded candidate for the next performance slot. The remaining
targets cross graph-structured stack (GSS) ownership, raw-shape selection,
field metadata, fragility, recovery, or incremental reuse boundaries.

P24i made no production or test-code change. It ran no 20-seed campaign. It
records a blocker and keeps the performance arm live.

### P24i focused baseline control

The focused Docker baseline passed at
`/tmp/gotreesitter-p24i-investigation/harness_out/docker/20260822T234737Z`.
The artifact runs these controls:

- `TestParseGoIncrementalRepeatedSingleByteEdit`
- `TestParserIncrementalArithmeticEditMatchesFreshParse`
- `TestRawShapeElisionDifferentialRecoveryFromSingleStackPrefix`

The Docker command exited zero. The metadata records no timeout and no
out-of-memory event. This artifact is a baseline control. It is not a
candidate result.

The artifact used commit `86bc8ae4641a884d94bf28dd2bf8e309367252ee`, the first
parent of the P24i experiment main. Later main commits added documentation and
one parity test. They did not change parser production code.

No P24i publication directory exists. No 20-seed, 40-process, or benchstat
receipt exists for P24i.

### P24i remaining profile attribution

The prior receipts leave these named profile ranges:

| Target | Observed profile range | Receipt |
|---|---:|---|
| `rawShapeComputeContentHash` | 47.25% cumulative CPU | P24e |
| `rawShapeChild.entry` | 24.71% cumulative CPU | P24e |
| `stackEntryNodeParseState` | 24.55% flat, 24.67% cumulative CPU | P24f |
| `prepareParseStacksForIteration` | 7.54% to 15.58% cumulative CPU | P24g |
| `applyReduceActionDispatch` | 20.00% to 28.64% cumulative CPU | P24h |
| `condenseWithOutcomeAtomic` | 9.2% cumulative CPU | Attribution board |

P24a through P24d supplied no fresh quiet-host profile range for a new
decision. These ranges do not prove that a safe local edit exists. They show
where a future profile must start.

### P24i proof obligations

Require every future bounded candidate to prove all of these properties:

- Prove equal results on linear and forked graph-structured stack (GSS) paths.
- Preserve child order, byte ranges, parser states, and selected lineage.
- Preserve pending-parent ownership, child references, flags, and materialization.
- Preserve raw-shape availability, hashes, precedence, and result selection.
- Preserve field identifiers, field sources, hidden flattening, and field order.
- Preserve fragility flags and fragile non-leaf reuse rejection.
- Preserve locked-C recovery, retries, missing tokens, and fallback decisions.
- Prove incremental trees equal fresh parses after clean and malformed edits.

The proof must cover both production and compact routes. It must cover clean,
forked, pending-parent, raw-shape, field, fragile, recovery, and incremental
fixtures. A focused unit result cannot replace these route proofs.

### P24i decision

**KEEP LIVE / NO-GO.** Do not ship a candidate or change production code. Do
not start a performance publication without a new candidate and a fresh
quiet-host profile.

### P24i reopening condition

Reopen P24i only when a fresh quiet-host profile identifies one bounded target.
The profile must use one CPU, `GOMAXPROCS=1`, and no concurrent CPU work. It
must name a function, show a range above the measured noise floor, and support
one local candidate edit.

After the proof obligations pass, run focused Docker correctness and parity.
Then run the full 20-seed, 40-process campaign with per-process RSS. Reject
the candidate when any route, proof, parity result, or required lane fails.

## 2026-08-22 P24h transient-parent dispatch predicate receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`.

P24h tested one bounded predicate in `applyReduceActionDispatch`. The
candidate changed exactly one file:

- `parser_reduce.go`

The candidate diff SHA-256 is
`2a57aef7c0eac33b802439506ac01b92e29d3286ee5a78fc482f26d4b7630c8c`.

### P24h profiler attribution and proof boundary

The P19 attribution packet is at `/tmp/gts-p19-evidence`. It used one central
processing unit (CPU), `GOMAXPROCS=1`, and stable benchmark settings. The warm
profiles place `applyReduceActionDispatch` at 20.00% to 28.64% cumulative CPU
across the early-newline, same-line, and token scenarios.

The candidate evaluates the transient-parent predicate once per dispatch. The
old code evaluated the same predicate at both dispatch sites. Production code
sets `p.reduceScratch.transientParents` once at parse setup and does not replace
the pointer during this dispatch. The candidate therefore selects the same
transient-parent or ordinary reduction path. It changes no reduction inputs,
children, fields, parser state, or tree output.

### P24h focused correctness and parity evidence

The focused Docker tests passed:

- `/tmp/gotreesitter-p24h-investigation.xb2SWG/harness_out/docker/20260822T231855Z`

The tests covered transient-parent reduction state and temporary entry reuse.
The run exited zero without an out-of-memory event or timeout.

The candidate Go real-corpus run reported 25 of 25 no-error cases, 24 of 25
S-expression matches, and 24 of 25 deep matches. It reported the known Go
range divergence. Its artifact is
`/tmp/gotreesitter-p24h-investigation.xb2SWG/harness_out/docker/20260822T231910Z-diag-go_lang`.

The candidate Go-to-C run reported 20 of 20 no-error cases, 7 of 20 tree
matches, and 15 known divergences. Its artifact is
`/tmp/gotreesitter-p24h-investigation.xb2SWG/harness_out/grammargen_cparity/20260822_161943/container.log`.
These counts match the baseline run from the same base commit.

### P24h randomized publication

The publication is at `/tmp/gts-p24h-publication.X0MEre`.

It used 20 shuffle seeds and 40 isolated processes. Odd seeds ran the baseline
first. Even seeds ran the candidate first. Each process used:

- `GOMAXPROCS=1`
- `-count=1`
- `-benchtime=750ms`
- `-benchmem`
- one process for each seed and variant

The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

The runner metadata is at
`/tmp/gts-p24h-publication.X0MEre/runner-metadata.txt`. All 40 processes
exited zero. The logs contain no failure, out-of-memory, timeout, panic, or
contamination marker.

For each seed, compute the geometric mean (geomean) across the three lanes.
For each order, take the geomean of the ten baseline seed geomeans and the ten
candidate seed geomeans. Report the candidate-to-baseline ratio minus one.

The paired analysis uses equal seeds. It uses the candidate-minus-baseline
relative change for each pair and a Student-t 95% interval over 20 pairs.

### P24h performance result

The benchstat result is at
`/tmp/gts-p24h-publication.X0MEre/benchstat.txt`.

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 7.036ms | 7.061ms | +0.36% | 0.620 |
| Single-byte edit | 768.5ns | 771.2ns | +0.35% | 0.172 |
| No edit | 2.537ns | 2.534ns | −0.12% | 0.909 |
| Geomean | 2.394µs | 2.399µs | +0.20% | — |

The paired intervals are in
`/tmp/gts-p24h-publication.X0MEre/paired-stats.txt`.

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +0.124143% | [−0.532215%, +0.780502%] |
| Single-byte edit | +0.275082% | [−0.286294%, +0.836458%] |
| No edit | +0.122733% | [−0.406059%, +0.651525%] |
| Geomean | +0.170048% | [−0.231804%, +0.571901%] |

The order geomean formula reports +0.214736% for baseline-first and +0.118408%
for candidate-first. Both order results include zero in their paired
intervals.

Raw FullDFA bytes per operation (B/op) averaged 54,885.9 for the baseline and
55,190.4 for the candidate. The candidate-to-baseline ratio was +0.554696%.
Benchstat rounded both medians to 53.67 KiB. Both variants used four
allocations per operation (allocs/op) in every sample. Both incremental lanes
used zero B/op and zero allocs/op.

An auxiliary exact-setting run measured maximum resident set size (RSS) at
602,400 KB for the baseline and 601,440 KB for the candidate. The run used one
process for each variant. It does not replace the 40-process campaign.

The candidate has no significant timing improvement. Its raw FullDFA byte
mean increased. The accepted result is **REJECT / NO-GO**. No code shipped.

### P24h reopening condition

Reopen P24h only with a different profiler-backed target. The new candidate
must show a clear primary-trio opportunity and improve a measured target.
Require focused correctness, Go and C parity, the same 20-seed campaign, and
per-process RSS before publication.

## 2026-08-22 P24g conditional recovery-scratch reset receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`.

P24g tested one bounded branch in
`prepareParseStacksForIteration`. The candidate changed exactly one file:

- `parser.go`

The candidate diff SHA-256 is
`f1bf93c698c41d06c77a123ee6ab255e7caae61f6406d7171cbeee4355bdaa01`.

### P24g profiler attribution and proof boundary

The P19 attribution packet is at `/tmp/gts-p19-evidence`. It used one central
processing unit (CPU), `GOMAXPROCS=1`, and stable benchmark settings. The warm
early-newline profile attributes 7.54% cumulative CPU to
`prepareParseStacksForIteration`. The same-line profile attributes 15.58%.

The candidate guards `resetCRecoveryMergeScratch`. The guard checks the same
four flags that the reset function clears. It calls the reset when any flag is
true. It skips four false stores when every flag is already false.

The single-stack recovery test proves that the early return leaves all four
flags false. The multi-stack path synchronizes the flags before merge work.
The candidate does not change parser state or merge decisions at either
boundary.

### P24g focused correctness and parity evidence

The focused Docker tests passed:

- `/tmp/gotreesitter-p24g-investigation/harness_out/docker/20260822T230243Z`
- `/tmp/gotreesitter-p24g-investigation/harness_out/docker/20260822T230305Z`

The tests covered the single-stack recovery reset, the one-cycle cost walk,
and recovery retry state reset. Both runs exited zero without an out-of-memory
event or timeout.

The candidate Go real-corpus run reported 25 of 25 no-error cases, 24 of 25
S-expression matches, and 24 of 25 deep matches. It reported one known range
divergence. The baseline reported the same result.

- Candidate: `/tmp/gotreesitter-p24g-investigation/harness_out/docker/20260822T230325Z-diag-go_lang`
- Baseline: `/tmp/gotreesitter-p24g-baseline.1Yk6zZ/harness_out/docker/20260822T231339Z-diag-go_lang`

The candidate and baseline Go-to-C runs each reported 20 of 20 no-error cases,
7 of 20 tree matches, and 15 known divergences. Their artifacts are:

- Candidate: `/tmp/gotreesitter-p24g-investigation/harness_out/grammargen_cparity/20260822_160406/container.log`
- Baseline: `/tmp/gotreesitter-p24g-baseline.1Yk6zZ/harness_out/grammargen_cparity/20260822_161349/container.log`

### P24g randomized publication

The publication is at `/tmp/gts-p24g-publication.5AaoUM`.

It used 20 shuffle seeds and 40 isolated processes. Odd seeds ran the baseline
first. Even seeds ran the candidate first. Each process used:

- `GOMAXPROCS=1`
- `-count=1`
- `-benchtime=750ms`
- `-benchmem`
- one process for each seed and variant

The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

The runner metadata is at
`/tmp/gts-p24g-publication.5AaoUM/runner-metadata.txt`. All 40 processes
exited zero. The logs contain no failure, out-of-memory, timeout, panic, or
contamination marker.

For each seed, compute the geometric mean (geomean) across the three lanes.
For each order, take the geomean of the ten baseline seed geomeans and the ten
candidate seed geomeans. Report the candidate-to-baseline ratio minus one.

The paired analysis uses equal seeds. It uses the candidate-minus-baseline
relative change for each pair and a Student-t 95% interval over 20 pairs.

### P24g performance result

The benchstat result is at
`/tmp/gts-p24g-publication.5AaoUM/benchstat.txt`.

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 7.062ms | 7.129ms | +0.94% | 0.004 |
| Single-byte edit | 768.9ns | 767.3ns | no significant change | 0.606 |
| No edit | 2.554ns | 2.623ns | +2.70% | <0.001 |
| Geomean | 2.402µs | 2.430µs | +1.14% | — |

The paired intervals are in
`/tmp/gts-p24g-publication.5AaoUM/paired-stats.txt`.

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +1.115890% | [+0.228433%, +2.003347%] |
| Single-byte edit | +0.662063% | [−0.789332%, +2.113459%] |
| No edit | +2.685050% | [+2.438164%, +2.931937%] |
| Geomean | +1.476205% | [+0.700682%, +2.251728%] |

The order split reports the following paired means:

| Benchmark | Baseline first | Candidate first |
|---|---:|---:|
| Full parse | +0.701348% | +1.530433% |
| No edit | +2.869775% | +2.500325% |
| Geomean | +1.200969% | +1.751441% |

The order geomean formula reports +1.200411% for baseline-first and +1.727965%
for candidate-first.

Benchstat reports a median of 55,399 bytes per operation (B/op) and four
allocations per operation (allocs/op) for FullDFA in both variants. Raw means
show 55,404.3 B/op for the baseline and 56,386.3 B/op for the candidate. Their
ratio is +1.772335%. The paired relative mean is +1.782949%. Candidate seed 2
reported 69,776 B/op and 5 allocs/op. Every other candidate FullDFA sample
reported 4 allocs/op.
Both incremental lanes reported zero B/op and zero allocs/op.

An auxiliary exact-setting run measured maximum resident set size (RSS) at
619,520 KB for the baseline and 589,920 KB for the candidate. The run was a
single process for each variant. It does not replace the 40-process campaign.

The candidate has significant FullDFA and no-edit regressions. The accepted
result is **REJECT / NO-GO**. No code shipped.

### P24g reopening condition

Reopen P24g only with a different profiler-backed hot path. The new candidate
must show material cost in the primary trio and must not repeat P24a through
P24f. Require focused correctness, Go and C parity, the same 20-seed campaign,
and per-process RSS before publication.

## 2026-08-22 P24f stack-entry state receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`f42b88ac9014537d20d3edd76e2c9caa4330a579`.

The candidate experiment used base commit
`48e844d9a73863cb92367c4db10f02bc5c09375d`.

P24f tested one bounded hot-path change in stack-entry state extraction. The
candidate rewrote `stackEntryNodeParseState` in `no_tree_node.go`. It checks
the payload pointer once, then selects the payload type from the stack-entry
kind. It does not change parser state, reuse selection, tree construction, or
recovery decisions.

The candidate changed exactly one file:

- `no_tree_node.go`

The candidate diff SHA-256 is
`3f930b8109c3a2be5b8aabfa4161c71148aa9a15277e2407dab32976b8c8e7d1`.

### P24f profiler attribution

The P19 packet at `/tmp/gts-p19-evidence` used one central processing unit
(CPU), `GOMAXPROCS=1`, and 20 samples per scenario. Its warm early-newline
profile used 25 samples. It named `stackEntryNodeParseState` at 24.55% flat
CPU and 24.67% cumulative CPU.

P24f targets state extraction inside the reparse and rebuild component. It does
not repeat the rejected P24a through P24e candidates. Its proof boundary is
the existing stack-entry representation. Each supported payload type stores
`parseState` at the field used by the old accessor path. A nil payload and an
unknown kind still return zero.

### P24f correctness evidence

The baseline focused Docker gate passed at
`/tmp/gts-p24f-artifacts/20260822T222719Z-p24f-baseline-stack-entry-state`.
The candidate focused Docker gate passed at
`/tmp/gts-p24f-artifacts/20260822T222738Z-p24f-candidate-stack-entry-state`.
The tests covered raw-shape behavior, raw-shape reclamation, cache eviction,
and incremental equality with a fresh parse. Both runs exited zero without an
out-of-memory event or timeout.

The Go and C parity Docker gate passed at
`/tmp/gts-p24f-artifacts/20260822T222826Z-p24f-go-c-parity`. It passed the Go
generalized LR (GLR) canary and three multiline `Tree.Edit` coordinate cases.

### P24f publication protocol

The publication used 20 shuffle seeds and 40 isolated processes. Odd seeds ran
the baseline first. Even seeds ran the candidate first. Each process used
`GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.

The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

The raw publication files are in `/tmp/gts-p24f-publication`. The aggregate
files are:

- `/tmp/gts-p24f-publication/baseline-all.txt`
- `/tmp/gts-p24f-publication/candidate-all.txt`
- `/tmp/gts-p24f-publication/benchstat.txt`
- `/tmp/gts-p24f-publication/paired-stats.txt`

All 40 processes emitted three valid benchmark rows. They reported no failure,
out-of-memory event, timeout, panic, or contamination marker.

### P24f performance result

The benchstat result compares the candidate with the baseline. The geometric
mean (geomean) combines the three primary benchmark lanes.

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 7.181ms | 7.189ms | +0.11% | 0.862 |
| Single-byte edit | 770.4ns | 770.5ns | +0.01% | 0.616 |
| No edit | 2.549ns | 2.549ns | +0.00% | 0.909 |
| Geomean | 2.416µs | 2.417µs | +0.04% | — |

For each seed, compute the geomean across the trio. For each order, take the
geometric mean of the ten baseline seed geomeans and the ten candidate seed
geomeans. Report the candidate-to-baseline ratio minus one. The order formula
reports −0.167% when baseline ran first and −0.317% when candidate ran first.

The paired analysis uses equal seeds. For each pair, compute the
candidate-minus-baseline relative change. Use all 20 pairs and a Student-t
95% interval.

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | −0.359% | [−1.405%, +0.688%] |
| Single-byte edit | −0.262% | [−0.804%, +0.280%] |
| No edit | −0.070% | [−0.518%, +0.378%] |
| Geomean | −0.236% | [−0.772%, +0.300%] |

All incremental lanes used zero bytes and zero allocations. Full parsing used
54.97 KiB and four allocations per operation in both aggregate variants.

An auxiliary exact-setting run measured maximum resident set size at 591,840
KiB for the baseline and 608,000 KiB for the candidate. The publication runner
did not record resident set size for each process.

The candidate has no significant lane improvement. The paired geomean interval
crosses zero, and the auxiliary candidate run used more resident memory. The
accepted result is **REJECT / NO-GO**. No code shipped.

### P24f reopening condition

Reopen P24f only if a new profile confirms this helper as a material cost. Use
a real incremental or generalized LR workload. Require focused correctness and
parity first. Then repeat the 20-seed, 40-process protocol and require an
interval that excludes zero without a resident-memory regression.

## 2026-08-22 P24e raw-shape hashing receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`56b97e092d0fb034bddb9e65cc617ebb933cc718`.

P24e tested one bounded hot-path change in raw-shape result selection. The
candidate read the packed child entry directly inside
`rawShapeComputeContentHash`. It did not restore parser state that hashing
does not inspect. This change differs from the rejected P24a through P24d
scheduler and memo candidates.

The experiment used base commit
`098620ad5e39d7b69b258239d1059a1e33bea892` and changed exactly one file:

- `raw_shape.go`

The candidate diff SHA-256 is
`e92471c690c7d6fed16d593044a449b0557b8b78380718173731a70a62842996`.

### P24e profiler attribution

The P19 attribution packet is at `/tmp/gts-p19-evidence`. It used Go 1.25.12,
`GOMAXPROCS=1`, and 20 samples per scenario. The warm early-newline profile
used 25 samples. It named `rawShapeComputeContentHash` at 47.25% cumulative
CPU. It named
`rawShapeChild.entry` at 24.71% cumulative CPU.

The same packet attributes incremental parse time as follows:

| Component | Observed share |
|---|---:|
| `Tree.Edit` | 0.16%–0.95% |
| Reuse cursor | 0.03%–0.98% |
| Reuse selection | 0.00%–2.79% |
| Reparse and rebuild | 96.98%–99.96% |

The candidate therefore targets a named reparse and result-selection cost.
It does not change `Tree.Edit`, reuse admission, parser decisions, or tree
shape rules. The proof boundary is the existing packed-entry representation:
the hash reads node payload fields and never reads the packed parser state.

### P24e correctness evidence

The focused Docker unit gate passed at
`/tmp/gotreesitter-p24e-candidate/harness_out/docker/20260822T214450Z`.
It covered raw-shape differential tests, cache eviction, hidden-shape
comparison, and the C result-selection control. The run exited zero without
an out-of-memory event or timeout.

The Go real-corpus smoke gate passed 25 of 25 cases for no-error,
S-expression, and deep parity. Its artifact is
`/tmp/gotreesitter-p24e-candidate/harness_out/docker/20260822T213032Z-diag-go_lang`.

The Go grammargen-versus-C gate passed five of five cases with two tree
matches and three known baseline divergences. Its artifact is
`/tmp/gotreesitter-p24e-candidate/harness_out/grammargen_cparity/20260822_143121-p24e-raw-shape-go`.

### P24e accepted publication

The accepted publication used 20 shuffle seeds and 40 isolated processes.
Odd seeds ran baseline first. Even seeds ran candidate first. Each process
used `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.

The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

This trio is a historical generated straight-LR control. It does not prove a
real generalized LR (GLR) improvement.

The accepted aggregate files are:

- Baseline: `/tmp/gts-p24e-publication/alternating-baseline-20.txt`
- Candidate: `/tmp/gts-p24e-publication/alternating-candidate-20.txt`

All 40 accepted processes emitted three valid benchmark rows. They reported
no failure, out-of-memory event, timeout, panic, or contamination marker.

The first seed-1 baseline construction was interrupted and is excluded from
the aggregate. Its quarantined file is
`/tmp/gts-p24e-publication/alternating-baseline/seed1.txt`. Clean seed-1
retries are recorded in the `seed1-retry.txt` files for both variants.

### P24e performance result

The combined benchstat result reports candidate versus baseline. The geometric
mean (geomean) combines the three primary benchmark lanes.

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 7.101ms | 7.128ms | +0.38% | 0.529 |
| Single-byte edit | 771.5ns | 772.6ns | +0.14% | 0.516 |
| No edit | 2.541ns | 2.539ns | −0.08% | 0.605 |
| Geomean | 2.405µs | 2.409µs | +0.16% | — |

For each seed, compute the geomean across the trio. Each order then compares
ten seed geomeans. For each order, take the geometric mean of the ten baseline
seed geomeans and the ten candidate seed geomeans. Report the
candidate-to-baseline ratio minus one. The order split reports changes of +0.278% when baseline ran
first and −0.228% when candidate ran first. The direction reverses.

The paired analysis uses equal seeds and the clean seed-1 retries. For each
pair, compute the candidate-minus-baseline relative change. Use all 20 pairs
and a Student-t 95% interval. The result is:

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +0.056% | [−0.634%, +0.746%] |
| Single-byte edit | +0.300% | [−0.707%, +1.307%] |
| No edit | −0.240% | [−0.937%, +0.457%] |
| Geomean | +0.034% | [−0.639%, +0.708%] |

Full parsing used 54.10 KiB and four allocations per operation in both
variants. Both incremental lanes used zero bytes and zero allocations.

An auxiliary exact-setting run measured maximum resident set size at 603,200
KB for baseline and 609,120 KB for candidate. The publication runner did not
record per-process resident set size.

The candidate has no significant lane improvement. The accepted result is
**REJECT / NO-GO**. No code shipped.

### P24e reopening condition

Reopen P24e only with a dedicated real generalized LR (GLR) benchmark that
exercises raw-shape result selection. Use the same 20-seed, 40-process protocol
and record per-process resident set size. Require correctness parity before
performance.

## 2026-08-22 P24d authenticated single-head shift receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

P24d tested one authenticated proof that a generic scheduler cell is an
ordinary shift on an exact single-head frontier. The proof lets the apply
path skip one repeated action-shape check. The experiment used base commit
`ed0568d9a822b1c83e7bb7e69b0e0f4b8ad529cf` and changed exactly:

- `parsercore_phase0_driver.go`
- `parsercore_phase0_generic_conflict_internal_test.go`

The combined candidate diff SHA-256 is
`5181912ec91f0bc1ecd504d06db488d3dd9145fdc5d3d5486300756661a4fc59`.
The per-file patch SHA-256 values are:

- Driver: `038ad2cf59286f93ecfdd36928cf0157022d6eb6fd414e01d14b95c4622f2fa6`
- Tests: `cfd9646fd8cf257718b0fb83eb8e735527335827a6bcf8a6429c7adef0aa9d1e`

The candidate source hashes are:

- Driver: `f3e279ea9fc3fece75b5e1b2b8ee760c4d8a26e2a5adb55af1d0c3e805f21a33`
- Tests: `a97472e2fbad888198fea03a01b0d59ce576b6572a221b9873d0b89982de85f7`

### Proof and validation

The proof requires one header, one elected state, header index zero, no
relexed symbol, matching checkpoint and parser state, and an unaccepted,
unshifted, unpaused, non-S3 header. It also requires an allowed selection,
an ordinary shift row, a valid action ordinal, and a non-extra shift action.

The focused tests cover the positive proof, a second header, a shifted header,
a checkpoint mismatch, a parser-state mismatch, a relexed symbol, an extra
shift, and a forged reduce action. Existing descriptor and unsupported-arm
checks remain active. Core owner, epoch, boundary, transaction, and rollback
validation remains active after the proof skips the repeated shape check.

The final focused Docker gate passed at
`/tmp/gts-p24d-docker-candidate-final/20260822T204456Z-p24d-unit-driver-final`.
The baseline control passed at
`/tmp/gts-p24d-docker-baseline/20260822T203123Z-p24d-unit-driver`.
Both runs used the local Go 1.25 image, one CPU, `GOMAXPROCS=1`, and `-p=1`.
Both runs exited zero without an out-of-memory event or timeout.

### Real-corpus and C parity

The Go real-corpus smoke gate passed five no-error, five S-expression, and
five deep checks for both variants. The reports are:

- Candidate: `/tmp/gts-p24d-real-candidate-final/diag_go_lang.log`
- Baseline: `/tmp/gts-p24d-real-baseline/diag_go_lang.log`

The grammargen-versus-C gate passed with five eligible cases and five no-error
cases for both variants. Both sides reported two tree-parity cases and three
known divergences. The candidate artifact is
`/tmp/gotreesitter-p24d-candidate/harness_out/grammargen_cparity/20260822_135328-p24d-candidate-final`.
The baseline artifact is
`/tmp/gotreesitter-p24d-baseline/harness_out/grammargen_cparity/20260822_133343-p24d-baseline`.

The known divergences are the blob root child-count difference and the two
statement-list end-byte differences. The candidate reproduces the baseline.

### Accepted publication

The final publication root is
`/tmp/gts-p24d-publication-final`.
The runner metadata is
`/tmp/gts-p24d-publication-final/runner-metadata.txt`.
It records 20 seeds, 40 isolated processes, and one process per seed.
Odd seeds run baseline first. Even seeds run candidate first. Each process
uses `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.
The primary trio is:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

All 40 status rows have exit code zero. The raw output and log files contain
no failure, out-of-memory, timeout, panic, or contamination marker.
The runner records Go 1.25.1 on host `Chi` and candidate primary-trio maximum
resident set size of 596,800 KB. The comparable baseline run used 605,120 KB.

### Performance result

The combined benchstat result reports candidate versus baseline:

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 6.635ms | 6.682ms | +0.71% | 0.096 |
| Single-byte edit | 732.2ns | 736.5ns | +0.59% | 0.256 |
| No edit | 2.344ns | 2.365ns | +0.87% | 0.031 |
| Geomean | 2.250µs | 2.266µs | +0.72% | — |

The order split reports these geomeans:

| Order | Baseline | Candidate | Result |
|---|---:|---:|---:|
| Baseline first | 2.252µs | 2.260µs | +0.37% |
| Candidate first | 2.247µs | 2.268µs | +0.93% |

The paired relative timing analysis reports candidate minus baseline:

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +4.088% | [-0.049%, +8.225%] |
| Single-byte edit | +3.426% | [-1.301%, +8.154%] |
| No edit | +3.243% | [+0.063%, +6.423%] |
| Geomean | +3.496% | [+0.179%, +6.813%] |

Full parsing used 50.50 KiB/op for baseline and 50.88 KiB/op for candidate.
The byte comparison was +0.74% with p=0.024. Incremental lanes used 0 B/op.
Median full-parse allocations were 4 on both sides with p=0.231. The
candidate used 5 allocations on seeds 11, 12, and 13; baseline used 4 on
every seed. Incremental lanes used 0 allocations on every seed.

The candidate is slower in the combined geomean and both order splits. The
no-edit timing and full-parse byte regressions are statistically significant.
The result is **REJECT / NO-GO**. No code shipped.

## 2026-08-22 P24c exact-single-pop helper-inlining receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

The original direct-append path already shipped in ancestor
`b9106e78635244b59dc9c9b75aa4863b89c99630` (`b9106e78`). P24c tested only
helper inlining inside `condenseWithOutcomeAtomic`. The audited candidate used
base commit `c46c9597005faf57ea810e3268e2cf55239da7e4` and changed exactly:

- `internal/parsercorephase0/core.go`
- `internal/parsercorephase0/condense_direct_append_test.go`

The audit recorded these three patch SHA-256 values:

- Core patch: `2dbcf54b185214e609d0cade7bf53ea6408150e6b0857ac104f24c3fee759438`
- Focused test patch: `418f60f9d20948d161adfe510fc068cc622ca4660f95aeef7476cde6ff05b0af`
- Combined two-file patch: `48a5ab605ff49373017aa5a5a31b423692619a3cecf71d9be9fd9f46a37190c3`

The focused tests directly cover direct publication shape, ordering, rollback,
and arena-cap behavior.

### Correctness evidence

The focused Docker unit gate passed at
`/tmp/gts-p24c-correctness/20260822T185424Z-p24c-direct-candidate-unit`.
The final parser-core package gate passed at
`/tmp/gts-p24c-correctness/20260822T185528Z-p24c-direct-candidate-package-r2`.

Exclude the stale pre-final package attempt at
`/tmp/gts-p24c-correctness/20260822T185452Z-p24c-direct-candidate-package`.
Its initial inline composite literal failed
`TestCondenseWithOutcomeAtomicProvenanceRatchet`. The final candidate fixed
that provenance-ratchet issue before the accepted package gate.

The generic driver failures reproduced on baseline. The candidate artifact is
`/tmp/gts-p24c-correctness/20260822T185553Z-p24c-direct-candidate-driver`.
The baseline artifact is
`/tmp/gts-p24c-correctness/20260822T185626Z-p24c-direct-baseline-driver`.
The shared failures are baseline-reproduced fixture and counter failures.
Do not attribute them to helper inlining.

The JSON real-corpus parity gate passed 5/5 on baseline and 5/5 on candidate.
Each side reported five no-error, five S-expression, and five deep checks.
The candidate report is
`/tmp/gts-p24c-reports/candidate-json/diag_json.log`.
The baseline report is
`/tmp/gts-p24c-reports/baseline-json/diag_json.log`.
The CSS grammargen-versus-C parity gate passed 5/5 on baseline and 5/5 on
candidate, with zero tree divergences on each side. The candidate artifact is
`/tmp/gts-p24c-single-pop-candidate/harness_out/grammargen_cparity/20260822_115824-p24c-direct-candidate-cgo`.
The baseline artifact is
`/tmp/gts-p24c-single-pop-baseline/harness_out/grammargen_cparity/20260822_115856-p24c-direct-baseline-cgo`.

### Accepted publication

The accepted publication root is
`/tmp/gts-p24c-publication-p24c-20260822-single-pop`.
It used 20 shuffle seeds, 40 isolated processes, and 120 benchmark rows.
Odd seeds ran baseline first. Even seeds ran candidate first. Each process
used `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.
The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

All 40 processes passed. The publication recorded no failures, out-of-memory
events, timeouts, or contamination. Raw headers prove the benchmark expression,
build tag, shuffle seed, and 750ms benchtime. The publication directory does
not itself encode GOMAXPROCS, count, benchmem, or isolated-process settings.
Those settings rely on external runner records.

### Performance result

The combined benchstat result reports candidate minus baseline:

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 6.512ms | 6.624ms | +1.72% | 0.925 |
| Single-byte edit | 725.2ns | 727.9ns | +0.37% | 0.805 |
| No edit | 2.347ns | 2.420ns | +3.11% | 0.129 |
| Geomean | 2.230µs | 2.268µs | +1.72% | — |

The order split reports these geomeans:

| Order | Baseline | Candidate | Result |
|---|---:|---:|---:|
| Baseline first | 2.246µs | 2.304µs | +2.60% |
| Candidate first | 2.229µs | 2.257µs | +1.26% |

The paired timing analysis reports candidate minus baseline:

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +0.4707% | [-1.6206%, +2.5620%] |
| Single-byte edit | +0.4155% | [-1.6074%, +2.4385%] |
| No edit | +0.3220% | [-2.8235%, +3.4676%] |
| Geomean | +0.3163% | [-1.1488%, +1.7814%] |

Full parsing used 50.13 KiB/op for baseline and 51.07 KiB/op for candidate.
The byte comparison had p=0.370. Incremental lanes used 0 B/op in both
variants. Full parsing used 4 allocations per operation in both variants.
Incremental lanes used 0 allocations per operation in both variants. Every
allocation comparison had p=1.000.

The geomean rose in the combined publication. Each paired interval includes
zero. The order split did not show a credible directional win. The accepted
result is **REJECT / NO-GO**. No code shipped.

## P24b owner-plus-lineage transaction receipt

P24b tested one scheduler transaction for owner and lineage state.
The hypothesis was that one transaction would reduce repeated scheduler work.
The candidate combined owner and lineage writes for converged, reference-bearing heads.
It preserved owner-only calls, rollback, journaling, and token validation.
It kept production defaults and profile grants unchanged.

The base commit was
a01f8037319c9f8f0ea12ae3c96523112656ce22.
The candidate changed these three files:

- internal/parsercorephase0/scheduler_owned.go
- parsercore_phase0_driver.go
- internal/parsercorephase0/scheduler_owned_owner_lineage_test.go

The full candidate diff had SHA-256
c064480db960885558cce4eff8b22679a3dffed996d908f938fa5e480f92dee.

Focused correctness gates passed. The corrected unit gate used
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T184141Z-p24b-owner-lineage-unit-corrected-20260822`.
Its Docker command selected these eight tests:

- `TestRecordHeadOwnerAndLineageOwnedCommitsAllState`
- `TestRecordHeadOwnerAndLineageOwnedRollsBackAllState`
- `TestRecordHeadOwnerAndLineageOwnedRejectsInvalidOwnerLineage`
- `TestRecordHeadOwnerAndLineageOwnedRejectsOwnerConflict`
- `TestRecordHeadOwnerAndLineageOwnedRejectsStaleToken`
- `TestRecordHeadOwnerAndLineageOwnedRollsBackReferenceFailure`
- `TestRecordHeadOwnerAndLineageOwnedHonorsSetDirtyFalse`
- `TestHeadOwnerRollsBackWithSchedulerTransaction`

The earlier unit receipt
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T180959Z-p24b-owner-lineage-unit-final2`
is superseded and excluded from this unit claim. Its anchored expression
matched no combined owner-lineage test, so it did not cover the new combined
operation.
The driver gate used
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T180751Z-p24b-owner-lineage-driver-final`.
The real-corpus smoke gate used
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T180806Z-diag-go_lang`.
The candidate C parity gate used
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/grammargen_cparity/20260822_110830-p24b-owner-lineage-cgo-final`.
The baseline C parity gate used
`/tmp/gts-p24b-baseline-20260822/harness_out/grammargen_cparity/20260822_110533-p24b-owner-lineage-baseline-cgo`.
The candidate and baseline C receipts showed the same three pre-existing
grammar divergences. They showed two EndByte differences and one root
child-count difference.

### Publication protocol

The accepted publication is
`/tmp/gts-p24b-publication-20260822T181351Z`.
It used the three required benchmarks:
`BenchmarkGoParseFullDFA`,
`BenchmarkGoParseIncrementalSingleByteEditDFA`, and
`BenchmarkGoParseIncrementalNoEditDFA`.
It used 20 seeds and 40 isolated processes.
Each process used `GOMAXPROCS=1`, `-count=1`,
`-benchtime=750ms`, `-benchmem`, and `-test.shuffle=<seed>`.
Odd seeds ran the baseline first. Even seeds ran the candidate first.
The run produced 120 accepted benchmark rows.
The run reported no out-of-memory event, timeout, or contamination.

The first attempt was quarantined at
`/tmp/gts-p24b-publication-20260822T181256Z-QUARANTINED`.
Its validator expected a hyphenated benchmark suffix.
Go emitted whitespace after the benchmark name.
The attempt contained only three seed-one baseline rows.
The analysis excluded that attempt from every aggregate and decision.

### Benchstat result

The analysis used
`/tmp/gts-p24b-analysis-20260822T182318Z/benchstat.txt`.
The table reports candidate minus baseline.

| Benchmark | Baseline median | Candidate median | Change | p-value |
|---|---:|---:|---:|---:|
| Full parse | 6.616ms | 6.585ms | -0.47% | 0.512 |
| Single-byte edit | 721.1ns | 722.0ns | +0.12% | 0.644 |
| No edit | 2.350ns | 2.347ns | -0.13% | 0.784 |
| Geomean | 2.238µs | 2.234µs | -0.17% | — |

Memory stayed neutral.
Full parse used 50.50 KiB and 4 allocations per operation in both variants.
Incremental benchmarks used zero bytes and zero allocations in both variants.
The full-parse allocation comparison had p=1.000.
The full-parse byte comparison had p=0.757.

### Paired and order results

The paired analysis is in
`/tmp/gts-p24b-analysis-20260822T182318Z/paired-summary.txt`.
Each interval uses 20 matched seeds and reports candidate minus baseline.

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +0.389% | [-1.376%, +2.154%] |
| Single-byte edit | +1.779% | [-0.198%, +3.756%] |
| No edit | +0.452% | [-0.555%, +1.459%] |

The order split is in
`/tmp/gts-p24b-analysis-20260822T182318Z/order-split.txt`.

| Order | Full parse | Single-byte edit | No edit |
|---|---:|---:|---:|
| Baseline first | +0.643% | +1.417% | +0.282% |
| Candidate first | +0.135% | +2.140% | +0.622% |

### Decision

**REJECT / NO-GO.**

The keep rule requires a target improvement without a correctness regression.
No benchmark lane improved with statistical significance.
The paired mean rose in all three lanes.
The memory result stayed neutral.
The candidate code does not ship.

## Why this document exists

Three prior estimates put the compact scheduler's share of full-parse CPU at
77-79%, at approximately 50%, and at 78-82%. Each estimate used a different,
undocumented attribution boundary: one drew the scheduler boundary wide
(covering shift, reduce, and election together), one drew it narrow, and one
used a different profiling method. The three numbers do not contradict each
other once you know their boundaries, but no one had written the boundaries
down, so the campaign could not tell whether a proposed perf lever targeted
77% of parse CPU or 50% of it.

Campaign v7 tranche C0 (`spec.campaign.v7`, `hypha://m31labs/gotreesitter`)
blocks new perf-lever bets until one receipt exists with a named boundary for
every component, a documented noise floor, and a cost-per-event model. This
document is that receipt's home. The attached tool
(`cgo_harness/attribution`) regenerates the receipt from one command.

## Scope

The receipt covers the compact parser core's fresh, non-incremental full
parse: the default route for eligible full parses (`admission_switch.go`),
and the diagnostic runner path (`parserCoreFreshFullRunner`) that exercises
the same scheduler and materialization code directly. Campaign v7 tranche B9
retired the compact admission switch's 64 KiB source-length eligibility
decline; the shipped route now attempts every eligible full parse regardless
of size, with a scheduler stop-control poll (tranche B8) bounding a large or
pathological input instead.

It does not cover: incremental reparse, error recovery, retry passes, the
production (non-compact) GLR engine, or any language other than the four
pinned Go canonical fixtures.

## The compact lane, by file

| Area | File(s) | Key entry points |
|---|---|---|
| Scheduler driver (root package) | `parsercore_phase0_driver.go` | `run`, `dispatchPass`, `dispatchPassActive`, `elect`, `canonicalize` |
| Core structures (`internal/parsercorephase0`) | `core.go`, `scheduler_owned.go`, `boundary_index.go`, `checkpoint_interner.go` | `ApplySchedulerAtomic`, `condenseWithOutcomeAtomic`, `popPaths`, `Derivations`, `MaterializationOrder` |
| Lexing | `parser_dfa_token_source.go`, `lexer.go` | `(*dfaTokenSource).Next` |
| Runner glue | `parsercore_phase0_fresh_full_runner.go` | `executeSchedulerOpen`, `parse`, `materialize` |
| Shipped-route fidelity tail | `admission_switch.go`, `admission_switch_candidate.go` | `normalizeReturnedTreeForParse`, `resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree` |

The four canonical fixtures are SHA-256-pinned in
`internal/benchfixtures/testdata` and authenticated the same way
`cgo_harness/pure_c/run_canonical_go_full_parse.sh` and
`diagnosticParserCoreCanonicalAdmissions` authenticate them:

| Fixture | Bytes |
|---|---:|
| `rewrite.go` | 5,116 |
| `query_compile.go` | 20,168 |
| `language.go` | 41,387 |
| `grammargen/lr.go` | 235,626 |

Before tranche B9, `grammargen/lr.go` sat above the compact admission
switch's 64 KiB source-length eligibility floor and never took the shipped
`Parser.Parse` route (diagnostic-lane only; see the first receipt below,
which predates B9). B9 retired that floor, so all four fixtures now run
through both lanes; see the B9 addendum.

## Two lanes, one methodology

Every fixture profiles under one or both of two lanes. Both lanes time an
identical, already-committed lifecycle; the tool never adds new parser calls.

- **Diagnostic lane.** `runner.parse`, shallow completeness validation, and
  `Tree.Release` — the exact timed region of
  `BenchmarkParserCoreFreshFullCanonical` (build tag
  `gts_parsercorephase0`). Runs for all four fixtures.
- **Shipped route.** `Parser.Parse` and `Tree.Release` — the exact timed
  region of `BenchmarkDiagnosticParserCoreWarmProductionParseQueryCompile`,
  generalized to all four fixtures (tranche B9 retired the 64 KiB eligibility
  floor that used to keep `grammargen/lr.go` diagnostic-lane only). Every
  sample is verified, by the admission-candidate routed/fallback counters, to
  have actually taken the compact route rather than falling back to
  production.

The tool labels every row with its lane so a reader never mixes shipped-route
public-API overhead into the diagnostic lane's scheduler-only numbers, or
vice versa.

## The attribution tree

Nine components, mutually exclusive and collectively exhaustive. Every
CPU-profile sample lands in exactly one.

1. **scheduler-dispatch** — the scheduler loop, per-token boundary
   classification, and the mechanical application of shift and accept
   actions. Owning functions: `run`, `dispatchPass`, `dispatchPassActive`,
   `applyGenericShifts(Owned)`, `applyGenericExtraShifts(Owned)`,
   `applyGenericAccept`, `finish`, `Core.Shift*`, `Core.ClassifyBoundary`,
   `Core.Actions`, scheduler construction and seeding, and the accepted-tail
   cleanliness check (`requireParserCoreFreshFullAcceptance`,
   `parserCoreFreshFullAcceptedTailIsClean` — see the naming-collision note
   below).
2. **elections** — choosing among competing alternatives: which GSS heads
   advance each round (frontier election), and which action wins at an
   ambiguous cell (GLR conflict resolution / dynamic precedence). Owning
   functions: `elect`, `executeDiagnosticParserCoreGenericConflictDetailed`,
   `applyGenericConflict(Owned)`, `diagnosticParserCoreConflictPolicyOrdinal`,
   `diagnosticParserCoreRepetitionFoldOrdinal`.
3. **reductions-and-pops** — applying a reduce action: enumerating GSS pop
   paths back to a production's children, building the parent subtree, and
   merging (condensing) the result into the derivation graph. Owning
   functions: `applyGenericReduction(Owned)`, `Core.Reduce`,
   `Core.ReduceOutputs*`, `Core.popPaths`, `Core.popSingleLinkPath`,
   `Core.factorExactPredecessor`, `Core.mergePredecessorsBounded`,
   `Core.insertLinkBounded`, `Core.appendNode`, `Core.appendSubtree`,
   `Core.walkCleanPrefixRanks`.
4. **canonicalization** — post-step header-lineage deduplication: merging
   headers that reconverge to the same `(state, byte-offset)` boundary after
   any dispatch action. Owning functions: `canonicalize`,
   `canonicalizeDiagnosticParserCoreHeaders`,
   `diagnosticParserCoreCanonicalScratch.canonicalize(Linear|Mapped)`,
   `mergeDiagnosticParserCoreCleanPathLineage`,
   `persistHeaderLineageOwned`, `RecordReductionLineageOwned`,
   `RecordHeadLineageOwned`.
5. **lexing** — producing the next token from source bytes. Owning function:
   `(*dfaTokenSource).Next`, and every function defined in
   `parser_dfa_token_source.go` or `lexer.go` (a whole-file rule, since both
   files exist for exactly this one job).
6. **materialization** — turning the accepted derivation into the returned
   public `*Tree`: selecting the canonical derivation when more than one
   exists, building nodes, attaching parent/sibling links, replaying parser
   states, and finalizing the root span. Owning functions:
   `completeAcceptance`, `selectCompactAcceptanceDerivation`,
   `Core.Derivations`, `materializeDiagnosticParserCoreAcceptedTree`,
   `materializeDiagnosticParserCoreAcceptedSelection`,
   `finalizeDiagnosticParserCoreAcceptedRootSpan`, `Core.MaterializationOrder`,
   `Core.VisitMaterializationPostorder`, `(*Parser).replayCompactDerivation`.
7. **compat-tail** — the shipped-route-only public-API fidelity tail that
   runs after the compact tree exists, so every `Parser.Parse` return matches
   production's API surface exactly. Not reachable from the diagnostic
   runner path. Owning functions: `normalizeReturnedTreeForParse`,
   `resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree`,
   `tryCompactFullParseRoute`, `attemptAdmissionCandidateFullParse`,
   `admissionCandidateFullParseEligible` (the whole of `admission_switch.go`
   and `admission_switch_candidate.go` — a whole-file rule).
8. **recovery** — the compact core's native locked-C recovery mechanism
   (error cost, missing-token insertion, retry, and recovery election over
   compact subtrees; campaign v7 tranche B3). Added in tranche B3 stage S1,
   before any recovery engine code exists, per gate G5 ("classifier
   first... so recovery cost can never hide in `other`"). It owns no
   functions yet — compact has no recovery implementation (B3 stages S2-S5
   add error-cost, absorb/condense-resume, election, and missing-token code
   class by class) — so it reads 0.0% on every clean canonical fixture
   today, the same way `compat-tail`'s conditional work reads 0.0% below.
   The whole-file rule for `internal/parsercorephase0/recovery_cost.go`
   (stage S2's planned file) is forward-declared now so landing that file
   does not also require touching this table.
9. **other** — every sample the walk cannot attribute to a named component:
   Go runtime, garbage collection, and goroutine-scheduling frames with no
   gotreesitter-domain ancestor, plus any genuinely unclassified function.
   The tool separately reports the largest unclassified, non-runtime
   functions it saw; a nonzero entry there is a signal this table has a gap,
   not a license to guess.

### A naming collision to avoid

`parserCoreFreshFullAcceptedTailIsClean` checks that no real source byte
follows the accepted head (only parser padding may remain) before the
scheduler declares acceptance. This is **not** the compat-tail component. It
is a scheduler-dispatch acceptance check that runs on both lanes. The English
word "tail" names two unrelated things here: an accepted-frontier byte-range
check (scheduler-dispatch), and a post-parse public-API fidelity pass
(compat-tail, shipped route only). The classifier and this document both use
the full function name, never the bare word "tail", to keep them apart.

## The disambiguation rule

Every named function belongs to at most one component's owned set, chosen by
its primary job, not by which caller happens to invoke it. A handful of
low-level primitives exist purely to serve more than one component
(transaction bookkeeping, checkpoint identity, and pure counters). Those
primitives are explicitly marked shared and are **not** classified directly.

A CPU-profile sample is a full call stack, leaf (currently executing frame)
to root. Attribution walks the stack from the leaf toward the root and
assigns the sample's entire value to the first frame the table recognizes.
A shared primitive is transparent to this walk: the sample "sees through" it
to the nearer ancestor frame, which is always the specific call site that
decided to use the primitive. A sample with no recognized frame anywhere on
its stack is `other`.

This single rule resolves every case where the same function serves two
components:

- **`condense` / `condenseWithOutcomeAtomic`** run from both `Core.Shift*`
  (scheduler-dispatch, appending a shifted GSS node) and reduction-output
  application (reductions-and-pops, appending a reduced parent node). Marked
  shared; the walk finds whichever caller reached it.
- **`ApplySchedulerAtomic` / `RunSchedulerOwned`** wrap the shift, reduce,
  conflict, and extra-shift appliers uniformly (four call sites in
  `parsercore_phase0_driver.go`). Marked shared for the same reason.
- **Checkpoint interning and external-scanner-state capture**
  (`diagnosticParserCoreInternCheckpoint`,
  `captureExternalScannerStateInto`) run both nested inside a live call to
  `dfaTokenSource.Next` (lexing) and directly from `elect` for the
  election's checkpoint-continuity proof (elections). Marked shared; the
  walk finds `Next` when the capture is nested inside token production, and
  finds `elect` when it is not, without any special case.
- **`canonicalize`** is called from inside every applier (accept, shift,
  reduce, conflict, extra-shift) as a post-step. It is deliberately **not**
  marked shared: its job (header-lineage deduplication) is the same
  regardless of caller, so it is classified directly as canonicalization.
  The walk finds `canonicalize`'s own frame before it would reach the
  calling applier's frame, so canonicalization is correctly separated from
  whichever action triggered it.

## The tool

`cgo_harness/attribution` is a small, dependency-free Go program (it lives in
the `cgo_harness` module and imports nothing beyond the standard library,
including its own minimal pprof-profile reader, so this measurement
infrastructure does not widen either module's dependency footprint). It:

1. Extracts and SHA-256-verifies the four canonical fixtures.
2. Builds two test binaries: one tagged `gts_parsercorephase0` (capture and
   noise floor), one tagged `gts_parsercorephase0,gts_workcount` (event
   counts). Building happens once, before any timed measurement, so
   compilation never contends with the host for CPU during a timed sample.
3. Runs `TestParserCoreAttributionCapture` (added in
   `parsercore_phase0_attribution_capture_internal_test.go`, inert unless
   `GTS_ATTRIBUTION_OUT` is set) to collect one gzip'd pprof CPU profile per
   lane per fixture. Every one-time warm pass, digest check, and GC/arena
   drain happens outside `pprof.StartCPUProfile`/`StopCPUProfile`, so the
   profile is not diluted by non-representative setup cost.
4. Runs the existing `TestParserCoreWorkCountChild` (already committed,
   `work_count_parsercore_child_internal_test.go`) once per fixture for
   event counts.
5. Runs the noise floor (see below).
6. Decodes every profile with a minimal, read-only pprof protobuf reader,
   classifies every sample by the rule above, and emits `receipt.json` and
   `receipt.md`.

Reproduce the whole receipt with one command from the repository root:

```sh
(cd cgo_harness && go run ./attribution -repo .. -duration 800ms -noise-samples 10 -noise-benchtime 750ms)
```

Add `-out <dir>` to choose the output directory (default:
`harness_out/perf_attribution/<UTC timestamp>`, already git-ignored). Add
`-core <cpu>` to pin every subprocess with `taskset`; the published receipt
below did not pin a core (see host class, next section).

## Noise floor

**Protocol.** Using the pinned local proxy benchmark
`BenchmarkParserCoreFreshFullCanonical`, the tool builds one test binary and
invokes that identical binary twice per pair, labeled A and B, interleaved
A,B,A,B,... for n pairs (n >= 10). Each invocation runs with `GOMAXPROCS=1`
and times all four fixtures in one process. For each fixture and pair `i`,
`delta_i = |A_i - B_i|`. The noise floor is the 95th percentile (linear
interpolation over the sorted deltas) of `delta_i`, reported both in
nanoseconds and as a percentage of the fixture's median ns/op across the
combined A and B samples.

**Host class.** Shared WSL2 development host, background load uncontrolled.
This is the local-development floor, not the quiet-host or enclave floor
that `BENCH.md`'s sealed epoch uses. Treat every ns/op number in this
document as wall-clock on a busy, shared machine (other agents were active
on the same host during this run), not as a publication-grade performance
claim. The relative component-share percentages are far less sensitive to
this noise than the absolute ns/op and ns-per-event numbers are, because Go's
CPU profiler samples on-CPU (SIGPROF/`ITIMER_PROF`) time, not wall time: host
contention stretches wall-clock ns/op without changing which component a
given on-CPU sample belongs to.

## Cost-per-event join

For each fixture, the tool divides the diagnostic lane's measured wall
ns/op by the `gts_workcount` build's authenticated event counts for that
same fixture and lifecycle (`TestParserCoreWorkCountChild`, schema
`gts-work-count-parsercore-child/v3`): shifts, reductions, emitted pop
paths, emitted pop payloads, canonicalizations, elections, and action
lookups.

This is an **average attribution, not a causal model**. Dividing total wall
time by an event count says nothing about which event class actually causes
the most CPU per occurrence; it says how the average work per fixture spread
across event classes on this run, on this host. Two events of the same class
can cost very different amounts (a pop path over a clean linear GSS chain is
far cheaper than one over a branching, ambiguous chain), and the classes are
not independent (an election's checkpoint-continuity proof is not free, but
it has no "count" of its own in this join).

## Results — first receipt

Generated 2026-08-01T11:12:22Z. Host: shared WSL2 development host,
background load uncontrolled. Other agents were active on the same host
throughout this session (22 registered at session start). Git commit
`f91e9c8cc7d2c8830c973cfa28b658c70dd0d0ba`. Full machine-readable receipt:
`docs/perf-attribution-receipt.json`. Regenerate both (and the larger raw
pprof profiles, which are not committed) with the one command above.

Component shares are computed to sum to exactly 100% of attributed samples
by construction (`other` absorbs the remainder); the displayed 1-decimal
rows are checked to sum to 100% within +/-0.2 percentage points of rounding
tolerance. Every row below is within that tolerance.

### Attribution shares

| lane | fixture | wall ns/op | coverage % | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | other | sum % |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.674ms | 105.0 | 22.9 | 9.5 | 33.3 | 4.8 | 14.3 | 13.3 | 0.0 | 1.9 | 100.0 |
| diagnostic lane | query_compile | 12.340ms | 104.8 | 20.8 | 4.7 | 30.2 | 8.5 | 19.8 | 16.0 | 0.0 | 0.0 | 100.0 |
| diagnostic lane | language | 13.195ms | 105.7 | 22.6 | 7.5 | 33.0 | 7.5 | 13.2 | 16.0 | 0.0 | 0.0 | 99.8 |
| diagnostic lane | grammargen_lr | 126.019ms | 106.1 | 25.2 | 7.5 | 22.4 | 5.6 | 19.6 | 17.8 | 0.0 | 1.9 | 100.0 |
| shipped route | rewrite | 2.707ms | 104.8 | 21.9 | 7.6 | 28.6 | 9.5 | 19.0 | 11.4 | 0.0 | 1.9 | 99.9 |
| shipped route | query_compile | 13.026ms | 104.7 | 30.5 | 3.8 | 21.9 | 1.0 | 20.0 | 22.9 | 0.0 | 0.0 | 100.1 |
| shipped route | language | 11.753ms | 105.9 | 25.2 | 4.7 | 30.8 | 6.5 | 15.9 | 16.8 | 0.0 | 0.0 | 99.9 |

`coverage %` is profiled CPU time divided by measured wall time on this
single-core-bound (`GOMAXPROCS=1`) run. It runs 105-106% here, not 100%,
because the Go runtime's background workers (GC assist, sweep) can add a
small amount of concurrent on-CPU time beyond the profiled goroutine's own
wall-clock window. A coverage figure far below 100% would mean too few
samples landed to trust the percentages; every row here is comfortably
above that concern.

`grammargen_lr` has no shipped-route row: it is above the 64 KiB floor, so
`Parser.Parse` never routes it through the compact candidate. `compat-tail`
measures zero on every shipped-route row: for these four fixtures every
parse is clean (no error nodes, no C-recovery-swallowed error to resolve),
so the shipped-route fidelity tail's conditional work never fires. This is
a genuine finding, not a classifier gap: the compat-tail functions exist for
correctness parity on error/recovery paths this receipt does not exercise,
not for cost on the clean path.

`other` stays at 0.0-1.9% on every row. Where it is nonzero, the largest
non-runtime contributors are `(*nodeArena).reset` (arena-pool bookkeeping
inside `Tree.Release`), `hiddenTreeHasFieldIDs`, and
`maxRetainedNodeCapacityForClass` — each under 1% of one profile's samples.
None of them changes the qualitative picture; they are recorded as a known
small residual rather than folded into a component that would overstate its
weight.

### B3 stage S1 addendum: the `recovery` component

The table above predates the `recovery` component (added to
`cgo_harness/attribution/classify.go` in campaign v7 tranche B3 stage S1, per
gate G5) and so has no `recovery` column. A local reproduction of the same
command on stage S1's branch, after the classifier change and before any
recovery engine code, confirmed `recovery` reads exactly 0.0% on every lane
and fixture — diagnostic lane (`rewrite`, `query_compile`, `language`,
`grammargen_lr`) and shipped route (`rewrite`, `query_compile`, `language`)
alike — matching `compat-tail`'s 0.0% pattern above. This is expected: the
component owns no functions yet (B3 stages S2-S5 add error-cost,
absorb/condense-resume, election, and missing-token code class by class), so
no sample can land there. This local run is a verification check, not a new
sealed or C0-authoritative epoch; it does not replace the receipt above or
`docs/perf-attribution-receipt.json`. The next full regeneration (any future
tranche that re-seals this board) will show `recovery` alongside the other
eight components by construction, with no further classifier change needed.

### B3 stage S2 addendum: the inert error-cost model

Stage S2 added `internal/parsercorephase0/recovery_cost.go`: compact
equivalents of `cNodeErrorCostLang`, `cSymbolVisibleLang`, and
`cVersionStatus`, callable on demand but wired into nothing (no call site
exists outside the file's own tests; a compile-time AST ratchet,
`TestRecoveryCostNoOutsideCallSitesRatchet`, enforces this). A local
reproduction of the documented command on stage S2's branch, with the file
present, again confirmed `recovery` reads exactly 0.0% on every lane and
fixture — diagnostic lane and shipped route alike. Every other component
share stayed within this host's published noise floor of the stage-S1-era
receipt above; the local noise floor measured on this run (8.2%-11.9% of
median ns/op) is consistent with that same shared, uncontrolled-host
variance, not a new finding. As with the S1 addendum, this is a
verification check, not a new sealed epoch.

### 2026-08-02 addendum: the provenance-predicate flip receipt

The compact scheduler's no-action head drop now decides with the
alternative-set containment predicate by default. The scalar rank-walk
(`markCleanProductionRank`) still exists in the source. It now runs only
under a diagnostic flag (`GTS_B4B_SHADOW_CENSUS`), not on the default
path. Alternative-set recording used to run only under that same flag. It
now runs unconditionally, because the containment predicate needs a
populated set to decide.

This addendum re-runs the documented command. It adds one targeted
profile of the changed functions. The goal is one question: how much CPU
does the changed provenance machinery cost today, and where should the
next performance lever land.

#### Component shares (regenerated receipt)

Generated 2026-08-02T04:38:36Z. Host: shared WSL2 development host,
background load uncontrolled. Git commit
`193c44b340538b78770da68e4246abd253c81710`. This run's noise floor (see
below) sits far above the first receipt's 7.4%-10.8%. Read every share
below as a rough reading, not an exact one. This run's raw JSON output was
not committed, matching the existing `harness_out/` git-ignore rule;
rerun the documented command to reproduce it.

| lane | fixture | wall ns/op | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | recovery | other |
|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.389ms | 21.6 | 8.0 | 22.7 | 10.2 | 21.6 | 14.8 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | query_compile | 11.201ms | 26.1 | 8.0 | 20.5 | 8.0 | 22.7 | 13.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | language | 12.511ms | 14.8 | 6.8 | 25.0 | 15.9 | 15.9 | 21.6 | 0.0 | 0.0 | 0.0 |
| diagnostic lane | grammargen_lr | 125.275ms | 22.9 | 7.3 | 20.8 | 15.6 | 15.6 | 16.7 | 0.0 | 0.0 | 1.0 |
| shipped route | rewrite | 2.452ms | 21.6 | 6.8 | 31.8 | 11.4 | 10.2 | 15.9 | 1.1 | 0.0 | 1.1 |
| shipped route | query_compile | 11.824ms | 21.6 | 4.5 | 25.0 | 10.2 | 18.2 | 20.5 | 0.0 | 0.0 | 0.0 |
| shipped route | language | 12.202ms | 10.2 | 4.5 | 33.0 | 12.5 | 20.5 | 19.3 | 0.0 | 0.0 | 0.0 |

`recovery` reads 0.0% on every row. This matches every prior receipt: the
inert cost model from stage S2 still has no call site. The `compat-tail`
reading for shipped-route `rewrite` (1.1%) is one profiled sample out of
roughly ninety. Treat it as noise, not a new conditional-cost finding,
given this run's wide noise floor.

Scheduler-dispatch, elections, reductions-and-pops, and canonicalization
together still cover 60.2%-71.6% of wall time across every fixture above.
No lever in the ranked list below reaches that scale on its own; see item
5.

#### Noise floor (this run)

| fixture | pairs | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|---:|
| `grammargen_lr` | 10 | 122.945ms | 68.439ms | 55.7% |
| `rewrite` | 10 | 2.448ms | 1.038ms | 42.4% |
| `query_compile` | 10 | 11.862ms | 4.626ms | 39.0% |
| `language` | 10 | 12.123ms | 3.725ms | 30.7% |

Other automated workers ran concurrent CPU-bound jobs on this host
throughout this run. Read every ns/op number above as directional, not
exact. Component shares still rest on firmer ground than raw ns/op does:
the CPU profiler samples on-CPU time, so host contention stretches
wall-clock duration without moving which component a sample belongs to
(see "Noise floor," above).

#### Targeted profile: the provenance-predicate family

The component table above answers where compact wall time goes, across
nine wide boundaries. It does not answer a narrower question: how much of
the reductions-and-pops and canonicalization shares still belongs to the
mechanism this flip changed. This section profiles that mechanism by
function name.

Method: `go test -tags gts_parsercorephase0 -bench
BenchmarkParserCoreFreshFullCanonical/query_compile -benchtime 6s
-cpuprofile`, `GOMAXPROCS=1`, pinned to one core with `taskset -c 0`,
`GTS_B4B_SHADOW_CENSUS` unset (the shipped default). 630 iterations, 9.55
seconds of profiled samples. `query_compile` matches the fixture the
original regression bisect used, so the reading below is
fixture-consistent with that bisect. The bisect's own numbers predate
this tool's interleaved-capture methodology, so treat the comparison as
fixture-consistent, not protocol-identical.

| function | flat % | cum % |
|---|---:|---:|
| `condenseWithOutcomeAtomic` | 5.1% | 9.2% |
| `persistHeaderLineageOwned` | 1.5% | 7.3% |
| `RecordHeadLineageOwned` | 0.2% | 4.0% |
| `RecordHeadOwnerOwned` | 0.2% | 1.9% |
| `recordNodeLineageSet` | 0.5% | 2.1% |
| `recordNodeLineage` | 0.5% | 0.7% |
| `RecordReductionLineageOwned` | 0.0% | 0.4% |
| `alternativeSetInsert` | 0.1% | 0.3% |
| `alternativeSetUnion` | 0.0% | 0.3% |
| `alternativeSetSortedContainsAll` | 0.0% | 0.0% |
| the v2 containment predicate (`dropGenericNoActionHeads`'s decider) | 0.0% | 0.1% |
| `enterLiveCondenseCandidates` | 0.3% | 0.3% |
| `clearLiveCondenseCandidates` | 0.1% | 0.1% |
| `condenseNodeIsLive` | 0.2% | 0.2% |
| `runLiveCondenseCandidates` (old closure-wrapper form) | 0.0% | 0.0% |
| `markCleanProductionRank` | 0.0% | 0.0% |
| `walkCleanPrefixRanks` | 0.0% | 0.0% |

`condenseWithOutcomeAtomic` is a shared GSS merge primitive that predates
the regression; it is not itself a provenance-family member, and it is
the single largest function this profile named. `condenseNodeIsLive` runs
nested inside it. `RecordHeadLineageOwned` and `RecordHeadOwnerOwned` run
nested inside `persistHeaderLineageOwned`, as two separate transactions,
not one inside the other; `recordNodeLineageSet`, `recordNodeLineage`,
`alternativeSetInsert`, and `alternativeSetUnion` run nested inside
`RecordHeadLineageOwned` in turn. None of these nested cum numbers add on
top of their parent row.

Pre/post comparison, rolled up to the mechanism level (query_compile;
pre-flip numbers from the regression bisect, at the commit that
introduced them):

| mechanism | pre-flip cumulative CPU | today's cumulative CPU |
|---|---|---|
| condense live-scoping and liveness check (`enterLiveCondenseCandidates`, `clearLiveCondenseCandidates`, `condenseNodeIsLive`) | 26%-30% (as the old closure-wrapper form, `runLiveCondenseCandidates`) | 0.6% |
| lineage-and-set recording (`persistHeaderLineageOwned` and everything nested below it, plus `RecordReductionLineageOwned`) | canonicalization's whole component share grew by +3.8 percentage points (3.0% to 6.8%) when this family was introduced, per the bisect's own before/after reading; the family did not yet have a separate cum number | 7.7% |
| scalar rank-walk (`markCleanProductionRank`) | part of the two rows above; not separated at bisect time | 0.0% (retired) |
| v2 containment predicate (decider) | did not exist | 0.1% |

This run's own component table (above) reads canonicalization at 8.0% for
`query_compile`, diagnostic lane. Lineage-and-set recording alone (7.7%)
now accounts for nearly all of that share. Canonicalization today is
almost entirely lineage-and-set recording, not the header-merge mechanics
the component predates.

Two readings follow.

The retirement holds. `markCleanProductionRank` and `walkCleanPrefixRanks`
carry zero samples on the default path. `runLiveCondenseCandidates`'s
closure-wrapper form also carries zero samples: an earlier, separate fix
replaced its call sites with direct calls before this predicate flip
landed. Both retirements match their design intent.

The residual moved; it did not vanish. Live-condense scoping fell from
26%-30% to 0.6%, but lineage-and-set recording rose from a gated-off 0%
to 7.7%, because recording is now unconditional. The family's live cost
today concentrates almost entirely in `persistHeaderLineageOwned` and its
two callees (7.3% cumulative), not in `alternativeSetInsert` or
`alternativeSetUnion` (0.3% cumulative, combined). An overflow fast path
already made the set math cheap: on this corpus, most sets saturate their
cap early, and both functions return immediately once a set carries the
overflow flag. The remaining cost sits above that math, in the per-header
transaction wrapper that runs whether or not the math underneath does
anything.

#### Ranked residual levers

Five candidate levers, ranked by addressable share:

1. **Further live-condense scoping reduction.** Addressable share: under
   1% cumulative CPU (0.6% combined, `query_compile`, the three-function
   row above). An earlier de-indirection fix already captured this
   lever's addressable share. The remainder is the scoping check's own
   cost, not avoidable call overhead. Not recommended: too little room
   left.
2. **Stage-3 scalar-field deletion.** `nodeLineageRecord` carries 36 bytes
   today, measured directly (`unsafe.Sizeof`). Two fields marked
   "transition only" in their own source comment, a 2-byte lineage ID and
   a 1-byte rank, could shrink it by roughly 11%. Both fields sit inside a
   component that already costs under 8% of wall time, so their removal
   has a ceiling near or under 1% of wall time. Worth doing for code
   hygiene; not a performance lever on its own.
3. **Recording-overhead reduction.** The set math is now cheap. The
   transaction wrapper around it, for each header, is not. Today,
   `persistHeaderLineageOwned` opens one scheduler-owned transaction for
   owner persistence on every header (`RecordHeadOwnerOwned`, 1.9%
   cumulative), and a second, separate transaction for lineage-and-set
   persistence on headers with a converged reduction split
   (`RecordHeadLineageOwned`, 4.0% cumulative). One source comment already
   states the merge goal for the second transaction's own two halves:
   halve the per-header token-validation cost. Folding the owner write
   into the same transaction is not free: the owner write fires on every
   header, and the lineage write fires only on converged ones, so a naive
   merge would change which headers get an owner write. A follow-up needs
   to preserve that distinction inside one transaction. Addressable
   share, before subtracting the work that must stay: up to 7.3%
   cumulative CPU on `query_compile`.
4. **The remaining L1 deterministic-frontier follow-ons.** An earlier
   lever selection modeled five single-head-frontier fast-path sub-items.
   A shipped change captured two of them: a checkpoint-digest skip inside
   election, and a canonical-group-key skip inside single-head
   canonicalization (roughly 4.3 modeled percentage points of an 8.1
   modeled total). Two more never landed: a direct-append path for
   `condenseWithOutcomeAtomic` on an exact single pop path, and a
   validation skip inside the dispatch loop's per-cell checks. Modeled
   addressable share for the two open sub-items: roughly 3.8% combined,
   against a `condenseWithOutcomeAtomic` share this run measured at 9.2%
   cumulative, the single largest function this profile named. The model
   predates this run by several days of intervening change and needs
   re-validation before a bet. The target function is still large and
   still un-optimized.
5. **The bytecode-scheduler structural bet.** Scheduler-dispatch,
   elections, reductions-and-pops, and canonicalization together still
   cover 60.2%-71.6% of wall time on every fixture in the component table
   above. None of the other four levers reach that scale; this is a
   separate, larger, and longer-horizon bet.

Recommendation: pursue item 4 next, ahead of item 3. Item 4 targets one
named function this run confirmed still costs 9.2% cumulative CPU. It
already has a capture model and two concrete, previously scoped
sub-items. Item 3 is real, but smaller: an overhead layer on an
already-cheap operation, entangled with a correctness distinction a
follow-up must preserve. It needs its own follow-up profile, to separate
wrapper cost from the work that must stay, before a precise size. Land
items 1 and 2 opportunistically. Neither earns a dedicated lever slot.
Item 5 stays the standing structural option, for whenever items 3 and 4
stop paying.

### Noise floor (interleaved A/A, identical binary, 12 pairs per fixture)

| fixture | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|
| `grammargen_lr` | 123.099ms | 9.069ms | 7.4% |
| `language` | 12.020ms | 1.299ms | 10.8% |
| `query_compile` | 11.772ms | 1.165ms | 9.9% |
| `rewrite` | 2.401ms | 0.256ms | 10.7% |

Read this as: on this shared, uncontrolled host, two runs of the literal
same binary can disagree by roughly 7-11% before you have measured any real
difference. Any comparison this receipt states below that floor is not a
finding; it is noise.

### Cost per event (diagnostic lane; average, not causal)

| fixture | wall ns/op | ns/shift | ns/reduction | ns/pop path | ns/pop payload | ns/canonicalization | ns/election | ns/action lookup |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `rewrite` | 2.674ms | 1984 | 1778 | 1625 | 894 | 1093 | 2581 | 753 |
| `query_compile` | 12.340ms | 1846 | 1643 | 1522 | 838 | 998 | 2407 | 708 |
| `language` | 13.195ms | 2026 | 1745 | 1575 | 834 | 1071 | 2609 | 707 |
| `grammargen_lr` | 126.019ms | 1906 | 1651 | 1506 | 830 | 1033 | 2525 | 684 |

The top-3 most expensive classes, by ns/event and consistent across all four
fixtures, are elections (2,407-2,609 ns), shifts (1,846-2,026 ns), and
reductions (1,643-1,778 ns). Elections costing more per event than shifts or
reductions is consistent with the shares table: on the diagnostic lane,
elections is the smallest of the four scheduler-family CPU shares
(4.7-9.5%) but fires the fewest times per fixture (1,036 on `rewrite`
versus 3,551 action lookups), so each election carries relatively more
weight. Full JSON: `docs/perf-attribution-receipt.json`.

### Which historical figure this receipt speaks to

Sum the four diagnostic-lane scheduler-family components
(scheduler-dispatch + elections + reductions-and-pops + canonicalization)
without lexing: 70.5% (`rewrite`), 64.2% (`query_compile`), 70.8%
(`language`), 60.7% (`grammargen_lr`) — average 66.5%. That number, on its
own, matches none of the three historical figures well.

Add lexing to that sum (scheduler-dispatch + elections +
reductions-and-pops + canonicalization + lexing): 84.8%, 84.0%, 84.0%,
80.3% — average 83.3%. This closely reproduces the **77-79% and 78-82%**
figures. Read literally, this means those two historical estimates most
likely counted token production as part of "scheduler cost per event" —
a defensible reading, since a shift cannot happen without first lexing the
token it shifts, but one this document did not previously state.

Sum only scheduler-dispatch and reductions-and-pops, the two components that
directly apply a table-driven action (excluding election/conflict overhead,
canonicalization bookkeeping, and lexing): 56.2%, 50.9%, 55.7%, 47.7% —
average 52.6%. This closely reproduces the **approximately-50%** figure. That
estimate most likely measured only the two dominant mechanical appliers and
implicitly folded election, canonicalization, and lexing cost into
"overhead" it did not separately name.

**Call: this receipt retires all three historical figures as decision
inputs**, not because any of them was numerically wrong, but because none of
them stated a boundary a later reader could reproduce or falsify. Each is a
reasonable reading of a different implicit boundary that this receipt now
makes explicit. From this tranche forward, campaign v7 should cite the
eight-component table above — with its stated boundary and disambiguation
rule — instead of any of the three legacy percentages.

### B9 addendum: the wall-removal receipt

Campaign v7 tranche B9 removed the compact admission switch's 64 KiB
source-length eligibility decline (`admission_switch.go`). The shipped route
now attempts every eligible full parse regardless of size; a scheduler
stop-control poll (tranche B8) bounds a large or pathological input instead
of a size wall. This addendum re-runs the documented command with
`grammargen_lr` added to the shipped-route lane (diagnostic-lane only in
every receipt above) and reports what changed. Per doctrine 10 (board
honesty), every row below still names its lane; nothing here merges a
diagnostic-lane number into a shipped-route claim or vice versa.

Generated 2026-08-02T11:02:14Z. Host: shared WSL2 development host,
background load uncontrolled -- the same local-development floor as every
prior receipt in this document, not a quiet-host or enclave measurement.
Git identity: `6fab988d30d42d931d42625eaa0cfa167d4877ba` (the capture-time
base commit; the wall-removal change is a working-tree diff on top of it at
capture time, so the tool's `git rev-parse HEAD` names the base, not a
B9-specific commit). This run's raw JSON output was not committed, matching
the existing `harness_out/` git-ignore rule; rerun the documented command to
reproduce it.

#### Attribution shares (regenerated; shipped route now covers all four fixtures)

| lane | fixture | wall ns/op | coverage % | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | recovery | other |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.381ms | 110.9 | 23.6 | 6.7 | 22.5 | 11.2 | 11.2 | 23.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | query_compile | 11.548ms | 110.1 | 21.3 | 6.7 | 28.1 | 11.2 | 14.6 | 18.0 | 0.0 | 0.0 | 0.0 |
| diagnostic lane | language | 12.156ms | 109.7 | 26.1 | 2.3 | 28.4 | 12.5 | 15.9 | 13.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | grammargen_lr | 126.493ms | 110.7 | 27.6 | 5.1 | 23.5 | 12.2 | 12.2 | 17.3 | 0.0 | 0.0 | 2.0 |
| shipped route | rewrite | 2.480ms | 109.9 | 15.9 | 3.4 | 30.7 | 12.5 | 21.6 | 15.9 | 0.0 | 0.0 | 0.0 |
| shipped route | query_compile | 11.661ms | 110.6 | 15.7 | 4.5 | 30.3 | 15.7 | 13.5 | 20.2 | 0.0 | 0.0 | 0.0 |
| shipped route | language | 12.782ms | 110.5 | 16.9 | 4.5 | 30.3 | 9.0 | 18.0 | 20.2 | 0.0 | 0.0 | 1.1 |
| **shipped route** | **grammargen_lr** | **140.751ms** | 111.3 | 11.7 | 10.6 | 29.8 | 8.5 | 19.1 | 20.2 | 0.0 | 0.0 | 0.0 |

`grammargen_lr` has a shipped-route row for the first time. The public
`Parser.Parse` route, with no per-Parser pin and no diagnostic flag, admitted
it at 140.751ms against the diagnostic lane's 126.493ms for the same
fixture -- a wall-time gap of 11.3% (not to be confused with the unrelated
111.3 coverage % figure in that same table row above; coverage % is
profiled-CPU-time over wall-time, see "Attribution shares" earlier in this
document). Read this 11.3% gap against the noise floor below before treating
it as a finding. A same-shape gap would be consistent with the compat-tail
and admission-switch bookkeeping the shipped route pays and the diagnostic
runner does not (`normalizeReturnedTreeForParse`,
`resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree`, plus
`Parser.Parse`'s own dispatch); `compat-tail` itself still reads 0.0% because
this run's four fixtures stay clean (no error nodes, no C-recovery-swallowed
error to resolve), so any such cost here is admission-switch dispatch
overhead, not compat-tail's own conditional work. Every shipped-route sample
was verified, by the admission-candidate routed/fallback counters, to have
actually taken the compact route rather than falling back to production.

#### Noise floor (this run, interleaved A/A, 10 pairs per fixture)

| fixture | pairs | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|---:|
| `grammargen_lr` | 10 | 120.831ms | 27.151ms | 22.47% |
| `language` | 10 | 12.164ms | 2.994ms | 24.61% |
| `query_compile` | 10 | 11.770ms | 4.986ms | 42.36% |
| `rewrite` | 10 | 2.458ms | 378.273us | 15.39% |

This run's noise floor is wide (15.4%-42.4% of median), consistent with a
busy shared host (other agents were active throughout this session). The
11.3% shipped-route/diagnostic-lane gap on `grammargen_lr` above sits BELOW
every one of these four floors, including `grammargen_lr`'s own (22.47%) and
`rewrite`'s (the narrowest, at 15.39%). This run's noise cannot resolve that
gap from zero: it is not a finding, directional or otherwise, until a
quieter run repeats the measurement with a floor tighter than 11.3%.

#### Cost per event (diagnostic lane; average, not causal)

| fixture | wall ns/op | ns/shift | ns/reduction | ns/pop path | ns/pop payload | ns/canonicalization | ns/election |
|---|---:|---:|---:|---:|---:|---:|---:|
| `rewrite` | 2.381ms | 1766 | 1583 | 1446 | 795 | 973 | 2298 |
| `query_compile` | 11.548ms | 1728 | 1538 | 1424 | 784 | 934 | 2253 |
| `language` | 12.156ms | 1867 | 1607 | 1451 | 769 | 987 | 2403 |
| `grammargen_lr` | 126.493ms | 1913 | 1658 | 1512 | 833 | 1036 | 2534 |

Every per-event reading sits within this document's established noise
floors from prior receipts. The wall-removal change moved routing, not the
compact engine's per-event cost.

This is a local, one-run, noisy-host receipt, not a sealed epoch. It does
not replace or invalidate the first receipt or its addenda above (all
measured before tranche B9 landed, correctly showing `grammargen_lr`
diagnostic-lane only at that time); it extends the same methodology to the
newly shipped-route-eligible fixture.

## 2026-08-22 P24a memo-probe performance receipt

Status: **NO-GO**. Do not ship the candidate code.

The receipt uses base commit `6c94426b69f5aa106873b0d667dd4112acc138f9`.
The candidate changes only these files:

- `parser_recover_c.go`
- `parser_recover_c_test.go`

The full candidate diff SHA-256 is
`d56566edb8d6587a13935dc13bc173db892b099950fb612f5c95c688f93cdeea`.
Compute it with:

```text
git diff --no-ext-diff --binary 6c94426b69f5aa106873b0d667dd4112acc138f9 -- parser_recover_c.go parser_recover_c_test.go | sha256sum
```

### Accepted comparison

The accepted publication used 20 seeds and 40 isolated benchmark processes.
Each seed ran one baseline process and one candidate process.
Odd seeds ran baseline first. Even seeds ran candidate first.
Each process used `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and
`-benchmem`. The benchmark expression selected the three canonical controls.

All 40 benchmark processes passed and emitted 120 benchmark rows.
No run reported an out-of-memory event, timeout, panic, or contamination
marker.

The focused Docker gate passed at
`/tmp/gts-p24a-cnode-memo-probe/harness_out/docker/20260822T170841Z-p24a-refresh-6c94426b-focused15-corrected-20260822`.
The historical `focused15` label is not a test count. The command ran 17
top-level tests and 20 tests when its three subtests were counted.

Exclude both prior attempts from this receipt.
Exclude the earlier focused attempt at
`/tmp/gts-p24a-cnode-memo-probe/harness_out/docker/20260822T170828Z-p24a-refresh-6c94426b-focused15-20260822`.
Its expression ran only five non-memo tests.
Exclude the quarantined publication at
`/tmp/gts-p24a-publication-20260822T170915Z`.
It contains the `QUARANTINED` marker and no accepted benchmark rows.

### Benchstat results

The following values are benchstat medians from
`/tmp/gts-p24a-analysis-20260822T171127Z/benchstat.txt`.

| Benchmark | Baseline | Candidate | p-value | Result |
|---|---:|---:|---:|---|
| `BenchmarkGoParseIncrementalNoEditDFA` | 2.342n | 2.349n | 0.401 | No significant change |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 721.2n | 727.9n | p<0.001 | **+0.93% regression** |
| `BenchmarkGoParseFullDFA` | 6.604m | 6.620m | 0.242 | No significant change |

The geomean change is +0.48%. The single-byte edit regression is significant.

### Paired results

The paired means use one baseline and one candidate value for each seed.
Each interval is a 95% paired confidence interval.

| Benchmark | Mean delta | 95% confidence interval | Relative mean |
|---|---:|---:|---:|
| `BenchmarkGoParseIncrementalNoEditDFA` | +0.00345 ns/op | [-0.001566, +0.008466] ns/op | +0.1486% |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | +5.79 ns/op | [+3.607, +7.973] ns/op | +0.8031% |
| `BenchmarkGoParseFullDFA` | +11,140.35 ns/op | [-18,442, +40,722] ns/op | +0.1718% |

The order split reports candidate versus baseline means:

| Benchmark | Odd seeds, baseline first | Even seeds, candidate first |
|---|---:|---:|
| `BenchmarkGoParseIncrementalNoEditDFA` | +0.077% | +0.218% |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | +0.769% | +0.834% |
| `BenchmarkGoParseFullDFA` | +0.099% | +0.238% |

The candidate remains slower in both orders. The order split does not invert.

### Memory and decision

Incremental benchmarks use 0 B/op and 0 allocations per operation in both
variants. Full parsing uses 50.50 KiB/op in both variants and 4 allocations
per operation in both variants. Benchstat reports p=0.066 for full B/op and
p=1.000 for every allocation comparison.

Use this reject rule: treat the first statistically significant positive
delta as the regression point. Reject a candidate when a required lane has
such a delta. Also reject an out-of-memory event, timeout, contamination
marker, or correctness failure.

P24a fails this rule because the single-byte edit lane is +0.93% slower at
p<0.001. Its paired interval is wholly positive. No code ships from this
candidate.

### Receipt sources

- Publication: `/tmp/gts-p24a-publication-20260822T171127Z`
- Analysis: `/tmp/gts-p24a-analysis-20260822T171127Z/benchstat.txt`
- Focused Docker gate: `/tmp/gts-p24a-cnode-memo-probe/harness_out/docker/20260822T170841Z-p24a-refresh-6c94426b-focused15-corrected-20260822`

## Caveats

- This is one run, on one noisy shared host, not a sealed-epoch,
  quiet-host, or enclave receipt. Treat the noise floor as a floor, not a
  ceiling: a quiet or enclave host would very likely show tighter deltas.
- The classifier's function table
  (`cgo_harness/attribution/classify.go`) is the single source of truth for
  every boundary rule in this document. If the two ever disagree, the code
  is authoritative; file a follow-up to reconcile the prose.
- `other` includes Go runtime and GC frames by design (a legitimate,
  expected component), not only classification gaps. The tool reports the
  largest non-runtime unclassified functions separately so a real gap is
  visible rather than silently absorbed.
- Cost-per-event numbers are an average, not a causal attribution (see
  above). Do not use them alone to justify a specific optimization target;
  pair them with the component shares.
