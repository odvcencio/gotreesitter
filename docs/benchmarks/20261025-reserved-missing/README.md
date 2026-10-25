# Reserved words and retained missing-token errors

The Go grammar now retains the pinned upstream grammar's 25 globally reserved words. Hidden missing-token errors also survive reduction and public-tree projection. These changes repair the frozen Go `}func` boundary, a C first edit, and EDS section error flags. They do not graduate a language.

The source starts at `eff701aa3962f88fbf6e8cd3b5994d7dca1d0a32`. Go's blob changes from `214322327c9ee7dda29febf32e415f7ed9e00a90b7690d3c32522e393211b8da` (51,685 bytes) to `9ad1f8d7d0f2b9aa32c61bd8295b56177caabb0ae2fb8dd4617559ee0f71fed4` (52,039 bytes). The upstream grammar remains commit `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`; its authenticated JSON SHA is `0dcc665aa521a1b73a200be13b4e376780cca037d764a165f12864babfe28000`.

## What changed

The JSON importer previously discarded global reserved sets without explicit context wrappers. The built-in Go definition also omitted that set. Assembly now preserves global reservations in non-error states without ordinary word actions, and the runtime keyword-state prefilter recognizes their valid keyword-DFA members. State zero, malformed dimensions, context-only sets, ordinary identifiers and prefix identifiers retain negative controls.

Creating a genuine MISSING token enables retained-history checks. Go's compatibility projection preserves the error when it hides that missing separator. Reconciliation returns the successful raw-only missing proof through the existing walk; the existing hidden-missing tree summary prevents an unproven clean forest rescue. No Node or Tree fields, extra raw-tree walk, cost ordering or language-name engine condition were added.

The new Go artifact requalifies the existing publisher and fixture identities. Its measured C4 table receipt changes only the blob hash and `expect_kw_states`, **0→1506**. The remaining table-shape figures are unchanged.

The existing missing-version scheduling grant is contained for included-range requests on this exact Go artifact through a grammar-owned scanner hook. Ordinary Go, SQL, unknown identities and older identities retain their prior policy. The range snapshots and existing digest pass again, but all eight complete range/C comparisons remain red. Their failures are retained; E-D2 remains open and compact range routing remains closed.

## Correctness evidence

The 66,634-byte original frozen Go witness and its 31-byte reduction now match locked C on both routes, including symbols, fields, spans, points, missing/extra/error flags and deep digests. The separated controls also match. Two delete/restore cycles on each route preserve those comparisons and zero-allocation no-edit reparses. Malformed fresh parses can still fall back: the owned recovery loop requires EOF after its first absorb. Clean fresh controls complete on compact. These are selected-result correctness checks, not proof of native non-EOF recovery.

The broader Go suite includes all 96 assignment-target edits at 16 sites per class and both routes. Across the fleet, **412 × 72-step sessions** pass on 206 grammars and both routes, with **29,664 edit steps**, zero no-edit allocations, and no skipped sessions. All **618 C fixture checks** pass, without skips. Focused memory, lifetime, race, public-view, immutable legacy-golden and query-work gates pass. Saved source passes the final focused Docker gate without overlays. That run passes the focused C4 receipt but skips the opt-in fleet test; a separate authenticated run explicitly enables and passes the 206-grammar C4 fleet receipt. The four permanent C/EDS regressions run separately, one grammar and test per process.

The C first edit previously returned 21 root children against C's 14; the new fresh and incremental results match C exactly. EDS previously suppressed `section.HasError()` on both its original source and first edit; those results now match fresh and incremental C, including their deep digests. Counter reproduction is checked before any tree traversal, C setup or additional Go parse can affect the measured edit.

The oracle is still **C 0.25.1**, commit `f5afe475deb7c0bae6407fb776c76824f717bb61`. There is no C 0.27.x or ARM64 qualification claim.

## Deterministic work and required review

The Go R4 default/candidate ledgers and all **24 complete Go operation records** are identical before and after. Of the **412 fleet route/fixture rows**, 410 are identical. C and EDS default first edits change; every delta is in [counter-deltas.json](counter-deltas.json).

C's edited node counter falls **1274→125** and tokens **320→16**, while EDS requires additional work to preserve C-correct errors. Four guarded changes need explicit owner approval: C block splices **148→24**; EDS nodes **4806→4930**, maximum versions **1→2**, and reused bytes **9873→9664**. [The review](counter-review.md) records the full trade and the unapproved state. No ledger rows or thresholds are reset.

A narrower private alternative enables retention only during selected Go publication. It passes the Go tests and leaves all 412 rows unchanged, but preserves the now-proven C and EDS defects. It is retained as a rejected experiment. Three repeated controls per variant attribute both non-Go row changes to genuine-MISSING creation; removing only the new summary output does not remove them.

## Paired timing

All comparisons use the unchanged randomized runner, 20 explicit seeds, identical seeded child order, alternating baseline/candidate process order, count 1, 750 ms, benchmem, one CPU and GOMAXPROCS=1. The Go primary suite calls the unchanged benchmark trio. These Linux amd64 runs use pinned Go 1.26.6.

| Route / primary operation | Before | After | Result |
|---|---:|---:|---|
| Default full | 4.017 ms | 3.897 ms | −2.98%, p<.001 |
| Default edit | 64.88 µs | 64.71 µs | inconclusive, p=.640 |
| Default no edit | 3.221 ns | 3.128 ns | −2.89%, p<.001 |
| Candidate full | 7.051 ms | 7.119 ms | inconclusive, p=.102 |
| Candidate edit | 67.36 µs | 67.35 µs | inconclusive, p=.764 |
| Candidate no edit | 3.236 ns | 3.124 ns | −3.45%, p<.001 |

Allocation medians remain **6/5/0** on default and **22/5/0** on candidate. The small default byte difference and all raw distributions remain in the archive. These results do not establish a compact throughput improvement.

| R4 route | Before | After | Result |
|---|---:|---:|---|
| Go compact | 753.7 µs | 754.1 µs | inconclusive, p=.841 |
| Go legacy | 646.0 µs | 644.1 µs | inconclusive, p=.383 |
| YAML compact | 224.7 µs | 226.3 µs | inconclusive, p=.841 |
| YAML legacy | 153.8 µs | 158.8 µs | inconclusive, p=.698 |

Every R4 compact sample reports one completed compact route and zero fallbacks. Allocation counts and byte medians are unchanged. These compare the current runtime, not a frozen M1 release. Paired compact/legacy median ratios are in [verification.json](verification.json); the archive retains all per-seed raw distributions. Go and YAML remain above E-A's 1.15 worst-file threshold.

## Scope and reproduction

[The evidence archive](evidence.tar.gz), [its checksum](evidence.sha256), and [the verification summary](verification.json) retain exact source maps, Docker commands, controls, raw logs, counter records and timing distributions. Executables, caches and private CSV/Enforce payloads are excluded. The archive also retains the failed receipt-read, measured-keyword-count and missing-manifest attempts. Completed primary samples were preserved when the separate R4 block initially failed; the corrected R4 run authenticates the same sources and controls before and after.

No allowlist, global default, fresh-result verifier, V4 reuse activation or main merge changes. The all-206 E-A exit gate, frozen M1 baseline, upgraded oracle, remaining native recovery and full per-language graduation requirements still block graduation. The route sanity test still permits `maxRatio=6.0`; graduation also requires its prescribed reduction to 1.15 after the gate passes. All merges to main remain held. The earlier Go assignment counter exception remains separately unapproved.
