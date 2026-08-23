# B1 depth fixtures — one fixture per currently-PASSING language (48; 14 repo-sourced, 34 authored)

Source for the PASS set: `docs/compact-route-coverage-census.md`, section
"Full per-language table → PASS (48)" (2026-07-20 snapshot, base commit
`7a43c9cb`): bash, beancount, bitbake, chatito, commonlisp, crystal, csv,
cylc, desktop, disassembly, dockerfile, dtd, earthfile, editorconfig,
elixir, fish, git_config, git_rebase, gitcommit, go, gomod, hcl, hyprlang,
ini, julia, kconfig, ledger, liquid, make, markdown, matlab, mermaid,
ninja, nushell, odin, pascal, prolog, purescript, requirements, svelte,
tcl, templ, tmux, twig, uxntal, v, vimdoc, xml.

Provenance classes:

- **repo** — a real corpus file that already exists in this worktree
  (`testdata/`, `corpuscheck/testdata/upstream_corpus/`,
  `cgo_harness/corpus_structural/`, `testdata/dispatcher_census_a0/`).
- **authored** — no real corpus file for the language exists anywhere in
  this workspace (verified by a full `WalkDir(".")` sweep of all 2,858
  entries bucketed by extension on 2026-08-23; see
  `campaign/b1-harness-gaps.md`). A representative multi-line fixture was
  authored under `campaign/fixtures/b1/<lang>/` (the only out-of-campaign-doc
  writes permitted by the task constraints). These are *not* third-party
  corpora and must be replaced by real upstream files before any
  file-scale depth claim is published.

| # | Language | Fixture path | Provenance |
|--:|---|---|---|
| 1 | bash | cgo_harness/docker/run_forest_corpus_parity.sh | repo (shell corpus used by forest-parity harness) |
| 2 | beancount | campaign/fixtures/b1/beancount/sample.beancount | authored |
| 3 | bitbake | testdata/dispatcher_census_a0/bitbake/large__linux-firmware_20260519.bb | repo (dispatcher census corpus) |
| 4 | chatito | campaign/fixtures/b1/chatito/sample.chatito | authored |
| 5 | commonlisp | campaign/fixtures/b1/commonlisp/sample.lisp | authored |
| 6 | crystal | campaign/fixtures/b1/crystal/sample.cr | authored |
| 7 | csv | campaign/fixtures/b1/csv/sample.csv | authored |
| 8 | cylc | campaign/fixtures/b1/cylc/sample.cylc | authored |
| 9 | desktop | campaign/fixtures/b1/desktop/sample.desktop | authored |
| 10 | disassembly | campaign/fixtures/b1/disassembly/sample.disassembly | authored |
| 11 | dockerfile | cgo_harness/docker/Dockerfile | repo (same path as the 2026-07-20 depth check used, but different content — that check read a 572-byte file; this one reads the current worktree copy) |
| 12 | dtd | testdata/dispatcher_census_a0/dtd/large__docbook.dtd | repo (dispatcher census corpus) |
| 13 | earthfile | campaign/fixtures/b1/earthfile/Earthfile | authored |
| 14 | editorconfig | campaign/fixtures/b1/editorconfig/sample.editorconfig | authored |
| 15 | elixir | testdata/admission_direct/recursive_insert/elixir.ex | repo |
| 16 | fish | campaign/fixtures/b1/fish/sample.fish | authored |
| 17 | git_config | campaign/fixtures/b1/git_config/sample.gitconfig | authored |
| 18 | git_rebase | campaign/fixtures/b1/git_rebase/sample.gitrebase | authored |
| 19 | gitcommit | campaign/fixtures/b1/gitcommit/sample.commitmsg | authored |
| 20 | go | cgo_harness/corpus_structural/go_sample.go | repo |
| 21 | gomod | cgo_harness/go.mod | repo |
| 22 | hcl | campaign/fixtures/b1/hcl/main.hcl | authored |
| 23 | hyprlang | campaign/fixtures/b1/hyprlang/sample.conf | authored |
| 24 | ini | testdata/dispatcher_census_a0/doxygen/small__example.cfg | repo (INI-shape config from dispatcher census corpus; actually a Doxygen .cfg repurposed — weak evidence for INI, nearest INI-family real file available) |
| 25 | julia | testdata/compact_selected_lineage/julia_utils.jl | repo |
| 26 | kconfig | campaign/fixtures/b1/kconfig/Kconfig.census | authored |
| 27 | ledger | testdata/dispatcher_census_a0/ledger/small__non-profit-test-data.ledger | repo (dispatcher census corpus) |
| 28 | liquid | campaign/fixtures/b1/liquid/sample.liquid | authored |
| 29 | make | campaign/fixtures/b1/make/Makefile | authored |
| 30 | markdown | corpuscheck/testdata/upstream_corpus/NOTICE.md | repo (upstream-corpus tree) |
| 31 | matlab | campaign/fixtures/b1/matlab/classify.m | authored |
| 32 | mermaid | campaign/fixtures/b1/mermaid/sample.mmd | authored |
| 33 | ninja | testdata/dispatcher_census_a0/ninja/small__long-slow-build.ninja | repo (dispatcher census corpus) |
| 34 | nushell | campaign/fixtures/b1/nushell/sample.nu | authored |
| 35 | odin | campaign/fixtures/b1/odin/main.odin | authored |
| 36 | pascal | campaign/fixtures/b1/pascal/census.pas | authored |
| 37 | prolog | campaign/fixtures/b1/prolog/census.pl | authored |
| 38 | purescript | campaign/fixtures/b1/purescript/Main.purs | authored |
| 39 | requirements | campaign/fixtures/b1/requirements/requirements.txt | authored |
| 40 | svelte | testdata/admission_direct/svelte_button.svelte | repo |
| 41 | tcl | campaign/fixtures/b1/tcl/census.tcl | authored |
| 42 | templ | testdata/dispatcher_census_a0/templ/medium__main.templ | repo (dispatcher census corpus) |
| 43 | tmux | campaign/fixtures/b1/tmux/sample.tmux.conf | authored |
| 44 | twig | campaign/fixtures/b1/twig/sample.twig | authored |
| 45 | uxntal | campaign/fixtures/b1/uxntal/census.tal | authored |
| 46 | v | campaign/fixtures/b1/v/main.v | authored |
| 47 | vimdoc | campaign/fixtures/b1/vimdoc/census.txt | authored |
| 48 | xml | campaign/fixtures/b1/xml/corpus-entry.xml | authored |

Notes

- Census execution: the manifest for these 48 fixtures is
  `campaign/fixtures/b1/depth-manifest.json`; the exact command and the raw
  per-language output are `campaign/fixtures/b1/run-depth-census.sh` and
  `campaign/fixtures/b1/depth-census-run.log` (results table:
  `campaign/b1-depth-census.md`).
- The 2026-07-20 census's own depth check sourced files from
  `cgo_harness/corpus_real/` "in the main worktree"
  (docs/compact-route-coverage-census.md, "Depth check" intro). That
  directory does not exist in this workspace (`caps.ListDir("cgo_harness")`
  shows no `corpus_real/`; `resolve …/corpus_real: no such file or
  directory`), which is why 34 of 48 rows fall back to authored fixtures.
- Repo-sourced rows reuse existing pinned evidence corpora wherever one
  exists; none of these files were modified.
