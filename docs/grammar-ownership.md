# Grammar ownership policy

Status: proposed

## Decision

gotreesitter ships local grammar corrections when upstream maintenance does
not meet user needs. Upstream review does not block a gotreesitter release.

Use upstream as a source, history, test set, and C compatibility oracle. Do
not let upstream response time control a user fix.

## Maintenance model

The priority tier defines the user commitment. The maintenance class defines
source custody. The labels are independent.

### Classes

| Class | Source custody | Update rule |
| --- | --- | --- |
| Mirror | Lock and import upstream source. | Adopt upstream only after review. |
| Lead | Keep pinned local overlays. | Remove an overlay after equivalent upstream proof. |
| Own | Store canonical editable source in this repository. | Use grammargen to emit the shipped blob. |

An Own grammar keeps its upstream source as a compatibility baseline. It does
not wait for an upstream merge before it ships.

## Tier 1

Tier 1 contains 30 critical grammar artifacts. Each Tier 1 grammar provides:

- Direct correctness intake.
- Reproducible artifacts.
- Realistic regression fixtures.
- Stable node, field, and query interfaces.
- Reviewed upstream reconciliation.

The initial Tier 1 set is:

| | | |
| --- | --- | --- |
| Bash | C | C++ |
| C# | CSS | Elixir |
| Go | GraphQL | HCL |
| HTML | Java | JavaScript |
| JSON | Kotlin | Lua |
| Markdown | Nix | PHP |
| Python | Ruby | Rust |
| Scala | SQL | Swift |
| TOML | TSX | TypeScript |
| XML | YAML | Zig |

The code manifest is [grammar_ownership.go](../grammars/grammar_ownership.go).
It records every Tier 1 class and owned grammar provenance.

Review Tier 1 at each major release. Replace a current member before adding a
new member beyond 30. Record an explicit capacity decision for an exception.

## YAML ownership pilot

YAML is Tier 1 and Own. Its canonical grammar source is
[yaml_grammar.go](../grammargen/yaml_grammar.go). Its canonical external scanner
is [yaml_scanner.go](../grammars/yaml_scanner.go).

The initial source is tree-sitter-yaml commit
`4463985dfccc640f3d6991e3396a2047610cf5f8`, under the MIT license. The Go DSL
is the release source. The checked-in blob is an output, not an editable input.

The owned scanner accepts quoted scalar continuation and closing delimiters
after a line break. The equivalent pinned C overlay is
[tree-sitter-yaml-multiline-quoted-scalars.patch](../grammars/patches/tree-sitter-yaml-multiline-quoted-scalars.patch).

The C overlay exists only for differential testing. `cmd/ts2go` does not
generate YAML artifacts. Grammargen owns the YAML release artifact.

YAML has 113 external scanner tokens. `YAMLGrammar` requires precise external
lexer states. The old reduced state builder lost required context in a
Kubernetes multi-document stream.

Regenerate the blob with:

```sh
go run ./cmd/grammargen emit yaml -bin grammars/grammar_blobs/yaml.bin
```

Run the focused evidence with:

```sh
go test ./grammargen -run '^TestYAML' -count=1
go test ./grammargen -run '^TestGrammargenOwnedBlobsAreReproducible$/^yaml$' -count=1
go test ./grammars -run '^TestYAMLSingleQuotedMultilineScalarWithoutIndent$' -count=1
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace/cgo_harness && go test . -tags treesitter_c_parity \
  -run '^TestYAMLOwnedGrammarLockedCParity$' -count=1 -parallel 1"
```

The reproducibility test requires the regenerated blob to decode to the same
`*gotreesitter.Language` as the shipped blob (see
`grammargen/blob_reproducibility_test.go` for why the check compares decoded
tables, not raw bytes). The C test requires matching tree shape, spans,
fields, and node names.

## Go ownership

Go is Tier 1 and Own. Its canonical grammar source is
[go_grammar.go](../grammargen/go_grammar.go). Its canonical external scanner
is [go_scanner.go](../grammars/go_scanner.go), which resolves automatic
semicolon insertion (ASI).

Regenerate the blob with:

```sh
go run ./cmd/grammargen -bin grammars/grammar_blobs/go.bin go
```

