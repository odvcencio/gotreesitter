# `dispatch.julia` decisive experiment — locked-C oracle three-way dump — 2026-08-24

Status: **`GO`. `RETIREMENT LICENSED`: `dispatch.julia` is a live defect** — the locked C
oracle matches the **raw Go** parse exactly on the ACTIVE witness, while production Go
diverges. Retirement change plan drafted below.

Base: `6eed698a` ("Merge pull request #946 … c26ag-table-identity-guard"). Decision 0007
authority applied: strict locked-C parity decides.

## Step 1 — re-verification of the wave-2 probe on THIS base

Re-ran the wave-2 census probe (`campaign/julia-probe-context/dispatch_julia_census_test.go`,
one-line path fix after archival from `campaign/probes/julia/`: fixture path
`../../../testdata/...` → `../../testdata/...`; no parser or probe logic changed):

```
go test ./campaign/julia-probe-context/ -run TestDispatchJuliaCensusProbe -v -count=1   # PASS, exit=0
```

The ACTIVE witness reproduces exactly as recorded at `af9ded2b`:

| witness | bytes | raw root | prod root | checked/run | visited | rewritten |
|---|---|---|---|---|---|---|
| inline-recovered-return-range | 38 | source_file, hasError=false, 1 child | source_file, **hasError=true**, 1 child | 1/1 | 23 | **12** |
| julia_utils.jl (clean control) | 1089 | source_file, no err | identical | 1/1 | 343 | 0 |

Arm still exists as described: `parser_result_compat.go:158` `case "julia":` →
`normalizeJuliaCompatibility` (`parser_result_julia.go:3-11`). Witness source:
`function f()\n    return 1:(2 + 3)\nend\n` (38 bytes).

## Step 2 — locked-C oracle run (cgo harness, focused host run)

New throwaway diag following the Corn `zz_c_tree_dump_test.go` pattern:
`cgo_harness/zz_julia_decision_dump_test.go` (`TestJuliaDecisionThreeWayDump`, build tags
`cgo,treesitter_c_parity`, dumps C-oracle / raw-Go / production-Go trees for one env-gated
witness and emits machine-checkable `VERDUMP` equality lines).

```
cd cgo_harness && GTS_PARITY_ALLOW_HOST=1 \
  GTS_PARITY_C_REF_BUILD_CACHE=/home/draco/work/gts-xver-c-ref-cache \
  go test . -tags "cgo,treesitter_c_parity" -run TestJuliaDecisionThreeWayDump -v -count=1
# PASS, exit=0 (4.7s)
```

`GTS_PARITY_ALLOW_HOST=1` used for a focused host debug run, same precedent as the Corn
probe. Locked-C identity: `grammars/languages.lock:41` pins
`tree-sitter-julia @ e0f9dcd180fdcfcfa8d79a3531e11d99e79321d3`; transport
`cgo_parity_binding`, go-tree-sitter v0.25.0, runtime 0.25.1
(`f5afe475deb7c0bae6407fb776c76824f717bb61`), grammar `-O2` shared object dlopen'd via
`ParityCLanguage("julia")`.

## Step 3 — three-way comparison on the 38-byte witness

Machine verdict lines from the run:

```
root error flags: C=false rawGo=false prodGo=true
structural digests: C=sha256:9e2de367…53e2 rawGo=sha256:9e2de367…53e2 prodGo=sha256:41b97834…cd0d
VERDUMP C==rawGo: true
VERDUMP C==prodGo: false
```

Locked C oracle (and identically, raw Go) around the return:

```
source_file [0:38] cc=1
  function_definition [0:37] cc=4
    block [17:34] cc=1
      return_statement [17:33] cc=2            ← clean, error-free
        range_expression [24:33] cc=3 "1:(2 + 3)"
          integer_literal "1"; ":"; parenthesized_expression "(2 + 3)"
```

Production Go (arm live) at the same region:

```
    block [17:34] cc=2                          ← extra child synthesized
      return_statement [17:32] cc=2             ← span truncated vs C [17:33]
        binary_expression [24:32] cc=4          ← range_expression destroyed
          integer_literal "1"
          ERROR [25:28] named extra ":("        ← synthesized ERROR node
          operator "+"; integer_literal "3"
      ERROR [32:33] named extra ")"             ← synthesized ERROR root child
```

### Answers required by the acceptance criteria

**Does locked C set the error flag the arm's repair produces? NO.** The locked C oracle
ships a clean, error-free `source_file` whose `return_statement` keeps its full span
`[17:33]` with an intact `range_expression`. It does **not** match the arm's repaired,
error-flagged output. **It matches the raw Go tree byte-for-byte, node-for-node**
(identical normalized-dump SHA256). Production Go is the only divergent tree of the three.

## Verdict

Per Decision 0007 (strict locked-C parity is the authority): retiring the arm moves shipped
julia output **toward** the C oracle, not away from it. The `dispatch.julia` repair for
recovered return ranges is not a compatibility shim — it is a live defect that destroys a
structure the native parser and the locked C oracle agree on, truncates spans, injects
`ERROR` nodes into a clean parse, and flips the root error flag. **Retirement is licensed.**

(The wave-2 draft blocker receipt is superseded by this record; its reopening conditions 1–3
remain valid hygiene items for any retirement PR but are no longer decision-blocking.)

## Follow-on: retirement change plan (draft)

Scope of deletion (single dispatcher arm, five sub-repairs, one file + one switch arm):

1. **Delete producer**: remove `parser_result_julia.go` entirely
   (`normalizeJuliaCompatibility` + `normalizeJuliaRecoveredReturnRange`,
   `normalizeJuliaMacroArgumentJuxtaposition`, `normalizeJuliaSubscriptSingleRowMatrix`,
   `normalizeJuliaTrailingCommaAssignmentTuple`, `normalizeJuliaBracketForComprehensions`,
   `normalizeJuliaRecoveredSourceRoot` + helpers).
2. **Delete switch arm**: `case "julia":` in `parser_result_compat.go:158-159`.
   Audit `incremental_leaf_fastpath.go:146` which also carries a `case "julia":` — confirm
   whether it routes to the same compat tail and retire consistently.
3. **Registry**: set `dispatch.julia` status `"retired"` (or delete the row) in
   `testdata/result_compat_ownership_v1.json:1554-1579`; `TestResultCompatibilityOwnershipRegistry`
   must pass.
4. **Route receipts before merge** (registry retirement condition vocabulary):
   - production route: the witness set above must ship raw-equal trees (this dump is the
     prototype receipt);
   - compact route (no fallback), forest fallback, incremental reuse: re-run the witness
     through each route post-deletion and diff against the locked-C digest;
   - c_oracle route: keep `TestJuliaDecisionThreeWayDump` as a permanent parity gate
     (rename out of `zz_` on landing) asserting `C==rawGo && C==prodGo && !hasError`.
5. **Regression coverage**: add a julia retirement parity test modeled on
   `awk_dispatch_blocker_receipt_test.go` pinning the three digests above
   (`9e2de367…` C/raw, `41b97834…` pre-retirement production) so any future reintroduction
   of a julia rewrite fails loudly.
6. **Gates to run**: full julia corpus parity (`parity_top50_test.go` includes julia),
   tracked census (`GTS_DISPATCHER_CENSUS=1`) showing the `dispatch.julia` pass gone,
   `TestResultCompatibilityOwnershipRegistry`, baseline `parity_cgo_test.go`.

Risk assessment: low. On clean real input the arm is INERT (`julia_utils.jl`: rewritten=0 /
343 visited), so only recovered-range sources change output — and those change **toward**
locked-C parity. The four other sub-repairs were INERT on the wave-2 witness set and their
trigger shapes are grammar-revision-dependent; deleting them removes dead code paths, and
the permanent gate in step 4 catches any behavioral regression.

## Artifacts

- `campaign/julia-probe-context/dispatch_julia_census_test.go` — wave-2 probe (path fix only)
- `cgo_harness/zz_julia_decision_dump_test.go` — three-way dump diag (build-tag gated, env-gated)
- This record.
