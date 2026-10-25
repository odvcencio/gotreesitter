# Go graduation: recovery correctness and current timing gaps

The remaining reduced assignment-prefix mismatch is fixed by certifying Go's existing missing-version scheduling protocol for the exact current blob. On `= a + b`, Go now lets the absorbing version compete before an eagerly shifted missing-target version; the cheaper skipped-equals ERROR matches C. The grammar blob stays `214322327c9ee7dda29febf32e415f7ed9e00a90b7690d3c32522e393211b8da`.

This bucket remains held for the [R3 counter exception](counter-review.md), broader final-source qualification, and the separate graduation gates. All merges to main remain held. No allowlist, global default, fresh-result verifier or V4 reuse activation changes.

## Distinguishing correctness evidence

The original 49-byte `= a + b` witness fails on both baseline routes. A private probe tests the existing missing-turn, full-order and combined protocols independently; missing turns alone repairs the result. The permanent prefix regression now includes that case.

A new 138,608-byte assignment witness, derived from the public Cognociente source by changing `x := a + b` to `x = a + b`, has SHA-256 `8dc8aad35490cffcb042bf3a79dc75d3fdd63be87fc92c638ea36351c57fce4d`. Its 16 sites cover the whole document. Insert, replace and delete at every site pass on both routes: **96 cases**, each comparing fresh Go/fresh C, incremental Go/fresh C and incremental Go/incremental C, including error children, fields, spans, points and flags. This extends the earlier single-site repair. The original declaration witness also keeps its 12 passing cases and identical complete-operation counts.

Both 72-step Go invariant sessions pass with zero no-edit allocations. Twenty memory/lifetime cases, native public-view/publication checks, the sealed Go trace/work gate, exact artifact grant controls and focused races pass. The new grant rejects mismatched artifacts and custom language names. The standard Go R4 default/candidate ledgers are byte-identical, including reuse and fallback reasons.

The 96-case log predates only the added SHA-only fixture guard and fresh-route/hash logging; its logged source hash is exactly the guarded hash. A broader final-source Go C run, including that guard, was deferred before measurement when current-head CI exposed correctness blockers. The initial broad selector matched fewer test families than intended, so its result is not claimed as complete Go-suite qualification. The retained command and logs make the actual scope visible.

The C oracle for these results is still runtime **0.25.1**, commit `f5afe475deb7c0bae6407fb776c76824f717bb61`, and the pinned Go grammar commit `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`. No C 0.27.x qualification is claimed.

## Counter exception and discarded alternative

The standard R4 ledger and every complete count for the original declaration witness remain unchanged. Correct assignment recovery keeps competing versions alive longer. On the large assignment delete, public tokens rise **3,572→3,832 / 3,585→3,850**, new nodes **7,210→8,652 / 7,175→8,641**, and maximum stacks **2→4** on each route. Full merge attempts rise **15,840→16,527** on each route; edit merge attempts rise **15,846→17,015 / 15,843→17,015**. Smaller reuse declines and all diagnostic storage/GSS fields remain explicit in [the public-profile delta](public-profile-changes.json).

[Every complete event change](complete-work-changes.json) and [the review table](counter-review.md) distinguish full operations from public profile fields. No ledger row is reset. R3 owner approval is required for these correctness costs; it is not supplied by this receipt or by earlier Liquid/YAML approvals.

The full C version-order alternative also passes the 96-case matrix and the Go suites selected by its command, but is rejected before timing: candidate R4 edit nodes rise **1,553→1,706**, while the report delete's stack-version creations rise **10,635→232,371**. [Its complete event changes](discarded-version-order.json) and exact source overlay are retained. It is not activated.

## Canonical parity gate repair

The existing canonical leaf control failed before this change because it required a compact rebuild, but the authenticated leaf path already returns zero rebuilt nodes. Its result was `unclassified` despite one dependency check and zero reparse work. The harness now has a separate `token_invariant_leaf_validation` classification requiring the positive dependency-check counter, one validated token, whole-syntax reuse, no rebuilt nodes, no recovery/retry and an accepted result. The manifest changes only that mechanism label; expected source bytes and parity outcomes stay strict.

Positive and negative classification controls pass, including missing proof, rebuild work, recovery, unsupported reuse and truncated results. All six Go canonical cases pass; the separate Python scanner case also passes in its own process. Fresh/incremental Go and C comparisons remain intact. The receipt now retains the dependency-check counter. Earlier invalid output-path and route attempts are preserved as failed diagnostics.

## Forty paired timing comparisons

