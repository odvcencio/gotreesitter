# LESS malformed slash: issue #1336

The reported slash now produces 80 sibling rules and the exact tree from the
locked C runtime's fresh parse. This receipt starts from main revision
`1621f0014caa7f2422706fecc500388221a076d0`.

The original input has 3,431 bytes. Generate 80 copies of
`.ruleN {\n  padding: 10px;\n  color: red;\n}\n` and insert `/` immediately
after the first `padding:`. The 11-byte nesting witness is `0{a:/a:} {}`.
Both witnesses fail on the baseline. The original Go tree has one top-level
rule with the following 79 rules nested inside it; C has 80 sibling rules.

| Original input, default and compact fallback | Before | After / fresh C |
| --- | --- | --- |
| Root children | 1 | 80 |
| Root span | 0–3431 | 0–3431 |
| Deep digest | `0d95300bec24c7d72ce6fdeaf21b00601bba4b78978a411476a27a8117d8e5ae` | `03af075d5f4dd36e0210cff0ad986212dd8f895f6fbcee8772be8219a2e680d9` |
| Tokens consumed | 1045 | 1042 |
| Nodes allocated | 3243 | 3499 |
| Maximum live stacks | 4 | 4 |
| Error after slash | bytes 19–20 | bytes 19–21 |

## Why the trees differed

C retains the original shift version and appends reduction versions during a
shift/reduce conflict. The shared Go lexer dispatched the reduction first.
Equally ranked paused versions therefore resumed the selector interpretation
instead of the declaration interpretation. The fix records conflict ancestry
in existing stack padding and restores that order for stateless external
scanners enabled in error-mode lexing, on prefixes parsed without subtree reuse. It leaves C's cost and
precedence comparison intact.

A competing binary expression contained an invisible missing integer token.
Public projection omitted that terminal and lost its 610-point recovery cost.
The fix retains the cost when a reduction projects all hidden children away,
including multiple missing terminals and chains of hidden reductions. The
visible parent keeps `HasError()`. The invisible terminal remains absent from
the public tree, matching C.

Finally, C's recovering version lexes a zero-width external marker that consumes
the space after the slash. A normal sibling's shared integer lookahead had
already skipped that space. The fix replays the marker only from the exact
skipped-prefix checkpoint, using a stateless scanner and no included ranges.
Recovery versions consume the marker through the existing C action-cell
routine before seeing the original shared token. Repetition shifts retain
their C no-op behavior. This makes the error end at byte 21, as C does.

An incremental parse that encounters hidden missing-token costs verifies its
result against a fresh Go parse. These costs have no public subtree reuse
proof. A no-edit reparse still allocates nothing.

The engine adds no language-name comparisons. Its new policies live in
`internal/recover`. Stack size remains 104 bytes, Parser size remains 2328 bytes, the original
hot field offsets are preserved, and memory-budget contracts
remain enabled.

## Deterministic counters and pin review

[The counter receipt](issue-1336-counters.json) lists every changed route row,
with before/after counters and locked-C digests. All 412 rows were collected
before timing. The unchanged 2% gate rejects the old Caddy pin first; it passes
after the separate pin refresh. No gate threshold or exemption was changed.

The separate `update(pins)` commit needs owner approval for six rows:

| Row | Before → after | Correctness evidence |
| --- | --- | --- |
| Caddy/default | Full nodes 1267 → 1536; edit nodes 1302 → 1579 | Previously lost error flags now match C; public-tree difference count falls 383 → 381 full and 385 → 383 edited. |
| EDS/default | Full nodes 6125 → 6127; edit nodes 4806 → 4932, versions 1 → 2, reused bytes 9873 → 9664 | Both trees now exactly match fresh C; six differing public-node records become zero. |
| LESS/default | Full nodes 2957 → 3033; edit nodes 1373 → 4340, tokens 327 → 1072, reused bytes 1118 → 0 | C difference count falls 1088 → 206 full and 1092 → 206 edited. The edit includes fresh verification of an unproven reuse frontier. |
| Prolog/candidate and default | Edited `has_error` false → true | Fresh C reports the error; public-node difference count falls 18 → 12. |
| PromQL/default | Edited tokens 146 → 149, nodes 458 → 280, versions 6 → 3 | C difference count falls 350 → 336 while node work falls. The three extra tokens cross the strict 2% ratchet. |

