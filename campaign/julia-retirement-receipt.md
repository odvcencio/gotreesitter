# `dispatch.julia` retirement execution receipt — 2026-08-24

Status: **receipt** (committed, re-runnable proof). Six-term vocabulary
throughout: **producer** / **defect** / **witness** / **census** / **receipt** /
**retirement** (registry `status: retired` with `retired_commit` +
`receipt_refs`, never row deletion — docs/compat-tier.md:77).

Authority: Decision 0007 strict locked-C parity; decision record at
`campaign/julia-decision.md` (`GO`, `RETIREMENT LICENSED`); producer-defect
filing filed BEFORE deletion at `campaign/julia-trailing-comma-defect.md`.

## 1. What was executed

1. **Producer deleted**: `parser_result_julia.go` removed in full —
   `normalizeJuliaCompatibility` and its five sub-repairs
   (`normalizeJuliaRecoveredReturnRange`,
   `normalizeJuliaMacroArgumentJuxtaposition`,
   `normalizeJuliaSubscriptSingleRowMatrix`,
   `normalizeJuliaTrailingCommaAssignmentTuple`,
   `normalizeJuliaBracketForComprehensions`) plus helpers.
2. **Switch arm deleted**: `case "julia":` removed from
   `parser_result_compat.go` (was lines 158-159).
3. **Fastpath KEPT**: `incremental_leaf_fastpath.go:146`
   `case "julia": return node.Type(p.language) == "line_comment" &&
   hashLineCommentTextInvariantEdit(...)` is untouched — it is an incremental
   line-comment reuse predicate unrelated to the compat arm (Decision 0007
   constraint); retiring it would break julia line-comment reuse.
4. **Registry flipped, never deleted**: row `dispatch.julia`
   (`testdata/result_compat_ownership_v1.json:1554-1585`) set to
   `status: "retired"` with all five route receipts
   (`production`/`compact`/`forest`/`incremental` =
   `retired_exact_receipt`; `c_oracle` = `retired_known_divergence_receipt`),
   `receipt_refs`, and `retired_commit`.
5. **Denominator arithmetic updated everywhere stated**
   (`docs/compat-tier.md:19-27`): dispatcher_arms 31→30,
   dispatcher_languages 33→32, live_entries 32→31, retired_entries 56→57,
   live language labels 35→34 ("name 34 language labels" at line 27).
6. **Defect filed before deletion**: raw-vs-C divergence on the trailing-comma
   witness recorded at `campaign/julia-trailing-comma-defect.md` (C:
   `open_tuple` with structured `ERROR(operator)` child, `hasError=true`;
   raw Go: bare extra `ERROR [2:5] " = "` leaf) so the C-shape knowledge
   survives the code deletion.
7. **Firing census run BEFORE deletion**: output preserved verbatim in
   `campaign/fixtures/julia/FIRING-CENSUS.md` §1 and
   `campaign/julia-trailing-comma-defect.md` §4 (24-witness corpus, base
   `62fa03fe`: exactly one sub-repair ACTIVE — recovered-return-range on two
   witnesses; other four sub-repairs unfired).

## 2. Regression corpus and gates

Corpus shipped as testdata under `campaign/fixtures/julia/` (25 `.jl` fixtures
+ `FIRING-CENSUS.md`): the firing witness
(`inline-recovered-return-range.jl`, 38 bytes) and the full 24-witness census
sweep plus one recovery variant. Permanent gates in
`parser_result_julia_retirement_test.go`:

- `TestJuliaRetirementRawMatchesLockedCOracleOnFiringWitness` — pins the raw
  normalized-dump digest of the firing witness to the locked-C digest
  `sha256:9e2de3670281c5a0013d55b570e6c175c7d7eb2d5142bca99276974ea61453e2`;
- `TestJuliaRetirementTrailingCommaDefectWitnessPinned` — keeps the defect
  filing observable (error flag + pinned raw digest);
- `TestJuliaRetirementProductionEqualsRawOverCorpus` — production == raw on
  every corpus witness (no behavior change where the arm never fired);