Do not pass `-lr-split`. Go declares one external symbol
(`_automatic_semicolon`). An external symbol switches `-lr-split` to
`usePreciseExternalBuilder` in `grammargen/lr.go`. That builder produces a
table that misparses ordinary single-line if-statements, for example
`if err != nil { foo() }` (confirmed 2026-09-20). The statement's `{ foo() }`
block attaches as an orphaned sibling, not as the `if` statement's body. See
[grammargen/README.md](../grammargen/README.md#authoring-commands) for the
mechanism.

Run the focused evidence with:

```sh
go test ./grammargen -run '^TestGrammargenOwnedBlobsAreReproducible$/^go$' -count=1
bash cgo_harness/docker/run_single_grammar_parity.sh go_lang --no-build
bash cgo_harness/docker/run_parity_in_docker.sh --no-build \
  -- "cd /workspace/cgo_harness && go test . -tags 'treesitter_c_parity gts_parsercorephase0' \
  -run '^(TestGoCompactIncrementalExecution.*Parity|TestGoCompactRecoveryVersionTurnsLockedC|TestStage6GoCompactCertification)$' \
  -count=1 -parallel 1"
```

`TestGrammargenOwnedBlobsAreReproducible` requires the regenerated blob to
decode to the same `*gotreesitter.Language` as the shipped blob. The Docker
suites require matching tree shape, spans, fields, node names, and the
certified compact/recovery route.

Regenerating the Go blob moves internal LALR state and production numbers.
These numbers are not part of the public contract. A handful of
`gts_parsercorephase0`-tagged tests in the root package pin them anyway, as
does `grammars/runtime/runtime_profiles.go`'s `"go"` certification entry.
Some pin the exact blob SHA-256; a few also pin exact internal state IDs or
work-item counts. Update all of them together with the blob. Run
`rg -n "<old-sha256-prefix>"` to find every pinned site.

## Regex ownership

Regex is grammargen-owned (`grammars/registry_builtin_gen.go`'s `"regex"`
entry sets `GrammarSourceGrammargenBlob`), but it has no
`cmd/grammargen` builtin grammar function. Its canonical source is
upstream's own resolved `grammar.json`, imported with `ImportGrammarJSON`.
Regex is not in the Tier 1 set.

The pinned upstream commit is `b2ac15e27fce703d2f37a79ccd94a5c0cbe9720b`
(tag `0.25.0`) of
[tree-sitter/tree-sitter-regex](https://github.com/tree-sitter/tree-sitter-regex),
MIT license, matching `grammars/languages.lock`'s `regex` entry. A pinned
copy of that commit's `src/grammar.json` lives at
[regex_upstream_grammar.json](../grammargen/testdata/regex_upstream_grammar.json)
(sha256
`a2e6cef007b68b11ea646d866e58747686b167c24789c1092958e9c77b27c6f6`), so the
reproducibility test does not need network access or a live checkout.

Regenerate the blob with:

```sh
go run ./cmd/grammargen -json grammargen/testdata/regex_upstream_grammar.json \
  -bin grammars/grammar_blobs/regex.bin
```

Do not set `BinaryRepeatMode` or `EnableLRSplitting`. `regex.bin` was
regenerated on 2026-09-20 with that plain recipe; the shipped grammar.json
is already the *resolved* form `tree-sitter generate` produces, so repeats
are already lowered and the DSL-only `BinaryRepeatMode` flag does not apply.
`TestRegexImportCharacterClassRangeParity` separately sets
`BinaryRepeatMode = true` before parsing two samples — that is a smaller,
independently-correct construction it uses for a behavioral check, not the
recipe that reproduces the shipped blob's tables.

Run the focused evidence with:

```sh
go test ./grammargen -run '^TestGrammargenOwnedBlobsAreReproducible$/^regex$' -count=1
go test ./grammargen -run '^TestRegexImportCharacterClassRangeParity$' -count=1
bash cgo_harness/docker/run_single_grammar_parity.sh regex
```

`TestGrammargenOwnedBlobsAreReproducible` requires the regenerated blob to
decode to the same `*gotreesitter.Language` as the shipped blob. The Docker
run compares the imported grammar's real-corpus parse output against the
tree-sitter C parser for 25 regex-pattern samples.

## Change and release gate

Use this sequence for every Lead or Own grammar change:

1. Capture a minimal valid source.
2. Add a realistic user fixture.
3. Change the canonical source layer.
4. Regenerate every affected artifact.
5. Build the matching C oracle.
6. Compare full trees and classify each difference.
7. Measure parse time, allocation, and peak memory.
8. Ship when the gates pass.

Do not edit a generated blob by hand. Do not adopt an upstream update without
the same evidence. Preserve the local fixture when upstream absorbs a fix.

## Automated lock updates

The weekly `grammar-lock-update` workflow plans every candidate commit, then
runs `cmd/grammar_update_guard` before it writes the lock. The guard blocks
only grammars whose scanner-facing files or external tokens changed
upstream. `cmd/grammar_updater` applies every other cleared update and holds
back just the blocked grammars, keeping their old lock commit. The pull
request body lists each held-back grammar's old and new refs and changed
files under "Held back" until a hand-written Go scanner port clears it.

## Automation rule

Use a manifest entry for each pinned overlay. Do not add grammar-name branches
for individual fixes. An overlay must fail when it no longer applies.

Use language-specific Docker parity runs for C validation. Run one grammar at
a time while a failure is under investigation.
