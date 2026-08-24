# `dispatch.julia` cheapest-probe verdict — 2026-08-24

Status: **`NO-GO`. `KEEP LIVE`: `dispatch.julia`.** (blocked — draft blocker receipt below)

## Arm and producer (source evidence)

- Switch arm: `case "julia":` → `dispatcherArmCensus(ctx, "dispatch.julia", func() { normalizeJuliaCompatibility(ctx.root, ctx.source, ctx.lang) })` — `parser_result_compat.go:158-159`.
- Producer: `normalizeJuliaCompatibility` — `parser_result_julia.go:5-12`. It chains five sub-repair walks, all gated on `lang.Name != "julia"`:
  1. `normalizeJuliaRecoveredReturnRange` (`parser_result_julia.go`) — rewrites recovered `return a:(b + c)` ranges into quote/binary shapes, inserts extra `ERROR` nodes, and **sets** the root error bit (`root.setHasError(true)`).
  2. `normalizeJuliaMacroArgumentJuxtaposition` — fuses macro-argument integer/binary and binary/string pairs into `juxtaposition_expression` wrappers.
  3. `normalizeJuliaSubscriptSingleRowMatrix` — retags single-row `matrix_expression` subscripts to `vector_expression`.
  4. `normalizeJuliaTrailingCommaAssignmentTuple` — rebuilds `x, = rhs` tuples with a synthesized `ERROR(operator)` child and sets `tuple.setHasError(true)`.
  5. `normalizeJuliaBracketForComprehensions` — retags `[expr for it in coll]` matrices to `comprehension_expression`; when it fires, `normalizeJuliaRecoveredSourceRoot` additionally retags a full-span `ERROR` root to `source_file`.
- Registry entry: id `dispatch.julia`, kind `dispatcher_arm`, functions `[normalizeJuliaCompatibility]`, files `[parser_result_julia.go]`, languages `["julia"]`, owner `scheduler_action_semantics`, witnesses `["cgo_harness/parity_cgo_test.go"]`, status `"live"`, `evidence_scope: baseline_corpus_wide_only` (`testdata/result_compat_ownership_v1.json:1554-1579`). Retirement condition (`:1570`): delete only after scheduler emits the same tree natively for every registered witness with exact production, compact, forest, incremental, and C-oracle route receipts.
- Live inventory row 16 records "`none`" registered local repairs at arm level and "**No dedicated receipt.**" (`campaign/a1-live-arm-inventory.md:72`); retirement queue row 23 lists "`dispatch.julia` | No receipt; baseline coverage only" (`campaign/a2-retirement-queue.md:77`). This receipt closes that gap.

## Census probe (what ran)

Smallest census = shipped dispatcher-census machinery applied to julia only: new external test
`campaign/probes/julia/dispatch_julia_census_test.go` (package `juliaprobe`), which parses eight
julia witnesses (one repo-shipped `.jl` file plus seven inline sources, each targeting one
registered sub-repair) with the production loader under `t.Setenv("GTS_DISPATCHER_CENSUS", "1")`
and reads the `dispatch.julia` pass from `tree.ParseRuntime().NormalizationPasses` (same read path
as `campaign/probes/yaml/dispatch_yaml_census_test.go`). It also prints raw
(`ParseNoResultCompatibilityBenchmarkOnly`) vs production root shape per witness.

Command and full output:

```
go test ./campaign/probes/julia/ -run TestDispatchJuliaCensusProbe -v -count=1   # PASS, exit=0
```

| witness | bytes | raw root | prod root | checked/run | visited | rewritten |
|---|---|---|---|---|---|---|
| inline-clean-program | 35 | source_file, no err, 1 child | identical | 0/0 | 0 | 0 |
| inline-recovered-return-range | 38 | source_file, no err, 1 child | source_file, **hasError=true** | 1/1 | 23 | **12** |
| inline-bracket-comprehension | 16 | source_file, no err, 1 child | identical | 1/1 | 11 | 0 |
| inline-subscript-single-row | 17 | source_file, no err, 2 children | identical | 0/0 | 0 | 0 |
| inline-macro-juxtaposition | 11 | source_file, no err, 1 child | identical | 0/0 | 0 | 0 |
| inline-trailing-comma-tuple | 7 | source_file, hasError=true, 1 child | identical | 1/1 | 6 | 0 |
| inline-broken | 35 | ERROR, hasError=true, 8 children | identical | 1/1 | 16 | 0 |
| ../../../testdata/compact_selected_lineage/julia_utils.jl | 1089 | source_file, no err, 5 children | identical | 1/1 | 343 | 0 |

