# B1 harness gaps — commands this run could not execute, and why

## RESOLVED G-1 (was: "the fine-grained census driver does not exist as a runnable artifact")

The prior checkpoint claimed no per-file depth driver existed in the tree and
that a new root-package test would be required. That claim was **stale**:

- `admission_real_corpus_matrix_test.go` (`//go:build gts_parsercorephase0`,
  `TestAdmissionCandidateRealCorpusMatrix`) is a shipped, env-driven depth
  driver: it takes a JSON manifest of `{language, bucket, bytes, output_path}`
  via `GTS_ADMISSION_REAL_CORPUS_MANIFEST`, reads each real file, and runs the
  scorecard's PASS/DIVERGE/FALLBACK classification with decline reasons
  (`admission_real_corpus_matrix_test.go:36-139`, `:271-284`).
- With `GTS_ADMISSION_CENSUS=1` (`admission_census.go`,
  `admissionCensusEnabled`/`admissionCensusClassify`) declines carry
  fine-grained `[mechanism=…]` tags.

Command actually executed on 2026-08-23 (repo HEAD `af9ded2b`,
go1.25.1 linux/amd64), via `campaign/fixtures/b1/run-depth-census.sh`:

```
GTS_ADMISSION_REAL_CORPUS=1 \
GTS_ADMISSION_REAL_CORPUS_MANIFEST=campaign/fixtures/b1/depth-manifest.json \
GTS_ADMISSION_CENSUS=1 \
go test -tags gts_parsercorephase0 -run TestAdmissionCandidateRealCorpusMatrix -v .
```

Exit code 0, all 48 fixtures exercised, raw log at
`campaign/fixtures/b1/depth-census-run.log`. Results recorded in
`campaign/b1-depth-census.md`.

## STILL OPEN G-2 — no real third-party corpus for 34 of the 48 PASS languages

A full sweep of the workspace (`caps.WalkDir(".")`, 2,858 entries,
bucketed by extension) found no pre-existing real corpus file for:
beancount, chatito, commonlisp, crystal, csv, cylc, desktop, disassembly,
earthfile, editorconfig, fish, git_config, git_rebase, gitcommit, hcl,
hyprlang, kconfig, liquid, make, matlab, mermaid, nushell, odin, pascal,
prolog, purescript, requirements, tcl, tmux, twig, uxntal, v, vimdoc, xml.

The 2026-07-20 census sourced real files from `cgo_harness/corpus_real/`
(docs/compact-route-coverage-census.md, "Depth check" intro); that directory
is absent here (`lstat …/cgo_harness/corpus_real: no such file or
directory`). For those 34 languages this run used representative authored
fixtures under `campaign/fixtures/b1/<lang>/` (see
`campaign/b1-depth-fixtures.md`). Consequence: their rows in
`b1-depth-census.md` are genuine executed results, but only against
authored inputs — before any file-scale coverage claim is published for
them, replace the authored fixtures with upstream corpus files via
`GTS_ADMISSION_REAL_CORPUS_MANIFEST` (no code change needed; the driver is
manifest-driven).

## STILL OPEN G-3 — one unclassified decline

ledger's decline ("accepted compact root is incomplete or erroneous:
span=0..932 expected=1..932 error=false allowErrorRoot=false") carries no
`[mechanism=…]` tag under `GTS_ADMISSION_CENSUS=1`; it exits through a path
that does not route through `admissionCensusClassify`
(`campaign/fixtures/b1/depth-census-run.log`, line 77). Recorded verbatim in
`b1-depth-census.md` row 27 rather than guessed.
