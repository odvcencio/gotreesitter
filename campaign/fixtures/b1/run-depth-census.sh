#!/bin/sh
# B1 depth census driver: builds the 48-fixture manifest from
# campaign/b1-depth-fixtures.md rows and runs the shipped real-corpus
# admission matrix (the depth driver) with fine-grained census classification.
set -e
cd "$(dirname "$0")/../../.."
python3 - <<'EOF'
import json, os
rows = [
 ("bash","cgo_harness/docker/run_forest_corpus_parity.sh"),
 ("beancount","campaign/fixtures/b1/beancount/sample.beancount"),
 ("bitbake","testdata/dispatcher_census_a0/bitbake/large__linux-firmware_20260519.bb"),
 ("chatito","campaign/fixtures/b1/chatito/sample.chatito"),
 ("commonlisp","campaign/fixtures/b1/commonlisp/sample.lisp"),
 ("crystal","campaign/fixtures/b1/crystal/sample.cr"),
 ("csv","campaign/fixtures/b1/csv/sample.csv"),
 ("cylc","campaign/fixtures/b1/cylc/sample.cylc"),
 ("desktop","campaign/fixtures/b1/desktop/sample.desktop"),
 ("disassembly","campaign/fixtures/b1/disassembly/sample.disassembly"),
 ("dockerfile","cgo_harness/docker/Dockerfile"),
 ("dtd","testdata/dispatcher_census_a0/dtd/large__docbook.dtd"),
 ("earthfile","campaign/fixtures/b1/earthfile/Earthfile"),
 ("editorconfig","campaign/fixtures/b1/editorconfig/sample.editorconfig"),
 ("elixir","testdata/admission_direct/recursive_insert/elixir.ex"),
 ("fish","campaign/fixtures/b1/fish/sample.fish"),
 ("git_config","campaign/fixtures/b1/git_config/sample.gitconfig"),
 ("git_rebase","campaign/fixtures/b1/git_rebase/sample.gitrebase"),
 ("gitcommit","campaign/fixtures/b1/gitcommit/sample.commitmsg"),
 ("go","cgo_harness/corpus_structural/go_sample.go"),
 ("gomod","cgo_harness/go.mod"),
 ("hcl","campaign/fixtures/b1/hcl/main.hcl"),
 ("hyprlang","campaign/fixtures/b1/hyprlang/sample.conf"),
 ("ini","testdata/dispatcher_census_a0/doxygen/small__example.cfg"),
 ("julia","testdata/compact_selected_lineage/julia_utils.jl"),
 ("kconfig","campaign/fixtures/b1/kconfig/Kconfig.census"),
 ("ledger","testdata/dispatcher_census_a0/ledger/small__non-profit-test-data.ledger"),
 ("liquid","campaign/fixtures/b1/liquid/sample.liquid"),
 ("make","campaign/fixtures/b1/make/Makefile"),
 ("markdown","corpuscheck/testdata/upstream_corpus/NOTICE.md"),
 ("matlab","campaign/fixtures/b1/matlab/classify.m"),
 ("mermaid","campaign/fixtures/b1/mermaid/sample.mmd"),
 ("ninja","testdata/dispatcher_census_a0/ninja/small__long-slow-build.ninja"),
 ("nushell","campaign/fixtures/b1/nushell/sample.nu"),
 ("odin","campaign/fixtures/b1/odin/main.odin"),
 ("pascal","campaign/fixtures/b1/pascal/census.pas"),
 ("prolog","campaign/fixtures/b1/prolog/census.pl"),
 ("purescript","campaign/fixtures/b1/purescript/Main.purs"),
 ("requirements","campaign/fixtures/b1/requirements/requirements.txt"),
 ("svelte","testdata/admission_direct/svelte_button.svelte"),
 ("tcl","campaign/fixtures/b1/tcl/census.tcl"),
 ("templ","testdata/dispatcher_census_a0/templ/medium__main.templ"),
 ("tmux","campaign/fixtures/b1/tmux/sample.tmux.conf"),
 ("twig","campaign/fixtures/b1/twig/sample.twig"),
 ("uxntal","campaign/fixtures/b1/uxntal/census.tal"),
 ("v","campaign/fixtures/b1/v/main.v"),
 ("vimdoc","campaign/fixtures/b1/vimdoc/census.txt"),
 ("xml","campaign/fixtures/b1/xml/corpus-entry.xml"),
]
entries=[]
missing=[]
for lang,p in rows:
    if not os.path.exists(p):
        missing.append((lang,p)); continue
    entries.append({"language":lang,"bucket":"b1-depth","bytes":os.path.getsize(p),"output_path":p})
print("missing:",missing)
json.dump({"entries":entries},open("campaign/fixtures/b1/depth-manifest.json","w"),indent=2)
print("entries:",len(entries))
EOF
GTS_ADMISSION_REAL_CORPUS=1 GTS_ADMISSION_REAL_CORPUS_MANIFEST=campaign/fixtures/b1/depth-manifest.json \
GTS_ADMISSION_CENSUS=1 \
go test -tags gts_parsercorephase0 -run TestAdmissionCandidateRealCorpusMatrix -v . \
  | tee campaign/fixtures/b1/depth-census-run.log