- `TestJuliaRetirementFiringCensusPostDeletion` — no parse records a
  `dispatch.julia` pass again;
- `TestJuliaRetirementFixtureCorpusIsComplete` — fixture dir matches the
  census corpus exactly.

## 3. Commands executed and outputs

All gates re-run at HEAD `8399393b` ("update: Update julia retirement fixtures
and receipt tests"), workdir `/home/draco/work/gts-campaign-a3-20260823`.

### Gate 1 — registry

```
$ go test . -run '^TestResultCompatibilityOwnershipRegistry$' -count=1
ok  	github.com/odvcencio/gotreesitter	0.039s
```

### Gate 2 — focused julia retirement tests

```
$ go test . -run 'TestJuliaRetirement' -v -count=1
--- PASS: TestJuliaRetirementRawMatchesLockedCOracleOnFiringWitness (0.03s)
--- PASS: TestJuliaRetirementTrailingCommaDefectWitnessPinned (0.01s)
--- PASS: TestJuliaRetirementProductionEqualsRawOverCorpus (0.18s)
    --- PASS: TestJuliaRetirementProductionEqualsRawOverCorpus/<all 25 witnesses>
--- PASS: TestJuliaRetirementFiringCensusPostDeletion (0.18s)
    --- PASS: TestJuliaRetirementFiringCensusPostDeletion/<all 24 canonical witnesses>
--- PASS: TestJuliaRetirementFixtureCorpusIsComplete (0.35s)
PASS
ok  	github.com/odvcencio/gotreesitter	0.957s
```

### Gate 3 — build

```
$ go build ./...
BUILD_OK        # exit=0, no output
```

### Gate 4 — vet

```
$ go vet ./...
VET_OK          # exit=0, no output
```

## 4. Draft ledger row (registry state as committed)

`testdata/result_compat_ownership_v1.json:1554-1585`, row id
`dispatch.julia`:

```json
{
  "id": "dispatch.julia",
  "kind": "dispatcher_arm",
  "functions": [ "normalizeJuliaCompatibility" ],
  "files": [ "parser_result_julia.go" ],
  "languages": [ "julia" ],
  "purpose": "Reconcile Julia recovery and returned-tree shape.",
  "authoritative_owner": "scheduler_action_semantics",
  "witnesses": [ "cgo_harness/julia_decision_dump_test.go" ],
  "retirement_condition": "Delete only after scheduler_action_semantics emits the same tree natively for every registered witness and production, compact, forest, incremental, and C-oracle route receipts are exact.",
  "route_coverage": {
    "production": "retired_exact_receipt",
    "compact": "retired_exact_receipt",
    "forest": "retired_exact_receipt",
    "incremental": "retired_exact_receipt",
    "c_oracle": "retired_known_divergence_receipt"
  },
  "status": "retired",
  "receipt_refs": [
    "campaign/julia-decision.md",
    "campaign/julia-trailing-comma-defect.md",
    "parser_result_julia_retirement_test.go"
  ],
  "retired_commit": "2c5ddcc9a566d2df4eed8544df0a022636d9977b"
}
```

Ledger denominators after this retirement (docs/compat-tier.md:19-27):
dispatcher_arms **30**, dispatcher_languages **32**, live_entries **31**,
retired_entries **57**, live language labels **34**.

## 5. Commit chain

- `62fa03fe` add: add julia dispatch retirement probes and decision doc
- `2c5ddcc9` remove(julia parser): retire dispatch.julia parser compatibility
  arm (= registry `retired_commit`)
- `00dfc540` update: update compat registry for julia retirement
- `5da4a14f` remove(tests): Remove Julia decision dump diagnostic test
- `8399393b` update: Update julia retirement fixtures and receipt tests
- this receipt commit

Reopen conditions: `campaign/julia-trailing-comma-defect.md` §5 (grammar
revision moving a pinned digest, any route receipt failure over the corpus, or
a consumer demanding the C-oracle `ERROR(operator)` tuple shape).
