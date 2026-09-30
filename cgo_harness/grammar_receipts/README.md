# Grammar receipts

`gts-grammar-receipt/v1` is a JSON snapshot for one grammar. The Go types and
validation live in `internal/grammarreceipt`. Receipts record inputs and
toolchain identities alongside fresh and incremental locked-C parity and the
current invariant result. A passing receipt records evidence; it does not
graduate a grammar.

The route probe is `grammars.ParseSmokeSample(grammar)`. It gives a stable,
small fixture for classifying the compact route as `accepted`, `declined` with
a reason, or `forest_route`. The receipt also records each selected corpus
file and the route class used for that parse. Corpus selection takes the four
largest matching files no larger than 1 KiB from the authenticated source
checkout. The per-file byte cap is recorded in the receipt. The corpus lock
digest authenticates the full 206-grammar lock; the manifest digest
authenticates the selected files and their locked revisions.

The incremental check uses the design's 72-step session on the largest
selected file: 24 insertions, 24 replacements, and 24 deletions across 16
labeled sites per edit class. At every step it compares incremental Go with
fresh Go and fresh locked C. These comparisons and the Go invariants determine
`step.pass`. C incremental versus fresh C and Go versus incremental C are
separate diagnostic axes, with separate failure records. C's own incremental
recovery choices or an unavailable C incremental tree do not fail Go's result
(organization decision 0012). A missing fresh C oracle still fails the parity
gate. The receipt also checks
root coverage, ERROR-root reporting, and zero allocations for a no-edit Go
reparse. These bounded checks are a receipt sample, not the wider E-A
graduation gate.

The generator runs inside the cgo harness Docker image and accepts exactly one
grammar per invocation:

```sh
flock /tmp/gts-docker.lock \
  cgo_harness/grammar_receipts/run_one_in_docker.sh \
  go /srv/gotreesitter-perf/corpus_sources \
  /path/to/corpus_sources.lock \
  "$(awk 'NR == 1 {print $1}' cgo_harness/perf_scan/corpus_sources.lock.sha256)" \
  /tmp/grammar-receipts
```

Lean 4 remains opt-in. Its C grammar and corpus source pins are in
`lean_c_oracle.lock` and `lean_corpus_sources.lock`; each has a SHA-256
sidecar. The generator loads the C runtime commit through
`cgo_harness.COracleIdentity`, so a runtime pin change regenerates receipts
without a command change. Each receipt records the SHA-256 of the compiled
C source manifest, which authenticates the binding and runtime bytes.

`run_one_in_docker.sh` defaults to a 90-minute per-grammar Docker wall limit.
Set `GTS_GRAMMAR_RECEIPT_WALL_TIMEOUT` to override it. A timeout writes a
valid receipt with `execution.status`, `time_limit`, and `timeout` parity and
invariant results; it never records a pass. The timeout receipt records the
same grammar, C oracle, route, cohort, and corpus identities without rerunning
parity.

`.github/workflows/grammar-receipts.yml` runs one grammar per matrix job on
GitHub-hosted runners and uploads each JSON receipt as a workflow artifact,
plus a summary artifact and a run summary table. It never writes receipt files
into the repository and needs no secrets. Each job loads the cgo harness image
and fetches only its own grammar's corpus checkout at the commit pinned in
the corpus lock. The lock is not committed: blocker receipts under
`cgo_harness/` assert that the repository has no `corpus_sources.lock`, so a
committed copy could stand in for authenticated evidence. The digest in
`cgo_harness/perf_scan/corpus_sources.lock.sha256` is the authentication. Each
run reads the lock from the `corpus_lock_url` dispatch input or the
`GTS_CORPUS_LOCK_URL` repository variable and checks it against that digest
(`scripts/fetch_grammar_receipt_lock.sh`). The URL is not a secret: the lock lists only public repositories at commits, so
any pull request run can read it, and the digest check, not URL secrecy,
authenticates the lock. Without a URL, dispatch, schedule and
release runs fail, and pull request runs skip the receipt jobs. The harness image
and the checkouts are cached with `actions/cache`. Triggers:

- `workflow_dispatch` with a `grammars` input (`smoke`, `all`, or a list such as
  `go,python`) and a `max_parallel` input;
- a weekly schedule and `release`: all 206 default grammars plus Lean 4;
- `pull_request` for changes under `cgo_harness/`, `grammars/`, `internal/`, or
  the root package: the grammars whose files the pull request changes (at most
  8), or a 3-grammar smoke set when it changes none.

`scripts/plan_grammar_receipts.py` chooses the grammars. The receipt records
the commit that ran: the pull request head for pull requests.

`run_all_in_docker.sh` generates every receipt on one machine. Set
`GTS_GRAMMAR_RECEIPT_CORPUS_ROOT` to a directory that holds one corpus checkout
per grammar (`cgo_harness/cmd/real_corpus_sources -apply` creates it from the
lock). Set `GTS_GRAMMAR_RECEIPT_CORPUS_LOCK` to the authenticated lock (fetch it
with `scripts/fetch_grammar_receipt_lock.sh <url> <path>`). Optional variables:
`GTS_GRAMMAR_RECEIPT_REPORT_DIR` (default `/tmp/grammar-receipts`; receipts and
the one-table summary go there), `GTS_GRAMMAR_RECEIPT_DOCKER_LOCK` (default
`/tmp/gts-docker.lock`), and `GTS_GRAMMAR_RECEIPT_WALL_TIMEOUT`. The script
takes the Docker lock for each Docker operation and releases it between
grammars.

To publish a qualification baseline, first commit the generator and engine,
then generate all 207 receipts from that clean revision. Seal the result:

```sh
GOWORK=off python3 scripts/seal_grammar_receipts.py \
  --report-dir /tmp/grammar-receipts \
  --output-dir docs/qualification/c027 \
  --revision "$(git rev-parse HEAD)"
```

The sealer reads pins and grammar blobs at the measured revision. It rejects
missing artifacts, dirty or mixed revisions, wrong runtime source manifests,
corpus lock drift, changed selection limits, and receipts that still mix C's
incremental disagreement into Go's pass result. Failures and unavailable
samples remain evidence; they never become passes. The archive filename is
its SHA-256, and the manifest records every receipt's digest and result.
Unlike routine CI receipts, this sealed baseline archive is checked in so its
evidence remains available at an immutable Git SHA. It contains no corpus
lock or source files. Publish it in a separate `update(pins)` commit, and
include the summary's separate oracle axes and unavailable reasons.
