# `dispatch.julia` producer defect filing — trailing-comma assignment tuple — 2026-08-24

Status: **producer defect FILED before deletion** (reviewer FIX 1 on
`campaign/julia-decision.md`). This record preserves the locked-C-shape knowledge carried by
the `normalizeJuliaTrailingCommaAssignmentTuple` sub-repair **before** the licensed retirement
of the `dispatch.julia` arm deletes its producer, so the knowledge is not lost with the code.

Ledger terms used here follow the compatibility-tier vocabulary: **producer** (the compat
function that rewrites returned trees), **defect** (a filed divergence between shipped output
and the locked-C oracle), **witness** (an exact input pinning behavior), **census** (a counted
firing survey over a fixed corpus), **receipt** (a committed, re-runnable proof), and
**retirement** (registry `status: retired` with `retired_commit` + `receipt_refs`, never row
deletion — docs/compat-tier.md:77).

## 1. The producer

`normalizeJuliaTrailingCommaAssignmentTuple` (`parser_result_julia.go:13-56`) is the fourth
sub-repair chained by the `dispatch.julia` dispatcher arm
(`parser_result_compat.go:158-159`, registry id `dispatch.julia`,
`testdata/result_compat_ownership_v1.json`). It walks for `open_tuple` nodes whose last child
follows a one-byte `,` with a single `=` gap between them, then rebuilds the tail into
`ERROR(operator)` — an error parent holding an `operator` leaf — sets `hasError` on the tuple,
and clears the production ID.

## 2. The defect (locked-C vs raw-Go divergence)

Witness: `x, = 1\n` (7 bytes, corpus name `inline-trailing-comma-tuple`).

Locked C oracle (`tree-sitter-julia @ e0f9dcd180fdcfcfa8d79a3531e11d99e79321d3`,
`grammars/languages.lock:41`, via the `TestJuliaDecisionThreeWayDump` harness):

```
open_tuple [0:6] hasError=true cc=4
  identifier [0:1] "x"
  comma      [1:2] ","
  ERROR      [3:4] hasError=true cc=1     ← structured error parent
    operator [3:4] "="                    ← operator child inside the ERROR
  integer_literal [5:6] "1"
```

Raw Go (`ParseNoResultCompatibilityBenchmarkOnly`, measured 2026-08-24 on base `62fa03fe`):

```
open_tuple [0:6] err=true cc=4
  identifier [0:1] "x"
  , [1:2] ","
  ERROR [2:5] " = " err=true extra=true cc=0   ← bare extra leaf holding ' = '
  integer_literal [5:6] "1"
```

Production Go (arm live) is byte-identical to raw Go on this witness — full-tree dump digest
`sha256:f6780ebcc18b9638afb915c133bc5d627bff6abdbe0570d8e6c783035aa47d03` on both sides under
the `TestJuliaDecisionThreeWayDump` normalized dump scheme (the pre-deletion firing census in
§4 records the same tree as `sha256:eea43579…bf93` under its own `"X "` dump spacing).

