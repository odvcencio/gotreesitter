# Streaming native projection: public-path screen

The lifecycle-fixed streaming builder measures **3.70% / 3.71% less owned-YAML
parse time** on R4 / the 8 KiB witness, with **one extra allocation and 248 bytes**
per parse. It regresses public Go **2.57% / 1.17%**, so the generic candidate is
not promoted. This is a measured diagnostic YAML pathway, not ordinary YAML
activation or compact graduation. The independently qualified reduction in
[core PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485) and selected
output work in [PR #1484](https://github.com/odvcencio/gotreesitter/pull/1484)
remain independent buckets.

Baseline: `22554e6bafe29422f0e7fed4b10cdb3e10db13ec`, the pinned output lane.
Evidence directories under the local `gotreesitter-evidence` store:
`20261014-native-streaming-projection` (two walks),
`20261015-native-streaming-fused-replay` (fused),
`20261015-native-streaming-qualification` (initial C/memory probes),
`20261015-native-streaming-lifecycle` (fixed source and timing), and
`20261015-native-streaming-lifecycle-qualification` (final C/memory probes).
The timing comparison uses a retained private runner through the real `Parser.Parse` API in both roles. It cannot be combined with
measurements on other baselines or runner lifecycles. Both roles share the
diagnostic hook and selected-policy/provenance extensions; the baseline retains
the existing native publisher, while the candidate binds the streaming publisher.

## Construction and correctness

The core emits public occurrences in iterative postorder into a native sink.
Native headers hold the construction state; an eight-byte side record retains
source payload, incoming field and projection flags. Existing logical-frontier
and traversal buffers are reused. No selected-record or selected-child arena is
built on the candidate publication path. Hidden wrappers follow the existing
selected/native precedence behavior; folded unary reductions retain their own
precedence and provenance.

The candidate retains authenticated table identity and metadata validation,
occurrence and child limits, stop controls, charged arena/scratch backing,
raw-child tiling, accepted-root leading-gap checks and native result framing.
Unary folds require exact raw/output spans, and visible children must remain
inside the raw parent. Unsupported recovery, reused-subtree expansion,
inherited/duplicate fields and lexer-prefix provenance decline. Scanner
checkpoint copying and ordinary replay/read/dependency publication remain
explicit; an unrepresented hidden checkpoint declines. None of these diagnostic
scope rules enables ordinary YAML ownership or changes an allowlist.

Separate Docker processes for Go and owned YAML pass the final focused gate:

- Four canonical Go files, eleven additional accepted Go snippets and the two
  owned-YAML witnesses match current native/selected structure and byte spans,
  fields, production IDs, precedence and supertype masks.
- Checked private parse/pre-goto states, proof bits, fragility, column flags
  and legacy lexer-read words match. Go additionally checks field-source origins
  and compact dependency byte bounds.
- All seventeen accepted witnesses retain exact committed core work. The
  generic-call witness retains its existing pre-projection live-link-cap decline.
- A six-edit, 8,938-byte Go session equals fresh parsing at each step. Both
  initial-tree variants reuse 8,712 / 8,718 / 8,719 / 8,712 / 8,724 / 8,724 bytes.
  No-edit reparsing allocates 0 bytes and 0 objects.
- Public-entrypoint tests require one successful streaming publication per
  parse, excluding silent fallback. A forced one-byte working-set budget returns
  no root or arena.

| Native node constructions | Before → after |
| --- | ---: |
| Go, 5,116 bytes | 2,247 → 1,524 |
| Go, 20,168 bytes | 11,326 → 7,524 |
| Go, 41,387 bytes | 10,917 → 7,083 |
| Go, 235,626 bytes | 107,964 → 71,768 |
| Owned YAML, 1,998 bytes | 484 → 232 |
| Owned YAML, 8,012 bytes | 1,936 → 925 |

These are construction reductions, not speed estimates. Native nodes remain
104 bytes and selected records remain 36 bytes. The single-document YAML result
has 231 reachable public nodes and constructs one additional compatibility node.

## Timing protocol and exclusions

The repository randomized runner uses twenty shuffled, alternating pairs,
`GOMAXPROCS=1`, `-count=1`, `750ms` and `-benchmem`, serialized in a 6 GiB,
one-CPU Docker container pinned to CPU 6. Each retained parser completes a warm
parse before its timer starts. Go includes the default full/edit/no-edit trio;
owned YAML runs in separate processes. Candidate benchmarks require a successful
streaming publication on every warm and timed parse.

An initial correctness attempt rejected hidden-wrapper precedence rather than
following the existing elision behavior; it is excluded after correction. An
intermediate test regex unintentionally admitted the YAML subtest in the Go
process; final anchored, language-isolated runs supersede that run. The first
timing attempt included retained-parser scratch growth inside the measured
region. It was deliberately stopped at exit 143 and all its partial samples are
excluded. Warm-parser measurements have their own immutable source pins.

The two-walk streaming screen completes all twenty pairs per benchmark, with
unchanged source hashes and Docker exit zero. It establishes no public Go or
owned-YAML throughput gain and is not promoted. All four target timings are
inconclusive; the default edit control regresses **4.29% (p=.024)**.

| Benchmark | Before → after time | p | Bytes before → after | Allocations before → after |
| --- | ---: | ---: | ---: | ---: |
| Public Go, 20 KiB | 11.488 → 12.096 ms | .086 | 141,140 → 142,972 (+1.30%) | 205 → 206 |
| Public Go, 235 KiB | 126.060 → 127.333 ms | .289 | 3,295,768 → 3,304,774 (+0.27%) | 3,092 → 3,095 |
| Default Go full DFA | 4.567 → 4.518 ms | .512 | 1,161 → 1,161 | 6 → 6 |
| Default Go byte edit | 111.021 → 115.785 µs (+4.29%) | .024 | 394 → 394 | 5 → 5 |
| Default Go no edit | 3.4385 → 3.4720 ns | .683 | 0 → 0 | 0 → 0 |
| Owned YAML R4 | 433.778 → 435.838 µs | .820 | 12,936 → 13,616 (+5.26%) | 111 → 111 |
| Owned YAML 8 KiB | 1.713 → 1.683 ms | .063 | 44,601 → 45,281 (+1.52%) | 345 → 345 |

The allocation increases clear the 10% allocation ratchet, but no time benefit
justifies this integration. This warm-run comparison is independent of the
earlier plan-based integration, whose parser lifetime and initialization costs
differ. Neither throughput nor allocation percentages are combined across them.

## Fused replay successor

The successor combines raw traversal, exclusive-ownership colors, state replay,
projection and native header stamping in one walk. It reuses the existing replay
frame rules: each root starts at the root pre-goto state; each child advances its
parent cursor even when the transition is unknown. It rejects cycles and repeated
raw ownership. The native sink resolves source points only when byte spans change.
No selected plan, replay-state array or second raw replay walk is retained.

The final language-isolated focused gate passes the same seventeen witnesses,
with explicit per-node point equality added, exact committed core work and the
same six-edit/no-edit results. The public hook additionally binds the exact core,
accepted head and payload sequence. Regressions reject a foreign core, invalid
head, altered payload list and wrong source extent, then publish successfully
without rebuilding the scheduler. The forced memory-budget rejection still passes.

The first fused attempt incorrectly treated quiescent checkpoint metadata as a
required scanner transfer. Its failed correctness run is excluded. The final
hidden-checkpoint rejection applies only when the language requires transfer and
the raw terminal carries a complete checkpoint pair. This preserves the existing
scanner capability boundary. No geometry-only standalone screen was run; that
prepared variant is superseded by the fused successor.

The fused screen also completes twenty pairs per benchmark, with exact source
pins and Docker exit zero. Its two YAML improvements are statistically supported;
both public Go directions remain inconclusive. These results are independent of
the two-walk trial and are not added to earlier reduction or projection wins.

| Benchmark | Before → after time | p | Bytes before → after | Allocations before → after |
| --- | ---: | ---: | ---: | ---: |
| Public Go, 20 KiB | 10.848 → 10.570 ms | .862 | 141,140 → 143,051.5 (+1.35%) | 205 → 208 |
| Public Go, 235 KiB | 117.386 → 111.902 ms | .738 | 3,295,768 → 3,304,848 (+0.28%) | 3,092 → 3,097 |
| Default Go full DFA | 3.996 → 3.934 ms (−1.54%) | .021 | 1,160 → 1,159 | 6 → 6 |
| Default Go byte edit | 105.505 → 102.152 µs | .201 | 394 → 394 | 5 → 5 |
| Default Go no edit | 3.2555 → 3.2210 ns | .204 | 0 → 0 | 0 → 0 |
| Owned YAML R4 | 401.713 → 382.625 µs (−4.75%) | .002 | 12,936 → 13,696 (+5.88%) | 111 → 113 |
| Owned YAML 8 KiB | 1.603 → 1.494 ms (−6.83%) | .006 | 44,601 → 45,361 (+1.70%) | 345 → 347 |

The default full-DFA direction is a control result, not a streaming speed claim.
Its byte-distribution difference is inconclusive (p=.057); edit byte medians stay
394 with p=.182. No-edit allocation stays exactly zero. All reported construction
counts remain identical to the two-walk table. The warm, retained-runner benchmark
does not certify throughput of ordinary pooled routing or ordinary YAML ownership.

## Direct C and memory checks

The qualification overlay copies the same fused builder and native sink into a
non-test diagnostic binding at normal runner construction. Its publication count
must increase on each owned witness; matching C through an old materializer or a
legacy fallback cannot satisfy that assertion. It exercises normal runner pooling
without changing the repository runtime. Qualification and timing source hashes
are recorded separately.

All four canonical Go files, including 235,626 bytes, match the locked C oracle
with exactly one streaming publication and no fallback. The owned YAML R4 scanner
transfer proof and the 1-, 4- and 16-document C checks pass; ordinary four-document
YAML retains its existing no-action decline and has zero streaming publications.
The additional seven-case YAML process also passes the exact R4 sample and its
leading-space and leading-newline variants. Each owned case records exactly one
streaming publication; the ordinary case records zero.

The **1,049,572-byte / 524-document YAML witness** is C-exact, covers the input
without error or fallback and records one streaming publication with **121,045
native constructions**. Peak RSS is **142,256 KiB**, below the 400-bytes-per-source-
byte bound. This is an absolute memory receipt, not a paired RSS improvement.
Three repetitions each of the adversarial Go and synthetic wide-GLR budget tests
pass: peaks are **1,072,880 / 13,068 bytes** under their existing 1 MiB contracts.
The independent wide link-cap decline remains intact.

A completed 6.32-second CPU-sample profile of fused large Go attributes 78.64%
cumulative CPU to scheduler execution, 25.63% to generic owned reduction, 16.61%
to the core reduction body and 20.41% to native publication. These shares overlap;
they must not be added or treated as speedups. The reduction-focused profile
attributes 5.22% cumulative CPU to atomic condensation and 3.48% to pop-path work.
These guide the next Go investigation, while the YAML result proceeds through
qualification.

## Publication lifecycle successor

A subsequent audit finds two blockers in the measured prototype: the root scratch
can retain a released public tree, and the publication hook lacks an explicit
comparison-purpose guard. Its Go compatibility frames also need the existing
runner cleanup contract. These findings prevent promoting the measured source.

The successor in `20261015-native-streaming-lifecycle` clears root slots, restores
`resetTreeBuffers`, restricts the hook to fresh publication without borrowed or
eager materialization, and reuses/charges the runner's link and compatibility
scratch. New regressions inspect every public-pointer slot through slice capacity,
verify that clearing scratch leaves the returned tree intact, and require
comparison trees to remain ineligible for publication/reuse proofs. All seventeen
witnesses, exact core work, the six-edit session, zero-allocation no-edit case,
selection binding and forced-budget checks pass on that source.

Twenty pairs on the fixed source complete with exact hashes and Docker exit zero.
The less variable Go distributions now establish regressions; the earlier noisy
Go medians do not count as a speed win. The corrected source still improves both
owned-YAML witnesses. Neither result is combined with the preceding screen.

| Benchmark | Before → after time | p | Bytes before → after | Allocations before → after |
| --- | ---: | ---: | ---: | ---: |
| Public Go, 20 KiB | 10.050 → 10.309 ms (+2.57%) | <.001 | 141,139 → 141,387.5 (+0.18%) | 205 → 206 |
| Public Go, 235 KiB | 108.059 → 109.324 ms (+1.17%) | .005 | 3,295,768 → 3,296,016 (+0.01%) | 3,092 → 3,093 |
| Default Go full DFA | 3.877 → 3.876 ms | .620 | 1,159 → 1,159 | 6 → 6 |
| Default Go byte edit | 99.434 → 99.910 µs | .127 | 394 → 394 | 5 → 5 |
| Default Go no edit | 3.129 → 3.131 ns | .387 | 0 → 0 | 0 → 0 |
| Owned YAML R4 | 388.120 → 373.763 µs (−3.70%) | <.001 | 12,936 → 13,184 (+1.92%) | 111 → 112 |
| Owned YAML 8 KiB | 1.509 → 1.453 ms (−3.71%) | <.001 | 44,601 → 44,849 (+0.56%) | 345 → 346 |

All native construction counts remain exact. Default trio directions are
inconclusive, and byte/allocation medians stay unchanged; full-DFA byte
distribution p=.221. The +248-byte allocation trade stays below the ratchet.
A language-general activation would worsen Go, so it remains external. A future
selector must be based on measured structural/capability evidence and pass its
own gates; no engine language-name comparison is introduced.

Final-source qualification in `20261015-native-streaming-lifecycle-qualification`
passes all four canonical Go/C comparisons, YAML scanner proof, all seven sample/
document/prefix cases, and the 524-document large C witness. Every owned witness
requires a streaming publication; ordinary YAML still declines. The large case
constructs 121,045 nodes, covers all 1,049,572 bytes without error or fallback and
uses **143,840 KiB peak RSS**. All six repeated budget probes pass with unchanged
1,072,880 / 13,068-byte peaks. Runtime and qualification-overlay hashes stay exact;
all final correctness and timing containers exit zero. The normal-pool bridge is
qualification-only; it supplies no pooled throughput claim.

This candidate remains a **measured, C-checked owned-YAML pathway**, not a general
runtime change. Its Go regressions preclude enabling it indiscriminately. The
qualified reduction PR remains unchanged, and ordinary YAML's ownership proof
still blocks activation.

## Qualification limits

This is a focused architectural screen with the named final-source C and memory
checks. It does not inherit full C/fleet/race qualification from the baseline or
earlier experiments. The new locked-C and large-YAML checks cover only the
named witnesses.
All-language invariants and the 412-row ledger, general alias/recovery/borrowed-tree
publication, lifecycle/source gates, races and wider large-file qualification
remain required before integrating runtime code. Ordinary YAML ownership, the
compact default/allowlist and graduation are unchanged. No merges are made in this work.
