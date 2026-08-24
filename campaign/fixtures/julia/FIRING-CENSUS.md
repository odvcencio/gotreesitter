# `dispatch.julia` firing-census receipt — corpus `campaign/fixtures/julia/`

Status: **receipt** (re-runnable proof; six-term vocabulary: producer / defect /
witness / census / receipt / retirement). This file records the firing census
for the retired `dispatch.julia` dispatcher arm over the committed regression
corpus shipped alongside it: the ACTIVE firing witness
(`inline-recovered-return-range.jl`) plus the full pre-deletion sweep
witnesses (24-witness canonical corpus incl. the `julia_utils.jl` control,
plus one extra recovery variant `inline-assignment-tuple-multi.jl` = 25 files).

## 1. Pre-deletion census (recorded BEFORE the deletion)

Run at base `62fa03fe` under `GTS_DISPATCHER_CENSUS=1`, reading the
`dispatch.julia` checked/run/visited/rewritten pass receipt per witness from
`tree.ParseRuntime().NormalizationPasses`. Executed **before** deletion commit
`2c5ddcc9a566d2df4eed8544df0a022636d9977b` removed the producer
(`parser_result_julia.go`) and the switch arm (`parser_result_compat.go:158-159`).
Full line output (also preserved in `campaign/julia-trailing-comma-defect.md` §4):

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

Census reading: exactly ONE sub-repair fires on this corpus —
`normalizeJuliaRecoveredReturnRange` on the two return-range witnesses (both
ACTIVE); the other four sub-repairs are unfired across all 24 inputs, and the
trailing-comma producer is INERT even on its own targeted witness because its
trigger slot is consumed by raw recovery (dead guard;
`campaign/julia-trailing-comma-defect.md` §3).

## 2. Post-retirement confirmation census

Gate: `go test . -run '^TestJuliaRetirementFiringCensusPostDeletion$' -v -count=1`
(`GTS_DISPATCHER_CENSUS=1` via `t.Setenv`). With the producer deleted, every
canonical witness reports `status=NO-PASS-RECORD`: no parse records a
`dispatch.julia` normalization pass, and any future pass record fails the test.
The companion gate `TestJuliaRetirementProductionEqualsRawOverCorpus` asserts
production==raw tree-digest parity on all 24 witnesses, and
`TestJuliaRetirementFixtureCorpusIsComplete` pins the shipped corpus itself
(24 census witnesses + 1 extra recovery variant) with production==raw parity
on each fixture file. Last verified 2026-08-24 at the registry
`retired_commit` `2c5ddcc9a566d2df4eed8544df0a022636d9977b`.

## 3. Reopen conditions

See `campaign/julia-trailing-comma-defect.md` §5. In brief: a grammar revision
that drifts either pinned raw digest (`sha256:9e2de367…53e2` firing witness,
`sha256:f6780ebc…7d03` defect witness), a route-receipt failure on any corpus
witness, or a consumer requirement demanding the C-oracle `ERROR(operator)`
tuple shape each reopens the retirement through a NEW producer filing with its
own firing witness, firing census, and route receipts — never by resurrecting
deleted code silently.