The defect: the shipped (and raw) Go tree flattens the region after the comma into a single
bare extra `ERROR [2:5] " = "` leaf, while the locked C oracle keeps a narrow error over the
`=` itself with a named `operator` child and lets the tuple continue to the real
`integer_literal`. The sub-repair encodes exactly the C shape (`newLeafNodeInArena(...
operatorSym ...) wrapped in an error parent, `parser_result_julia.go:42-48`) — but it never
fires.

## 3. The dead guard (why the producer can never fire)

The repair locates the comma as `children[len(children)-2]` of the tuple and requires
`source[comma.startByte] == ','` (`parser_result_julia.go:33-37`). On the current grammar
revision, raw recovery already fills that slot with its own error node: the second-to-last
child of `open_tuple` is the bare `ERROR [2:5]` leaf, whose first byte is a space, so the guard
fails on every candidate and the walk returns without rewriting. A repair whose trigger shape
is consumed by the raw layer it was written to post-process is unreachable code; the census
receipt below confirms this at runtime (`rewritten=0` despite `run=1`).

## 4. Pre-deletion firing census (recorded BEFORE the retirement)

Corpus: 24 julia witnesses (7 targeted inline shapes incl. the defect witness, 15 additional
clean/error-bearing snippets, `julia_utils.jl` control, plus two extra recovery variants),
run under `GTS_DISPATCHER_CENSUS=1` reading the `dispatch.julia`
checked/run/visited/rewritten receipt per witness, base `62fa03fe`. Full line output:

```
FIRING CENSUS corpus_size=24 lang=julia arm=dispatch.julia (pre-deletion)
witness=inline-clean-program bytes=35 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-recovered-return-range bytes=38 status=ACTIVE checked=1 run=1 visited=23 rewritten=12 prod_eq_raw=false
witness=inline-bracket-comprehension bytes=16 status=INERT checked=1 run=1 visited=11 rewritten=0 prod_eq_raw=true
witness=inline-subscript-single-row bytes=17 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-macro-juxtaposition bytes=11 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-trailing-comma-tuple bytes=7 status=INERT checked=1 run=1 visited=6 rewritten=0 prod_eq_raw=true prod_digest=sha256:eea43579e576b5ee66b9380b37e25de790b83c2819cc91ab257a273bf6d1bf93
witness=inline-broken bytes=35 status=INERT checked=1 run=1 visited=16 rewritten=0 prod_eq_raw=true
witness=julia_utils.jl bytes=1089 status=INERT checked=1 run=1 visited=343 rewritten=0 prod_eq_raw=true
witness=inline-subscript-binary-index bytes=24 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-macro-string-arg bytes=10 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-bare-return-range bytes=32 status=ACTIVE checked=1 run=1 visited=17 rewritten=5 prod_eq_raw=false
witness=inline-clean-for-loop bytes=32 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-clean-struct bytes=24 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-unbalanced-paren bytes=4 status=INERT checked=1 run=1 visited=3 rewritten=0 prod_eq_raw=true
witness=inline-string-interpolation bytes=13 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-ternary bytes=19 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-chained-comparison bytes=16 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-while-break bytes=25 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-local-func bytes=15 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-comment-only bytes=17 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-try-catch bytes=43 status=NO-PASS-RECORD ... prod_eq_raw=true
witness=inline-nested-module-error bytes=22 status=INERT ... prod_eq_raw=true
witness=inline-trailing-comma-newline-rhs bytes=7 status=INERT ... prod_eq_raw=true
witness=inline-recovery-missing-end bytes=26 status=INERT ... prod_eq_raw=true
```

Reading: only the recovered-return-range sub-repair fires on this corpus (two ACTIVE
witnesses, both return-range shapes); the trailing-comma producer is INERT on its own targeted
witness because of the dead guard in §3. Post-retirement, production must equal raw on all 24
witnesses; the retirement regression test pins each witness's raw-side dump digest and asserts
production parity, so any future drift reopens loudly.

## 5. Retirement disposition and reopen condition

Disposition: the producer is deleted with the licensed `dispatch.julia` retirement
(`campaign/julia-decision.md`). Because raw Go still differs from the locked C oracle on the
defect witness even without the arm, the registry row carries
`route_coverage.c_oracle = "retired_known_divergence_receipt"` citing this filing, while
production/compact/forest/incremental carry exact raw-parity receipts.

Reopen condition — file a new producer (never silently resurrect deleted code) when ANY of:

1. A locked grammar revision (`grammars/languages.lock` bump for tree-sitter-julia) changes
   the raw/native recovery shape for `x, = 1\n` such that the pinned witness digest
   `sha256:f6780ebcc18b9638afb915c133bc5d627bff6abdbe0570d8e6c783035aa47d03` or the
   return-range raw==C digest
   `sha256:9e2de3670281c5a0013d55b570e6c175c7d7eb2d5142bca99276974ea61453e2`
   drifts in the retirement regression test;
2. A route receipt (production, compact-final-ref, forest fallback, incremental reuse, or the
   `TestJuliaDecisionThreeWayDump` C-oracle gate) fails on any of the 24 corpus witnesses;
3. A consumer requirement demands the C-oracle `ERROR(operator)` tuple shape in shipped Go
   output — in that case the repair must be re-filed as a new live producer entry with its own
   firing witness, firing census, and route receipts, and must be written against the raw
   shape actually produced by the grammar revision current at that time (post-processing the
   raw-filled `ERROR [n:m]` slot, not the historical comma slot).
