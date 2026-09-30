# Incremental reuse decision census

Build with `gts_incr_census` and call `DiagnosticObserveIncrementalReuse` around
`Tree.Edit` and the incremental parse. The normal build has no observer symbols,
parser fields, clocks, defers, or runtime switches. Compile-time guards remove
hooks; the few Boolean adapters inline to their argument.

The recorder lives in `internal/diag/incrcensus`. The root files only adapt node
identities and forward hooks. The observer is process-scoped: run it without
concurrent parser calls or nested observers. It releases all node references
when the callback returns, including after a panic.

Each report contains ordered decisions, the first true operand of compound
refusal guards, successful reuse events, whole-edit fallback reasons, dispatch
positions where reuse was not offered, and exclusive phase time. It separates
`Tree.Edit`, cursor work, lexical proof, scanner proof, subtree selection,
rebuilding, retrying, and fresh recovery verification. Dispatch intervals label
reparse work performed while multiple stacks, a dead stack, or EOF barred reuse.
Those intervals measure work under a barrier; they do not estimate the savings
from removing that barrier. The guard's own evaluation cost is distinct from
reparse work.

The timing rows plus observer time sum exactly to the observed edit wall time.
Observer time covers the recorder itself. Snapshot and returned-tree walks run
outside that time. Branch and clock overhead can still perturb parsing, so use
separate unobserved timings for throughput. The census is wall-time attribution,
not a CPU sampling profile or a counterfactual speedup prediction.

Lost-node shares use old node identity in the selected result. Reused descendants
are subtracted from rejected ancestors. A node's own refusal takes priority;
otherwise a dispatch refusal at its start position, then the nearest rejected
ancestor, explains its loss. A selected fresh fallback that retains no old node
owns all lost nodes. The remaining bucket is `not_offered_or_selected`.
This partitions physical old nodes without counting overlapping rejected
subtrees several times. It does not count structurally equal new allocations as
reuse, and a rejected ancestor is an observed explanation rather than proof that
its guard alone caused every descendant's loss.

The report's `final_fallback` is the last fallback event observed across all
attempts. A later retry can select an incremental tree, so that event alone does
not prove that the returned tree is fresh. Check retained identities and the
returned runtime's `IncrementalOldTreeReuseRoute` as well. Verification and retry
time includes discarded attempts. Dependency-proof outcomes distinguish
`skip_edit_visit` from `needs_edit_visit`; needing an invalidation visit is not
itself a refusal to reuse a node.

`fixtures.json` records the closest unmodified tracked real source file to each
32 KiB and 137 KiB target, with path, byte count, commit, and SHA-256. The
selection includes tests, generated files, malformed inputs, and pre-existing
parity failures. Files are neither cropped nor padded. The nearest locked
PowerShell file to 137 KiB is 104,415 bytes; that size is reported explicitly.
The external corpus lock must match the repository's committed lock digest and
stays outside the repository.

The matrix includes fixed quarter, middle, and three-quarter sites:

- `one_byte`: insert one ASCII space at the fixed byte offset.
- `100_byte`: insert 100 ASCII spaces at the same offset.
- `splice`: replace 100 bytes with the following 32 original bytes.
- `numeric_replace`: change one digit in the nearest numeric leaf, exercising
  the same-width lexical proof separately from insertion.

Each cell runs three Go-C-C-Go cycles with one parser per implementation.
Parser construction, old-tree parsing, digesting, source loading, and tree release
are outside the edit timer; `Tree.Edit` is inside. An extra observed call records
all decisions. Six Go and six C samples per cell expose local variation. Each
language runs in a separate Docker process with `GOWORK=off`, `GOMAXPROCS=1`,
one pinned CPU, and an 8 GiB container limit. The runner compiles first and
measures the test binary's RSS separately from compilation.

Run the locked matrix with:

```sh
GOWORK=off GTS_INCR_CENSUS_CPU=0 \
  bash scripts/run_incremental_reuse_census.sh "$CORPUS_ROOT" "$OUTPUT_DIR"
```

The runner verifies source checkout commits and file hashes before running.
`TestIncrementalReuseCensus` requires an explicit language and output path and
emits every edit, including malformed cases. It asserts observer transparency,
unchanged work counters, ERROR-root flags, and accepted-root coverage. Its
receipt records incremental/fresh Go, fresh Go/C, and incremental/fresh C
outcomes. That inventory supplements the existing invariant and parity gates;
it does not grant an exemption for a mismatch.

The locked C oracle is the repository's current runtime commit
`f5afe475deb7c0bae6407fb776c76824f717bb61` (0.25.1), transported by the pinned
`go-tree-sitter` 0.25.0 binding. Each language receipt includes grammar commit,
source/artifact identities, flags, and runtime identity. The task does not
upgrade the oracle.

The reducer preserves every edit's digests, counters, and outcomes. It omits
unabridged event arrays from the checked-in summary while recording their count
and the SHA-256 of each complete raw JSONL artifact. It retains both exact
reasons and readable groups. The ranked results and integration body belong in
`docs/receipts/incremental-reuse-census/`.

The standard core no-edit API remains allocation-free in the transparency
checks. Java's token-source factory entry creates its token source before the
unchanged-tree check. On the smoke fixture, a cached factory allocates once and
the registry-style wrapper allocates twice per call, in both baseline and
instrumented revisions. This pre-existing API-path cost remains outside this
diagnostic change; a passing core check does not certify every factory entry.

The reducer keeps the requested 144 edits (three classes, three sites, two
sizes, eight languages) separate from 48 numeric replacement controls. It also
reports each class and each size separately. The comparison tool requires exact
digests, verdicts, work counters and retained identities before timing:

```sh
GOWORK=off python3 scripts/compare_incremental_reuse_census.py \
  "$BASELINE_ARTIFACTS" "$CURRENT_ARTIFACTS" \
  go javascript typescript python rust java c_sharp powershell \
  --output "$COMPARISON_JSON"
```

Original PowerShell runs hit the duplicate error-subtree dependency walk added
in `e91b944fc`; its 72-step invariant session timed out after 20 minutes on both
original engine revisions. The existing fix `4afda6f2d` was applied only to
isolated PowerShell measurement checkouts. Receipts label that dependency
explicitly. It is not part of the instrument, and original PowerShell timing
must not be inferred from those control measurements.
