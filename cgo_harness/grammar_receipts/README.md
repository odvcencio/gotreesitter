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
fresh Go and both with fresh and incrementally edited locked C trees. It also checks
root coverage, ERROR-root reporting, and zero allocations for a no-edit Go
reparse. These bounded checks are a receipt sample, not the wider E-A
graduation gate.

The generator runs inside the cgo harness Docker image and accepts exactly one
grammar per invocation:

```sh
flock /tmp/gts-docker.lock \
  cgo_harness/grammar_receipts/run_one_in_docker.sh \
  go /srv/gotreesitter-perf/corpus_sources \
  /srv/gotreesitter-perf/corpus_sources.lock \
  "$(awk 'NR == 1 {print $1}' cgo_harness/perf_scan/corpus_sources.lock.sha256)" \
  /tmp/grammar-receipts
```

Lean 4 remains opt-in. Its C grammar and corpus source pins are in
`lean_c_oracle.lock` and `lean_corpus_sources.lock`; each has a SHA-256
sidecar. The generator loads the C runtime commit through
`cgo_harness.COracleIdentity`, so a runtime pin change regenerates receipts
without a command change.

`run_one_in_docker.sh` defaults to a 90-minute per-grammar Docker wall limit.
Set `GTS_GRAMMAR_RECEIPT_WALL_TIMEOUT` to override it. A timeout writes a
valid receipt with `execution.status`, `time_limit`, and `timeout` parity and
invariant results; it never records a pass. The timeout receipt records the
same grammar, C oracle, route, cohort, and corpus identities without rerunning
parity.

`.github/workflows/grammar-receipts.yml` runs one grammar per matrix job on
`[self-hosted, perf]` runners and uploads each JSON receipt as a workflow
artifact. It never writes receipt files into the repository. It starts only by
manual dispatch today: no runner with that label is registered for this
repository, so the pull request, schedule, and release triggers are commented
out in the file until one exists.

`run_all_in_docker.sh` generates every receipt on one machine. Set
`GTS_GRAMMAR_RECEIPT_CORPUS_ROOT` and `GTS_GRAMMAR_RECEIPT_CORPUS_LOCK` to the
authenticated corpus checkout and its lock. Optional variables:
`GTS_GRAMMAR_RECEIPT_REPORT_DIR` (default `/tmp/grammar-receipts`; receipts and
the one-table summary go there), `GTS_GRAMMAR_RECEIPT_DOCKER_LOCK` (default
`/tmp/gts-docker.lock`), and `GTS_GRAMMAR_RECEIPT_WALL_TIMEOUT`. The script
takes the Docker lock for each Docker operation and releases it between
grammars.
