# Ordinary YAML ownership: correctness progress, performance rejection

The single-prefix prototype reaches compact acceptance on the ordinary YAML
R4 and document streams without relaxing snapshot equality or dropping an
alternative. It is **not promoted**: the first 20-pair screen regresses ordinary
R4 / 8 KiB time 22.61% / 48.47%. A separate scratch-reuse screen reduces bytes
but still regresses time 16.02% / 41.08%. Neither supplies a graduation claim.
All merges to main remain held.

Baseline: `c498e889ec2d86382a3a0baad58020d7df7145b3`. Evidence date: 2026-10-15
UTC. Frozen private source and raw evidence remain in local
`gotreesitter-evidence/20261015-single-prefix-lexer-ownership`.

## Ownership mechanism

A diagnostic locates the first ordinary YAML conflict at election 10, state
816, byte 310, with one live header; the four-document witness reaches the
same state at election 11, byte 314. The proposed internal prefix proof starts
only at an empty fresh Core seed. Any unowned split permanently invalidates
that prefix; collapsing back to one header cannot revive it. Only the existing
authenticated owned-to-shared rejoin can begin another prefix.

Before that first conflict, the scheduler transfers the already executed shared
request into private exact before/after snapshots. It keeps the original elected
lexer state, even when reductions changed the current graph state while the
lookahead remained pending. It does not scan the token again. Activation requires
an unchanged scanner binding and contract, exact checkpoint bytes, one original
elected state, an unchanged token, a clean sole header, and a scanner owning the
complete terminal lexicon. An advertised cross-tree identity remains mandatory;
within-parse ownership does not certify cross-tree reuse. Diagnostic ownership
activation remains separate.

The execution-mode bit belongs to the scheduler transaction frame, rolls back
with its checkpoint, and clears when its owner ends. Canonicalization preserves
independent pending requests only under diagnostic ownership or this actual
owned execution mode. Unit regressions cover stale/foreign owners, rollback,
completion without Core reset, and an unowned split that canonicalizes to one.

The earlier broad attempt changed 13 ordinary ledger rows, including a Djot
acceptance regression, and raised diagnostic YAML tokens. It was rejected before
timing. The final narrower attempt changes only ordinary YAML admission and
keeps both diagnostic-owned rows exact. These attempts are stored separately;
their evidence is not interchangeable.

## Completed correctness and work screen

All seven focused locked-C comparisons pass: ordinary R4, ordinary one/four/
sixteen documents, and diagnostic-owned one/four/sixteen documents. Every final
case requires compact=1, fallback=0 and compares the complete public structure,
fields, byte/point spans and error flags. The four-document source is 8,012 bytes,
SHA-256 `778d1dec01bef4da1cef26acff6b8012c9701a23af67bdb9511b419912f04400`.

Full core and phase-zero-A tests pass. Focused root/scanner/canonical checks,
Go/YAML/CMake/HTML/Python edit invariants, the diagnostic-owned YAML invariant,
and focused locked-C checks pass in serialized Docker, one language per process.
The static 206 table-shape receipt and default/opt-in C4 checks pass.

Of 412 ordinary ledger rows, **411 remain exact**; ordinary YAML changes from
decline to acceptance. Both diagnostic-owned YAML rows remain exact. Because the
normal ledger omits counters after a decline, a separate observation-only probe
continues collecting the actual public fallback result:

| Ordinary YAML candidate phase | Tokens before → after | Public node constructions before → after | Reused bytes / splices before → after |
| --- | ---: | ---: | ---: |
| Full parse | 107→107 | 288→484 (+68.06%) | 0 / 0→0 / 0 |
| First edit | 107→107 | 288→484 (+68.06%) | 0 / 0→0 / 0 |

The first edit still takes the fresh fallback, with
`candidate_incremental_not_routed`; no incremental route is certified. These
ordinary counters are not covered by the owner's earlier diagnostic-owned
482→484 approval. No exception is requested for this rejected performance trade.

## Twenty alternating shuffled pairs

The repository runner uses one process per seed, `GOMAXPROCS=1`, `-count=1`,
750 ms and `-benchmem`, under both locks in serialized 6 GiB Docker on CPU 6.
All frozen source hashes and all 20 pairs per benchmark pass the audit. Timed
ordinary routes change from fallback=1 to compact=1 on every operation. Other
route counters remain exact.

