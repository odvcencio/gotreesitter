# Single-version execution and direct compact output

The candidate batches deterministic reductions and the final shift under one fresh-session owner, and can publish an accepted clean tree directly into the compact reader's backing. It uses the existing graph and authenticated native projector. It does not introduce another stack representation, import/flush cycle, or a second pointer tree on the read-only route.

Baseline: `64e22ad72e99a817d6e75c1e6ccd849ede4ec6ef`, on stacked draft PR #1512. These are branch-relative measurements. Main-based qualification, compact graduation, and all merges remain held.

## Execution and output contracts

The internal loop consumes compiled singleton action rows. It constructs bounded, deterministic reductions directly and publishes the resulting head and lineage in the same owned operation. Scanner-produced shifts require an authenticated election; an unbound external token is handed back before its shift is consumed. Ambiguity, extras, recovery, wider pops, historical destinations, contextual-token handling, and other unsupported cases retain the existing scheduler. A reduction handoff carries its already-resolved GOTO, preventing another counted lookup.

The adapter preserves stop polling, dispatch limits, leaf receipts, and dependency publication. The elected token and its examined frontier stay fixed through a batch, so the dependency proof is established once while every resulting payload still receives its receipt. Errors poison and discard the fresh session. Full diagnostics and phase-zero-A retain their reference operations.

`BuildNativeSelectedOutput` writes the authenticated native projection directly to the existing pointer-free `SelectedStore`. The result supports traversal and selected-store queries without building `*Node` objects. Replay scratch is charged to the output budget; published records survive a core reset and release independently. Recovery and unsupported prefix provenance fail closed. Root framing, fields, aliases, compatibility rules, and source coverage use the existing checked paths.

This output route remains an internal read-only prototype. It is not a replacement for public `Parser.Parse`, an incremental tree, a completed structure-of-arrays layout, or D4's lazy stable `*Node` views. Its timings must not be presented as public-API speedups. Both parse-only and complete recursive traversal are timed, and Go highlight queries are checked for identical ordered captures.

## Completed compact-output measurements

Linux amd64, Go 1.26.6, one pinned CPU, GOMAXPROCS=1, 750 ms, count=1, benchmem. Each comparison uses the repository's randomized runner, 20 explicit seeds, matched workload orders, and alternating baseline/candidate order. Correctness and counters complete before timing.

| Read-only workload | Public-tree baseline | Compact result | Parse-time change | With complete traversal |
| --- | ---: | ---: | ---: | ---: |
| Go query source, 20,168 bytes | 9.553 ms | 8.443 ms | −11.61% | −11.53% |
| Go LR source, 235,626 bytes | 101.17 ms | 89.26 ms | −11.78% | −11.81% |
| YAML R4, 1,998 bytes | 197.4 µs | 170.8 µs | −13.48% | −12.97% |
| YAML EOF, 8,012 bytes | 767.3 µs | 673.1 µs | −12.28% | −11.90% |
| Generated Go, 1,048,591 bytes | 615.0 ms | 514.0 ms | −16.42% | −16.83% |

All ten timing differences have p<.001. These are warm-runner comparisons; parser creation and source loading are outside timing. LR allocated bytes fall 59.10%, from 2.564 MiB to 1.049 MiB, and allocations fall 1,807→1,590. Query-source bytes fall 6.99%, with 120 allocations unchanged. YAML R4 bytes fall 8.50% and allocations 39→32. EOF YAML bytes fall 1.53%, trading one additional allocation, 70→71 (+1.43%). On the generated 1 MiB witness, allocated bytes fall 65.61% (24.713→8.499 MiB) and median allocations fall 673→30 (−95.54%). Its source hash matches the independent C gate, and each benchmark preflight compares the complete compact/public metadata before timing. The narrower output contract is part of these comparisons.

With the existing public-tree publisher, 20 pairs measure Go query parsing at 9.543→9.381 ms (−1.70%, p<.001), Go LR at 100.9→100.3 ms (−0.53%, p=.023), YAML R4 at 197.2→186.6 µs (−5.40%, p<.001), and YAML EOF at 771.0→725.4 µs (−5.91%, p<.001). This adapter-level comparison isolates the loop from the new output representation; the real public `Parser.Parse` controls are reported separately below. No public allocation count changes on these four adapter workloads.

The default-route Go control trio preserves all allocation counts and zero-allocation no-edit reparses. Full DFA time moves −0.42%, single-byte editing +0.57% (83.86→84.34 µs, p=.005), and no-edit −1.70%. These controls do not execute the new compact loop; their small timing movements are recorded without attribution or speedup credit.

Twenty pairs through the actual public `Parser.Parse` compact route give:

| Public compact Go workload | Baseline | Candidate | Time change |
| --- | ---: | ---: | ---: |
| Full DFA | 8.705 ms | 7.964 ms | −8.51% |
| Fresh parser, 1 MiB | 621.8 ms | 602.7 ms | −3.06% |
| Query source | 9.615 ms | 9.470 ms | −1.52% |
| LR source | 100.61 ms | 99.52 ms | −1.08% |

All four have p<.001. All public allocation counts are unchanged. Single-byte edit time rises 0.34% (84.15→84.43 µs, p=.001); no-edit falls 1.98% and remains zero bytes/allocations. The fresh verifier remains in place. The no-edit timing movement does not exercise the new loop and is not credited to it. All timed work and route metrics remain exact in the paired audit.

Public ordinary YAML improves 6.27% on R4 (207.6→194.6 µs) and 6.86% on EOF 8 KiB (782.8→729.1 µs), both p<.001. Diagnostic-owned YAML improves 1.31% / 1.89%, while legacy control timings are inconclusive. All six public YAML allocation counts are unchanged. The ordinary route is exercised without enabling the diagnostic ownership override.