Reading per `parser_result_compat.go:302-315`: census = fingerprint diff before/after the
arm; `rewritten > 0` ⇒ **ACTIVE**, `rewritten == 0` with a run record ⇒ INERT.

## Findings

1. The arm is not inert. On `inline-recovered-return-range` the arm rewrote 12 of 23 visited
   nodes and flipped the tree from a clean raw parse (`source_file`, no error) to a shipped tree
   carrying `hasError=true`. This is exactly the registered repair in
   `normalizeJuliaRecoveredReturnRange` / `juliaRewriteReturnRange`, which synthesizes
   `errorSymbol` nodes and calls `root.setHasError(true)` — i.e. the Go-only compatibility layer
   *introduces* an error flag the native parse does not have.
2. Unlike yaml/cobol, only one of the five sub-repairs fired on this witness set. The other four
   (macro juxtaposition, subscript matrix, trailing-comma tuple, bracket comprehension) were INERT
   here: either their trigger shapes were not produced by the current grammar revision (three
   witnesses recorded no pass record at all — checked=0/run=0), or the raw parse already produced
   the repaired shape. Their liveness is unproven either way; the probe makes no claim beyond its
   own receipts.
3. On well-formed real input the arm is INERT (`julia_utils.jl`: rewritten=0 over 343 visited),
   so a clean-input-only corpus would give a false INERT receipt; error-bearing/recovered-root
   witnesses are what expose this arm.
4. Coverage gaps that block any retirement decision:
   - `testdata/dispatcher_census_a0/` has **no `julia/` directory** and
     `testdata/dispatcher_census_a0_manifest_v1.json` contains no julia fixture (walk shows zero
     julia entries); the tracked census denominator excludes julia entirely.
   - The registry's only registered witness is `cgo_harness/parity_cgo_test.go`
     (`testdata/result_compat_ownership_v1.json:1567-1569`) — baseline corpus-wide parity,
     `evidence_scope: baseline_corpus_wide_only` (`:1579`), not per-route receipts.
   - `cgo_harness/corpus_real` has no `julia` directory, so the shipped real-corpus census gate
     cannot cover julia.
   - No C-oracle digest comparison was performed: this probe is Go-internal only.

## Draft blocker receipt (exact divergence to close)

**Divergence:** for recovered `return a:(b + c)` sources the Go production tree differs
structurally from the raw parse the arm edits: the raw parse ships a clean, error-free
`source_file`, while production ships a tree whose `return_statement`/range region is rebuilt into
quote/binary/error-node shapes and whose root carries `hasError=true`
(witness `inline-recovered-return-range` above: raw no-error vs prod error, 12 of 23 nodes
rewritten). Retiring the arm changes shipped output for every such source unless C parity proves
the C oracle natively produces the same repaired, error-flagged shape — which is unlikely given
the flag is set by Go-only code (`parser_result_julia.go`, `normalizeJuliaRecoveredReturnRange`,
`normalizeJuliaTrailingCommaAssignmentTuple`).

Reopening conditions (mirroring the shipped receipts' checklist):

1. Add authenticated A0 julia witnesses (small/medium/large, including a recovered
   return-range source that triggers the live repair and witnesses targeting each of the five
   sub-repairs) under `testdata/dispatcher_census_a0/julia/` with manifest entries and locked
   SHA256s.
2. Rerun the census at a locked grammar revision with per-witness checked/run/visited/rewritten
   receipts committed to `testdata/dispatcher_census_tracked_v1.json`.
3. Determine whether the four inert-on-probe sub-repairs are still reachable under the current
   grammar revision; retire or re-trigger each individually with focused witnesses.
4. Produce C-oracle route parity for every witness: raw, production, compact (no fallback),
   forest, and incremental reuse digests matching the locked C output — including the
   error-bit-flip divergence, which must be shown to exist (or not) in the C oracle.
5. Reconcile the three no-pass-record routes observed on tiny inputs so the census denominator is
   complete.
6. Only then rerun the tracked-census gates and file the retirement PR.

No parser code or registry field was modified by this probe (Rule 2); all additions live under
`campaign/probes/julia/`.