| Public route | Median time before → after | Direction | B/op before → after | Allocations before → after |
| --- | ---: | ---: | ---: | ---: |
| Ordinary R4 attempt | 195.547→239.763 µs | +22.61%, p<.001 | 17,144→22,536 | 37→60 |
| Ordinary 8 KiB attempt | 617.972→917.472 µs | +48.47%, p<.001 | 58,057→88,282 | 34→172 |
| Legacy R4 control | 147.789→149.270 µs | +1.00%, p=.004 | 1,440→1,440 | 16→16 |
| Diagnostic-owned R4 control | 358.167→356.299 µs | −0.52%, p=.265 | 12,872→12,872 | 109→109 |
| Diagnostic-owned 8 KiB control | 1,385.270→1,394.944 µs | +0.70%, p=.289 | 44,312→44,312.5 | 336→336 |

The legacy control bypasses the changed scheduler path, so its direction is not
attributed to ownership transfer. Both diagnostic timing directions are
inconclusive; the half-byte median is retained rather than presented as a
meaningful allocation change. Ordinary compact remains substantially slower
than legacy, even though it improves on the diagnostic-owned route's cost.

The first final-source Buckley review times out without a completed report;
later review and the private binding correction are described below. Full fleet
invariants, the 618-fixture C fleet, strict census, races, memory budgets and
large-file RSS are not transferred to these rejected prototypes. The qualified
runtime, allowlists and global default remain unchanged.

## Reusing the existing after-snapshot scratch

An allocation profile attributes 16,768 bytes per parse (about 16.4 KiB) to a
fresh temporary after-snapshot buffer under `transferSinglePrefixLexerRequest`.
The profile includes setup and is attribution, not a warm throughput benchmark.
A one-line follow-up fills the existing `relexAfterScratch` instead; the owned
constructor still clones all four mutable buffers. This remains an exact
snapshot transfer, without a scanner-state equality exception.

This separate screen again compares with production `c498e889e`, not with the
first prototype. All 20 alternating shuffled pairs, source pins, expected route
changes and work checks pass. The seven focused C cases, full core/phase-zero-A
and focused Go/YAML/CMake/HTML/Python edit and C checks pass before timing.
411 ordinary rows and both diagnostic-owned rows remain exact; ordinary YAML
admission changes as above, with the same observed 288→484 construction cost.

| Public route | Median time before → after | Direction | B/op before → after | Allocations before → after |
| --- | ---: | ---: | ---: | ---: |
| Ordinary R4 attempt | 195.548→226.878 µs | +16.02%, p<.001 | 17,144→5,896 (−65.61%) | 37→54 |
| Ordinary 8 KiB attempt | 620.612→875.562 µs | +41.08%, p<.001 | 58,057→20,952 (−63.91%) | 34→142 |
| Legacy R4 control | 147.911→148.539 µs | +0.42%, p=.002 | 1,440→1,440 | 16→16 |
| Diagnostic-owned R4 control | 357.988→357.610 µs | −0.11%, p=.301 | 12,872→12,872 | 109→109 |
| Diagnostic-owned 8 KiB control | 1,384.475→1,377.590 µs | −0.50%, p=.211 | 44,312→44,312 | 336→336 |

The lower byte counts are experimental savings, not a qualified runtime win:
time and allocation counts still rise sharply. The bypassed legacy direction
is recorded without attribution; diagnostic timing remains inconclusive. No Go
timing comparison or full qualification follows this failed YAML screen. Raw
samples, profiles and audits are in `20261015-single-prefix-reusable-after`.

## Historical scanner binding: private correctness follow-up

Buckley's completed source review catches a gap in this private prototype:
activation checks the current scanner binding across callbacks, but does not
retain the binding that produced the historical before-snapshot. Pointer-backed
fingerprints already distinguish replacement. Opaque value-backed scanners can
share a fingerprint while their interface bindings differ.

A fresh-owner regression uses two such scanners whose closures own different
payloads. Before the fix, replacement wrongly activates ownership and creates
one request. The fix captures the actual binding before serialization, retains
it with the before-snapshot, checks it before ownership activation, and clears
it with version state. Every prior contract, identity and payload check stays.
The live election path passes that captured binding explicitly; the existing
private pre-serialized test helper keeps its call signature.

The regression fails before and passes after. Snapshot/canonical tests, all
seven positive C comparisons and the 72-step YAML invariant pass; no-edit
reparses allocate zero. Both ordinary YAML ledger rows exactly match the
preceding prototype. Source hashes and terminal exits pass the audit in
`20261015-single-prefix-binding-guard`. Buckley's focused follow-up finds no
concrete remaining defect in the supplied diff; it does not independently
inspect other callers or verify execution evidence.

**All timings above predate this binding fix.** They do not qualify its source.
The fixed private prototype has no new timing, full-fleet, race, memory-budget
or RSS qualification. No prototype runtime is committed or enabled, and no
counter exception or graduation approval is requested.