Twenty alternating baseline/candidate pairs run on each route using the original primary Go benchmark functions in an identical explicit seeded order, count 1, 750 ms, benchmem and GOMAXPROCS=1. The baseline differs only by the old Go runtime profile; the mounts share the canonical checkout and per-file overlays define the comparison. Host: Intel Core Ultra 9 285, Linux amd64, pinned Go 1.26.6.

| Route / operation | Before | After | Result |
|---|---:|---:|---|
| Default full | 6.556 ms | 6.455 ms | inconclusive, p=.565 |
| Default edit | 100.58 µs | 99.72 µs | inconclusive, p=.565 |
| Default no edit | 4.835 ns | 4.936 ns | inconclusive, p=.995 |
| Candidate full | 7.563 ms | 7.569 ms | inconclusive, p=.925 |
| Candidate edit | 70.77 µs | 70.41 µs | inconclusive, p=.383 |
| Candidate no edit | 3.297 ns | 3.272 ns | inconclusive, p=.218 |

Every allocation median is unchanged: default **6/5/0**, candidate **22/5/0**. All byte differences are inconclusive. No throughput win is claimed for the correctness repair. These runs compare the repair within each route; their separate route blocks are not the paired compact-versus-legacy graduation comparison.

## Current compact-versus-legacy census

The separate frozen census uses all **206** pinned R4 sample identities, one grammar per process, 20 explicit seeds, 750 ms, count 1, shuffled legacy/compact child order and unchanged randomized runner. It includes declined work and real token-source factories. CSV and Enforce are read from existing local checkouts after commit, source and license-evidence authentication; their payloads are not redistributed. Each process has a wall bound and every failure/timeout remains recorded. Compiled sources, test overlay, runtime inputs, toolchain, wrapper, runner and binary are authenticated before running, so later edits cannot change the measured executable.

The first two completed 20-pair results are:

| R4 language | Legacy median | Compact median | Median paired ratio | Compact routes/fallbacks |
|---|---:|---:|---:|---:|
| Go | 664.65 µs | 783.93 µs | **1.1720** | 1 / 0 |
| YAML | 136.89 µs | 197.49 µs | **1.4317** | 1 / 0 |

Both inputs execute compact successfully. These are current-runtime measurements, not an authenticated frozen M1-release comparison. The fleet is incomplete at this receipt's capture, and these ratios already exceed the E-A worst-file limit of 1.15. [The snapshot](current-route-snapshot.json) preserves medians, route counts, allocation costs and raw hashes. The census was deliberately deferred after 139 completed languages when current-head CI exposed correctness blockers. Its completed samples, interrupted Org sample and frozen executable are preserved; the unfinished census is not graduation evidence.

The next throughput experiment extends the owned core loop across token elections. The existing corridor VM already loops across elections without returning to the general scheduler; the experiment removes per-token VM/core entry and repeated value adaptation while retaining the existing election. The prior profile attributes 17.39% Go / 31.08% YAML cumulative CPU to token election, including lexer work. Those shares are not the cost removed by the loop experiment or evidence of a speedup. YAML checkpoint/publication costs remain separate targets. The optional default-route V4 experiment does not improve ordinary candidate-route work, so it stays deferred from the graduation path.

## Graduation remains unproven

The owner design requires the all-206 E-A distribution before language graduation, the frozen M1 legacy baseline, a pinned C 0.27.x oracle and regenerated boards, generated 32 KiB/137 KiB/1 MiB timing, real files, 1 MiB RSS at or below legacy, cliff/budget/race checks, reuse and query/highlight/tag parity. The current 6.0 sanity threshold still skips declines and is not that gate. No one-language receipt can substitute for the missing fleet gate.

Upstream **0.27.1** now resolves to immutable commit `10eb5ac119eec9cb4267a8e035066473983f5f3a`, verified against its Cargo version and ABI15/min13 headers. A private default-API compatibility preflight is prepared and deferred; deprecated timeout/cancellation APIs explicitly fail closed in that prototype, so it is not full binding compatibility. The harness pin has not changed. Native platform coverage and the remaining full graduation requirements are also outstanding.

The archive contains completed raw correctness/work/performance logs, failed controls, source overlays and census controls, plus a frozen Go/YAML snapshot. It excludes executables, the private CSV/Enforce payloads and the unfinished fleet's mutable outputs. Commands use the canonical checkout read-only, the shared runtime caches and the shared Docker workload lock. Source manifests and the archive member manifest bind the evidence to its tested inputs and state its limits.