The independent audit retains 1,040 benchmark rows per role across the initial and revised experiments, verifies every paired workload order and alternating role order, and checks every timed work/route metric. All timing, allocated-byte, and allocation-count median ratchets pass. RSS is recorded separately below; these are not graduation receipts.

## Correctness and deterministic work

The revised loop passes 412 invariant sessions across 206 grammars and both routes: 29,664 edit steps, with zero no-edit allocations. All 1,236 standard locked-C checks pass without skips. All 824 complete-operation work rows match the baseline, and all 206 selected ledgers preserve work, reuse, coverage, error, completion, and route metadata exactly. No ledger exception, reset, or allowlist change is used.

| Fixture / route / operation | Tokens before→after | New nodes before→after | Reused bytes before→after | Splices before→after |
| --- | ---: | ---: | ---: | ---: |
| go R4 / candidate / full | 589→589 | 893→893 | 0→0 | 0→0 |
| go R4 / candidate / first edit | 374→374 | 1553→1553 | 1784→1784 | 266→266 |
| go R4 / default / full | 589→589 | 1922→1922 | 0→0 | 0→0 |
| go R4 / default / first edit | 572→572 | 3810→3810 | 1166→1166 | 296→296 |
| yaml R4 / candidate / full | 107→107 | 232→232 | 0→0 | 0→0 |
| yaml R4 / candidate / first edit | 107→107 | 232→232 | 0→0 | 0→0 |
| yaml R4 / default / full | 107→107 | 288→288 | 0→0 | 0→0 |
| yaml R4 / default / first edit | 107→107 | 480→480 | 0→0 | 0→0 |

Core normal/race/386/shadow checks, focused root and race suites, 20 memory contracts, larger YAML ownership/EOF checks, and the generated 1 MiB Go C comparisons pass. Differential tests cover graph/link/subtree/lineage identity, bounded reduction chains, handoff, cancellation, authenticated external shifts, and unbound-token refusal. Direct-output tests compare complete metadata on four Go witnesses and ordinary/owned YAML witnesses, plus ordered Go captures; ownership, limits, cancellation, table identity, and repeated occurrences have explicit tests. The all-language public checks qualify the loop; they do not imply all-language admission of the new read-only output route.

The latest Cognociente report remains pinned at [issue #454, comment 6311551850](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850). Its exact 137 KiB witness still has 12 independent incremental/fresh agreements and eight clean C agreements. Four malformed-delete C checks retain the known ERROR-child mismatch. The 31-byte reduced witness remains unresolved. Neither the fresh verifier nor this failure is waived.

## Why this is not yet the larger Go speedup

The new loop executes 35,157 reductions and 30,869 shifts on the Go LR witness. General dispatch still runs 59,842 times; 53,173 of those passes have multiple headers. That frontier is outside the single-version proof. In YAML, the loop handles 693 of 735 reductions and 321 of 337 ordinary shifts. Profiles put the remaining cost in different places: ambiguous-frontier construction/publication in Go, and election/scanner work plus construction in YAML. Cumulative profile rows overlap and are not added into a savings estimate.

The first loop version is retained as a rejected screen: public Go timing is inconclusive and public YAML parsing regresses 2.52–2.60%. Moving loop entry after opcode gates, reusing the decoded first row, avoiding duplicate entry polling, and keeping authenticated scanner shifts inside the loop produces the revised candidate. A checkpoint test initially assumed the general core rejected an unbound token immediately; the revised fast path instead refuses that shift and leaves the original publication contract intact. The frozen V1/V2/V3 sources, the V2 checkpoint-test failure, and every randomized sample remain in the receipts. Earlier development logs in the shared focus directory were overwritten by subsequent focused runs; they are not represented as complete archived runs.

No new native-C speed ratio or ARM64/native-platform qualification is claimed. The earlier archived reduction experiment's C ratios do not describe this candidate. Existing malformed-input, main-based, memory, and graduation holds remain.


## Evidence, memory, and integration limits

The [qualification](qualification.json), [source manifest](qualified-source.json), [paired-sample audit](performance-analysis.json), and [correctness audit](correctness-analysis.json) retain the exact scope. The [complete receipts](receipts.tar.gz) include frozen sources, overlays, commands, raw measurements, profiles, counters, and test logs. Every one of the 2,080 timing rows is retained. Replaying on the final checkout explicitly hides the newly installed candidate-only core test from the baseline; that file was absent when the baseline ran. The committed core test differs from the measured test only by gofmt, and the installed runtime is byte-identical to V3.

Tables separate [read-only Go](benchstat-v3-direct-go.txt), [read-only YAML](benchstat-v3-direct-yaml.txt), [read-only 1 MiB](benchstat-v3-direct-large.txt), [public compact Go](benchstat-v3-compact-primary.txt), [public YAML routes](benchstat-v3-yaml-routes.txt), and [default-route controls](benchstat-v3-primary.txt). Adapter-only tables remain available for [Go](benchstat-v3-public-go.txt) and [YAML](benchstat-v3-public-yaml.txt).

Bounded public-compact 1 MiB RSS probes report 237,984 KiB for the baseline and 230,596 KiB for the candidate, one process each, both exit zero. These are completion/footprint probes, not a 20-pair RSS ratchet or a claim about the read-only output's peak memory. The first RSS command stopped with exit 127 because the pinned image lacks GNU time. Only that uncompleted step was resumed, mounting the host's hashed GNU time executable read-only; completed correctness and throughput samples were not rerun or discarded. Container audits report no OOM kills.

The public single-version loop is installed on the draft branch. Direct compact output is available internally for qualified read-only consumers and tests; public lazy views and incremental integration remain outstanding. No merge, allowlist change, new work-counter exception, C-speed claim, or graduation follows this bucket.
