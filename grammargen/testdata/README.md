# Real-corpus parity floor data

`real_corpus_parity_floors.json` is a ratchet-floor file for the
grammargen real-corpus parity tests (`parity_real_corpus_test.go`,
`bash_parity_test.go`, and similar per-grammar witnesses). For each
grammar, it compares grammargen's compiled output against the shipped
reference blob. The compiled output builds directly from the grammar's
source `grammar.json`. The comparison runs over a locally seeded
real-world corpus.

Generated: 2026-07-16 (`generated_at` in the file). Format: version 3.

The floor is per grammar, per metric (`eligible`, `no_error`,
`sexpr_parity`, `deep_parity`). A test may only raise its floor; a run
that falls below its recorded floor fails. These tests are skipped by
default (`GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1` and a locally seeded
corpus are required) and are not part of the default `go test ./...`
gate.

This file tracks the grammargen lane only: the code generator's
output against the shipped reference blob. It is not a statement of
shipping-grammar parity quality against the upstream tree-sitter
oracle. For that claim, see [docs/benchmarks.md](../../docs/benchmarks.md) and the
C-oracle parity suites under `cgo_harness/`.
