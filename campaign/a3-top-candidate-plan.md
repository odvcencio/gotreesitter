# A3 — Full retirement change plan: `dispatch.corn`

Campaign prong A, artifact 3. Plan only; this document ships no code, test,
or registry change. Target candidate per `campaign/a2-retirement-queue.md`:
`dispatch.corn`, the number-one retirable-now candidate.

## Candidate summary

- Registry entry: `dispatch.corn` in
  `testdata/result_compat_ownership_v1.json`; status `live`;
  `authoritative_owner: scheduler_action_semantics`;
  `evidence_scope: baseline_corpus_wide_only`.
- Switch arm: `runLanguageResultCompatibility` →
  `dispatcherArmCensus(ctx, "dispatch.corn", func() { normalizeCornCompatibility(...) })`
  at `parser_result_compat.go:137` (verified in this worktree).
- Implementation: `normalizeCornCompatibility(root *Node, source []byte,
  lang *Language)` at `parser_result_corn.go:3`. No registered subpasses.
- Languages: exactly `corn`.
- Existing receipt: "2026-08-23 Corn blocker receipt"
  (`docs/root-normalization-retirement.md`), status NO-GO / KEEP LIVE:
  792 A0 visited nodes, zero error roots, zero rewrites — the cleanest
  zero-rewrite A0 profile among receipted arms.
- Route coverage today: production/compact/forest/incremental =
  `shared_result_compatibility_tail`; `c_oracle = curated_single_grammar_parity`.

## Change plan (ordered)

### Step 0 — Precondition receipts (no code change)

1. Compatibility-free producer proof: extend the Corn blocker receipt's A0
   probe to every registered witness and show native reduction emits the
   locked-C tree with zero `dispatch.corn` rewrites.
2. Production receipt: exact raw/production digests per witness.
3. Compact fallback receipt: compact admission or documented fallback per
   witness.
4. Forest receipt: exact forest output or documented decline per witness.
5. Incremental reuse receipt: nonzero reuse or documented unsupported reason,
   fresh and edited, per witness.
6. Isolated C-oracle parity: locked-C digest equality on every covered route
   for every witness.

If any route diverges: stop, file the divergence as a new blocker receipt,
keep the arm live. This plan proceeds only when all six pass.

### Step 1 — Delete the switch arm

File: `parser_result_compat.go`

- Remove the `dispatch.corn` arm at line 137:
  the `dispatcherArmCensus(ctx, "dispatch.corn", ...)` block calling
  `normalizeCornCompatibility`.
- No generic pass follows language dispatch (`generic_passes: 0`), so no
  downstream dispatch target needs rewiring; the `corn` case simply
  disappears and parsing falls through to the native result path only.

### Step 2 — Delete the implementation

Files:

- Delete `parser_result_corn.go` (contains `normalizeCornCompatibility`).
- Grep for residual references before deletion:
  `normalizeCornCompatibility`, `dispatch.corn`, `parser_result_corn.go`.
  Expected references at plan time are limited to the dispatcher call site
  (Step 1), the registry JSON, census manifests/tests, and docs/campaign
  records. Any unexpected reference is a stop condition.

### Step 3 — Update the registry

File: `testdata/result_compat_ownership_v1.json`

- Move entry `dispatch.corn` from `status: "live"` to `status: "retired"`,
  keeping the historical record (deleting it is not retirement).
- Add commit hash + receipt references to the retired entry.
- Update `denominator`: `live_entries` 32→31, `retired_entries` 56→57;
  `dispatcher_arms` 31→30; `dispatcher_languages` 33→32.

### Step 4 — Update census denominators and ratchets

Files:

- `testdata/dispatcher_census_a0_manifest_v1.json` — remove/annotate the corn
  fixture entries so the A0 manifest matches the post-retirement denominator.
- `testdata/dispatcher_census_tracked_v1.json` — same for the tracked census
  (seven fixtures; confirm whether a corn fixture is present).
- `parser_result_test/dispatcher_census_test.go` — the census tests
  (`TestDispatcherArmCensusOverRealCorpus`,
  `TestDispatcherArmCensusTrackedReceipt`) read the registry JSON directly
  (line 50) and drive `dispatcherArmCensus`; after Steps 1–3 they must see
  31 live entries with no `dispatch.corn` arm identifier. Update any
  hard-coded arm list / expected counts in the test.
- The registry ownership guard test that validates
  `testdata/result_compat_ownership_v1.json` against the dispatcher (the
  `TestResultCompatibilityOwnershipRegistry` named in
  `docs/compat-tier.md`) must pass with the new denominator.

### Step 5 — Witness tests (new focused regression, following program rules)

New file: `grammars/corn_native_regression_test.go` (mirroring the FIDL /
HLSL retirement-test pattern):

- For each previously registered corn witness: parse natively
  (compatibility-free producer), assert the tree equals the recorded
  pre-retirement production digest byte-for-byte.
- Assert the retired label `dispatch.corn` is absent from the dispatcher
  census output and that no compatibility walk fires for `.corn` sources.
- Compact fallback behavior: assert compact either admits the witness
  directly or fails closed identically to the pre-retirement compact route.
- Incremental: fresh and edited edits reproduce the pre-retirement incremental
  reuse profile (reuse counts or documented unsupported reason).
- C-oracle parity spot check via the existing isolated parity harness pattern
  (`cgo_harness/*_parity_cgo_test.go` style) if the harness includes corn;
  otherwise record the isolated locked-C digest comparison in the receipt.

### Step 6 — Documentation and ledger

Files:

- `docs/compat-tier.md` — remove corn from the live-arm narrative; add the
  retirement note with the new denominator (30 explicit arms, 32 languages,
  31 live entries, 57 retired entries).
- `docs/root-normalization-retirement.md` — append a dated Corn retirement
  receipt: base commit, six route receipts, census logs, decision GO, reopen
  conditions ("Reopen the entry if a future witness rewrites a node or
  diverges on any route").
- Progress ledger row: `| Corn dispatcher arm | retirement change | 1
  dispatcher arm | 0 | ... |` citing the six-route receipts.
- `CHANGELOG.md` entry per program convention.

## Census commands (evidence collection)

Run from the repository root; these are observation commands only:

```sh
# All-arm runtime probe through the production parse path (A0-style):
GTS_DISPATCHER_CENSUS=1 go test ./... -run 'TestDispatcherArmCensus' -count=1

# Real-corpus census (R2-style): per-arm checked/run/rewritten counts;
# must report zero rewrites for corn pre-retirement and no corn arm after:
go test ./parser_result_test -run TestDispatcherArmCensusOverRealCorpus -count=1 -v

# Tracked-census receipt:
go test ./parser_result_test -run TestDispatcherArmCensusTrackedReceipt -count=1 -v

# Registry/denominator ownership guard after the JSON edit:
go test ./parser_result_test -run 'Ownership|Registry' -count=1 -v

# Focused native regression (Step 5):
go test ./grammars -run CornNativeRegression -count=1 -v

# Whole-tree gates before merge:
go build ./... && go vet ./... && go test ./...
```

Docker-run variants of the route receipts land their `container.log` under
`harness_out/docker/<timestamp>-corn-retirement-*` per program convention and
are cited by SHA-256 in the retirement receipt.

## Merge gate

Correctness is the merge gate (`docs/root-normalization-retirement.md`):
all six Step-0 receipts exact, all census commands green with the new
denominator, CI green, and the registry/test/doc triple updated in one
change. Any node, flag, digest, or route divergence reopens the candidate
and returns `dispatch.corn` to `KEEP LIVE`.
