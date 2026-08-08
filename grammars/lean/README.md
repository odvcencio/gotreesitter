# Native Lean grammar

This package ships an opt-in Lean 4 grammar. The grammar uses the repository's
Go domain-specific language (DSL). It targets Lean 4.32.2.

Import the package to register `.lean`, `lean`, and `lean4` detection:

```go
import _ "github.com/odvcencio/gotreesitter/grammars/lean"
```

Call `lean.Language()` when you only need the language tables.

## Scope

Lean modules can add syntax, notation, macros, and parser extensions at
runtime. A fixed grammar cannot reproduce all imported syntax.

This grammar provides stable nodes for core commands and declarations. It
keeps extension-specific syntax in line-scoped `custom_command` nodes.

The package also provides:

- nested block comments;
- documentation comments;
- highlight captures;
- outline tags;
- incremental parsing support.

## Regenerate the blob

Run this command from the repository root:

```sh
go run ./cmd/grammargen emit \
  -lr-split \
  -bin grammars/lean/lean.bin \
  lean
```

`TestPackagedBlobMatchesNativeGrammar` rejects a stale generated blob.

## Run the corpus gate

Use the Lean 4.32.2 source at commit
`f3b06c705e6c85f5314019d5d3baab0fec5b580c`.

```sh
LEAN4_ROOT=/path/to/lean4
GOT_LEAN_CORPUS_ROOTS="$LEAN4_ROOT/src:$LEAN4_ROOT/tests/elab:$LEAN4_ROOT/tests/lean" \
  go test ./grammars/lean \
  -run '^TestOfficialLeanCorpusCharacterization$' \
  -count=1 \
  -v
```

The current gate covers 3,699 files and 15,540,447 bytes. All files parse
without recovery nodes or early stops.

The grammar remains opt-in. This keeps the default 206-language graduation
gates unchanged. Promote it to the default registry only after the fleet
receipts move to 207 languages.