The difference count above is the symmetric difference of public node records,
including paths, fields, spans and flags. It summarizes an audit, not a gate or
a substitute for exact comparison. The original issue witnesses use direct
deep-tree and flag comparisons. The broader LESS corpus sample still has
existing C differences, and is not claimed to graduate.

## Rejected candidates

Ordering scanners outside error-mode lexing also regressed Go's existing
malformed-selector witness: public-node differences from C rose 97 → 98.
The final error-mode token proof preserves its baseline digest and difference
count. Requiring hidden missing costs before any ordering repair preserved Go
but restored the original LESS nesting bug; that candidate was rejected.

Restoring shift order alone selected a falsely clean binary expression; its
invisible missing token had lost its error cost. Making that terminal public
also differed from C and was rejected.

Summing all raw-child costs changed unrelated recovery choices. Angular nodes
rose 865 → 2188 with no tree improvement; ASM's C difference count rose
1148 → 1589. That broader cost change was rejected.

Unrestricted version reordering worsened requirements' C difference count
1061 → 1147. Reordering reused AWK prefixes raised edit nodes 407 → 442 and
worsened its difference count 452 → 461. The final scanner and prefix proofs
preserve both baseline trees and counters.

Restricting order repair to the first cost-free recovery lost later LESS
recovery choices: full nodes rose to 3831 and edit nodes to 6202 while its C
difference count remained 1085/1089. The final scoped order repair gives
3033/4340 nodes and 206/206 differences. Absorbing padding without dispatching
its recovery versions was also rejected in favor of the C action-cell routine.

## Reference and reproduction settings

The reference is fresh C, plus incremental/fresh Go equality (D8), including
root spans, fields, points and flags:

- C runtime: `f5afe475deb7c0bae6407fb776c76824f717bb61` (0.25.1).
- LESS grammar: `2bd739e106a3485bca210cf7b6d25ba09fd10dff`.
- Corpus lock SHA-256: `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
- Docker image: `gotreesitter/cgo-harness:go1.25-local`, Go 1.25.14.
- `GOWORK=off`, `GOMAXPROCS=1`; heavy checks use one grammar per process.
- `benchstat`: golang/perf revision `fd4a688df89207abdabe0a0cf5b2cd9ccfd376d2`,
  the last revision before its Go 1.26 requirement.

The corpus lock was fetched and its digest verified. It was not copied into
the repository. The requested Go 1.25-compatible image and existing C cache
were used throughout.

## Validation

The focused hidden-missing-cost tests, hot-layout test, LESS regressions and
locked-C fresh/incremental comparisons pass. The direct slash edit session
checks clean → slash → clean → slash on both routes, compares every result
with fresh Go and fresh C, and requires zero allocations for no-edit reparses.

All 206 languages passed the fresh, incremental and error-flag parity suites
in separate Docker processes: 690 subtests ran with no language skips. The
full R4 72-step invariant sessions passed for 198 of 206 languages, including
LESS. The other eight also fail on the baseline:

- AWK, JavaScript and Meson have the same incremental/fresh digest mismatch.
- Elsa, Haskell, Kotlin and PowerShell hit the same three-minute timeout,
  with missing-node dependency cycles during `Tree.Edit`.
- Twig still fails at step four with the same incremental digest. Its fresh
  tree now exactly matches locked C: its public-node difference count falls
  10 → 0. The unproven incremental reuse remains a pre-existing D8 failure.

The final error-mode token guard excludes six scanner contracts: comment, fish,
Go, Godot resource, Racket and Yuck. Its 412 deterministic route rows are
identical to those of the fleet-checked candidate.

No test was removed, disabled, or given a weaker threshold. These existing
fleet failures remain open; this change does not claim fleet graduation.

## Timing and memory

The randomized comparisons use `scripts/run_randomized_benchmarks.sh`,
20 paired shuffle seeds, alternating baseline/current order, one process per
seed, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`. The
LESS benchmarks parse fresh trees in Go and the locked C runtime. Clean
fixtures must parse every rule with an accepted, error-free root before timing.
The malformed baseline is allowed to time its known wrong nesting, making
the correctness tradeoff explicit. Ratios divide Go time by fresh C time.


