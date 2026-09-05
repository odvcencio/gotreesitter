# Scripts

For heavy correctness, parity, and race work, use CI or the Docker runners under
`cgo_harness/docker` and keep runs to one language at a time. The scripts in
this directory are focused host-side helpers, not the default path for OOM
diagnosis.

`with_grammar_subset.sh` is the host-side low-memory wrapper for focused grammar
work. It forces serial subset builds, wires in external blob loading, and can
point built-in grammar loaders at local grammargen `.bin` overrides.

`prune_harness_artifacts.sh` reports root build products, reproducible caches,
and run receipts separately. It defaults to a dry run. `--delete` removes only
root build products and reproducible caches; durable harness and benchmark
receipts require the separate `--delete-receipts` opt-in. It never includes
private `.gts/`, `.tiller/`, or other agent notes.

`canopy_query.sh` validates the cached index before each structural query. It
uses a fresh scoped query when indexed files or the source set changed.

`refresh_canopy_index.sh` builds a temporary index and validates it. It records
the Git commit only after it promotes the validated index.

`run_randomized_benchmarks.sh` runs the production, incremental, recovery,
replay, compact-core, and corridor benchmarks once per explicit shuffle seed.
Pass each output file to `benchstat` for a before-and-after comparison.

The default set includes:

- the primary full-parse and incremental benchmarks;
- the random-edit and parser-core controls;
- the recovery and synthetic-root replay targets;
- the warm and corridor scheduler lanes;
- the fresh full and selected-store canonical fixtures;
- the tags, legacy fact, and compiled `FactProgram` extraction lanes.

Set `GTS_RECOVERY_CORPUS_FILE` and `GTS_RECOVERY_CORPUS_LANG` to add one exact
corpus file. The script skips `BenchmarkRecoveryCorpusFile` when either value
is absent. Use the same file and language for both comparison runs.

```sh
export GTS_RECOVERY_CORPUS_FILE=/absolute/path/to/corpus-file
export GTS_RECOVERY_CORPUS_LANG=elixir
```

Run a paired comparison from the candidate checkout. Supply the baseline
checkout and a separate output path.

```sh
bash scripts/run_randomized_benchmarks.sh \
  --baseline-root /absolute/path/to/baseline \
  --baseline-output /tmp/gts-base.txt \
  --output /tmp/gts-head.txt
benchstat /tmp/gts-base.txt /tmp/gts-head.txt
```

Each pair uses the same shuffle seed. The driver reverses checkout order for
each successive seed and holds one lock across the complete campaign.
Each process uses `GOMAXPROCS=1`, `-count=1`, and `-benchmem`.
The defaults remain 20 seeds and a 750 millisecond benchmark duration.

The current driver runs both checkouts. The baseline does not need to contain
this script. Each output records the checkout identity, settings, and seed
boundaries. Dirty checkout metadata identifies development runs; it does not
authenticate their source contents.

Require `# status: complete` as the final line in both outputs before comparing
them. A failed or interrupted process leaves the campaign incomplete.
Output paths must differ and must not already exist.

Use `--require-benchmarks` with comma-separated names to require those timing
rows in every process. The driver removes Go's CPU suffix before matching.
Continuous integration requires each member of the primary trio for every
seed. A partial sample set cannot pass as a complete comparison.

Set the same build tags and fixture controls for both checkouts. Use
`--tags ''` to measure the ordinary build. Keep instrumented diagnostics
separate from timing evidence. Paired order reduces drift bias but does not
establish a quiet host or certify grammar parity.

Continuous integration uses paired runs for its advisory Go benchmark trio.
It retains the separate memory probe and reports interrupted campaigns as
inconclusive. Use certified corpus and fleet runs for language-wide claims.
