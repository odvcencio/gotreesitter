# Bounded ASCII self-loops with a reserved-value fallback

Evidence date: 2026-10-16 UTC. Baseline:
`8701f7f91a1e9edb41cfb452b0ffc223b8d6ac7b`.
Qualified runtime: `f7ec67210486be4398d962fa0cf7e73b02da1c41`. All main merges remain held for 0.56.0.
This supplies a Go full-parse improvement; compact graduation stays gated.

The scalar DFA scanner repeats acceptance and priority work for every byte.
The internal/lexrun helper consumes explicit, non-SKIP ASCII transitions that
return to the current state, stopping at newlines, non-ASCII bytes and other
transitions. Same-state acceptances have the same priority and token start;
the last byte position dominates earlier positions. The scalar scanner records
that final acceptance and handles every other transition, EOF and error rule.
Columns advance by the same byte count. No cache, allocation or language-name
gate is added. The table builder and helper share NoMatch/SkipBit constants.

Buckley's bounded source review catches a reserved no-match value collision
in an earlier private helper. A regression fails there with three consumed
bytes and passes after the unsigned guard returns the original cursor for
negative/reserved states. The maximum non-sentinel state remains eligible.
Reserved-state handling falls back to the existing scanner; no parser-state
limit is narrowed. The completed read-only follow-up finds no concrete remaining
defect in the supplied complete scanner/helper/builder source. It does not
independently execute tests or approve a merge. An earlier structured review
launches broad host verification and times out at snapshot initialization;
its salvaged result is not approval or execution evidence. No matching process
remains. That workflow is excluded; all required checks use Docker.

## Counters and correctness before timing

All 412 ordinary ledger rows, canonical Go tree/Core.Work vectors and 21 raw
fields stay exact. The owned-YAML 484-node work guard, focused C cases and
Go/YAML 48-step edit sessions pass before timing, including zero no-edit
allocations. A frozen scalar oracle matches **81,144 complete token/Lexer
states**, covering priority, SKIP, immediate/zero-width behavior, EOF, invalid
UTF-8, Unicode, failed scans, read spans and position overflow.

The final standalone owned export uses a main-package initializer and proves
compact acceptance with **484→484 full and first-edit nodes**. Both owned rows
match exactly, completing **414 valid exact rows**. Earlier extra exports used
a test-only initializer that the standalone command did not execute, so those
two exports duplicated ordinary routing. They are excluded; the main-package
export corrects collection only, after timing and before publication. The
pre-timing owned work guard and actual owned benchmarks/C/edit sessions remain
valid. No runtime input, ledger ratchet or exception changes.

Representative before→after rows (tokens / new nodes / maximum live versions):

| R4 path | Tokens | Nodes | Maximum live versions | Reused bytes / block splices |
| --- | ---: | ---: | ---: | ---: |
| Go candidate full | 589→589 | 1,251→1,251 | 3→3 | 0→0 / 0→0 |
| Go candidate first edit | 374→374 | 1,553→1,553 | 8→8 | 1,784→1,784 / 266→266 |
| Ordinary YAML default full | 107→107 | 288→288 | 1→1 | 0→0 / 0→0 |
| Ordinary YAML default first edit | 107→107 | 480→480 | 1→1 | 0→0 / 0→0 |
| Owned YAML candidate full | 102→102 | 484→484 | 2→2 | 0→0 / 0→0 |
| Owned YAML candidate first edit | 102→102 | 484→484 | 0→0 | 0→0 / 0→0 |

Ordinary YAML retains its existing compact decline and default fallback.
Canonical Go lookup proxy stays **7,553 / 37,640 / 37,479 / 397,227**; all
other raw fields stay exact. No owner counter exception or reset is requested.

## Twenty-pair final-source measurement

The repository runner uses 20 alternating shuffled pairs per language:
Go **24301–24320**, YAML **24401–24420**; GOMAXPROCS=1, count 1, 750 ms,
benchmem, one process per role/seed, CPU 6, 6 GiB Docker and both locks.
Sources, data, orders, samples and timed completion/work columns verify.