The recovery-garbage KDL expectation changes from
`cbe533875bb790c77d7c7ae47d36b2d43205c3978846a98bd5d00463b45b1212`
to `5a7a058782904c979e04df75ec55d571c26d39317fb443e3f189998ce403e375`.
The same 15,718-byte input retains its full root span and accepted stop.
Public-node differences against fresh C fall 8,688 → 4,409. The expectation
refresh is a separate `update(pins)` change requiring owner approval; KDL
still has existing differences from C.

Two more recovery pins move in a separate `update(pins)` commit. AWK's locked
production witness keeps full 7,392-byte coverage and accepted completion;
its public-record difference count falls 9,960 → 9,311. Swift's existing
unsafe witness recovers the `@available` attribute at bytes 6536–6566 and the
function start at 6536, matching fresh C. The old function began at 6567.
Its record differences without ancestor paths fall 7,693 → 7,676. Its
path-aware count rises 37,965 → 37,968: the existing enclosing ERROR remains
wrong and child indexes shift. This directional tradeoff needs owner review;
Swift is not claimed to reach whole-file C parity. Both Swift digest pins
move together, while the locked-C digest stays fixed. See
[the additional pin receipt](issue-1336-recovery-pins.json).

The requested root `GOWORK=off go test ./...` ran in Docker and completed
with failures. It took 50m31s and reached 4,538,304 KiB maximum RSS. Four
generator failures reproduce on the baseline: the Swift rule-count pin,
two Markdown generated/bundled comparisons, and the nonterminal-alias
inventory. The generator package also hit its cumulative 35-minute timeout
while the Swift smoke test had run for 45 seconds; that unfinished coverage
remains unverified. No timeout or gate threshold was changed to obtain a pass.

The final root-package rerun passed every test except the 30ms injection
cache-timeout test. That test then passed five focused repeats on the final
tree and five on the baseline, without changing its threshold. The final
corpuscheck suite passes. The internal parsercorephase0 package passes after
a clean rebuild, including all tests; the earlier sweep's dependency plan
had become stale while the new recovery helper was added.

## Measured results

All 20 paired seeds completed for both suites. The primary Go trio has no
statistically significant time, byte or allocation change (`benchstat`, n=20).
Full parse is 14.53 → 14.11 ms, single-byte edit 301.7 → 301.4 µs, and
no-edit reparse 14.85 → 14.59 ns. No-edit remains 0 B/op and 0 allocs/op.

| Fresh LESS fixture | Go/C time before → after | Go ns/op before → after | Go B/op before → after | Go allocs/op before → after |
| --- | --- | --- | --- | --- |
| Malformed80 | 1.339 → 1.428 | 3834496 → 4306719 | 5672 → 1408 | 150 → 15 |
| Clean80 | 1.431 → 1.485 | 3634104 → 3671498 | 1240 → 1240 | 8 → 8 |
| Clean16384 | 1.540 → 1.578 | 815824282 → 851434039 | 8914136 → 8914136 | 10 → 10 |

For the 742,554-byte, 16,384-rule file, total-process maximum RSS is
140,904 → 140,804 KiB in Go and 48,748 → 48,644 KiB
in C: Go/C memory ratio 2.890 → 2.895.
Both runtimes accept every rule without errors. These RSS probes use separate
compiled processes with one warm-up and one measured parse; their single-run
times are not comparison evidence. C `B/op` excludes native C allocations.

[The benchmark receipt](issue-1336-benchmarks.json) includes every seed,
medians, Go/C ratios, allocations, bytes, revisions and RSS measurements.
The timing comparisons use the required randomized wrapper. To reproduce the
baseline, check out the pinned main revision and copy the new LESS benchmark
and its `issue1336LessSource` fixture helper there; both runtimes then parse
identical bytes. The baseline malformed tree is deliberately known to be
wrong, so its timing is not presented as a correct recovery result.

No PR was opened under the owner's autonomy rule; the lead integrates this
branch. The engine commit closes #1336 when merged. Counter and fixture pin
refreshes require owner approval. Existing fleet invariant failures,
generator failures and unfinished full-sweep coverage remain open.