| Compact Go file | Before → after | Time change | p |
| --- | ---: | ---: | ---: |
| Rewrite | 1.771060 → 1.757717 ms | −0.75% | .021 |
| Query | 8.5031255 → 8.479957 ms | −0.27% | .429 |
| Language | 8.857246 → 8.689026 ms | −1.90% | <.001 |
| Large | 90.702975 → 89.8359145 ms | −0.96% | .114 |

Rewrite and language improve significantly. Query and large are inconclusive.

| Other path | Before → after | Time change | p |
| --- | ---: | ---: | ---: |
| Default Go full | 3.8646345 → 3.8385265 ms | −0.68% | .001 |
| Default Go single-byte edit | 98.7625 → 99.1265 µs | +0.37% | .183 |
| Default Go no-edit | 3.136 → 3.0785 ns | −1.83% | <.001 |
| Ordinary YAML attempt/fallback | 192.9605 → 192.1735 µs | −0.41% | .056 |
| Owned YAML R4 | 349.4220 → 347.4875 µs | −0.55% | .277 |
| Owned YAML 8 KiB | 1.3610455 → 1.3546400 ms | −0.47% | .183 |

Default full improves significantly. The edit increase and all YAML changes
are inconclusive; every directional change stays recorded. Control movements
do not establish an intrinsic speed change in a bypassed path. All allocated
byte and object medians stay exact. Compact Go objects remain **21 / 129 / 243 /
1,716**, bytes **1,776 / 138,725 / 284,629 / 3,251,743**. YAML objects stay
**37 / 109 / 336**, bytes **16,760 / 12,872 / 44,313**. No-edit remains
**0 B/op / 0 allocations**. No YAML speed improvement is claimed.

A copied manifest hash accidentally underwent the shuffle-seed substitution
24101→24301. The original pre-existing manifest verifies all **3,509 native
inputs and 296 data inputs** unchanged. Only that metadata hash and its
manifest-entry digest are corrected; old metadata/failure are archived. Runtime,
tests, timing scripts and benchmark inputs do not change, and timing is not
repeated for that collection typo.

## Final-source qualification

All **206 invariant sessions plus owned YAML**, **618 C fixture checks**,
focused Go/YAML C cases, core races/phase-zero-A/EOF shadow, root lexer/ownership
races, 386 tests and memory-budget checks pass. The strict 204-file census stays
**159 PASS / 45 known FAIL**, with all **120 compact acceptances C-exact** on
every checked structural axis. Existing failures are preserved, not waived.

The 1,049,572-byte, 524-document owned YAML Go+C probe accepts without fallback,
covers the input and matches the pinned C structure/digest. It peaks at
**153,184 KiB RSS**, below 400 bytes/source byte. Canonical large Go peaks at
**122,156 KiB**. These are single safety probes, not statistical RSS wins.
No OOM or timeout occurs in the required container gates. Post-copy source
hashes, helper and selected-one-pass core races, canonical/lifecycle checks,
386 library build, API/layout guards and CI coverage pass: **756/756 root Go
files, 12 API tag sets, 454 language-name uses, 101 environment reads, 64 CI
selectors in seven workflows and 7,693 defined functions**. CI's support lane
includes the new package. No admission allowlist or global default changes.

The first private comparison (`20261016-ascii-self-loop`, base86879374e) stops
its full gate at a pre-existing generator 386 compile error. Separate Buckley
commit **8701f7f9** fixes the `int(MaxUint32)` overflow using a uint64 comparison,
preserving the exact 4,294,967,295-state limit. Existing generator tests pass on
amd64/386 and the root 386 failure reproduces before the fix and passes after.
The second private comparison (`20261016-ascii-self-loop-portable`) passes its
full gates but remains uncommitted after the source review finds the helper's
sentinel edge. Those earlier timings do not qualify this final fixed source,
and their percentages are not added to these results.

Frozen source, scalar oracle, sentinel regression, corrected counter export,
source-only reviews and limits, shuffled samples, benchstat, strict/fleet/RSS,
post-copy and promotion audits remain in local
`gotreesitter-evidence/20261016-ascii-self-loop-sentinel`. Native lexer/language/
CI bytes are preserved for reproduction. `/baseline` and `/workspace` bind the
same physical worktree; after promotion use a native checkout or restore both
lexer.go and language.go in the baseline overlay. Do not silently compare the
promoted source with itself. The continuous graduation goal remains active;
all main merges stay held for 0.56.0.
