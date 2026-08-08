# Adding a language without forking

This guide is for grammar authors who have a working tree-sitter grammar (a
`grammar.js` that `tree-sitter generate` accepts) and want gotreesitter to
parse it — as a library dependency, not as a fork.

Historical context, because it shaped this document: a downstream project
(pawnkit, a Pawn toolchain) once forked this repo and renamed the module
just to add one line to a then-unexported language-name switch that
controlled the GLR forest fast path. v0.20.8 (#134) closed that gap: forest
opt-in is now a public `Language.WantsForest` field, a
`grammargen.Grammar.WantsForest` flag, and a declarative `"gotreesitter"`
object in `grammar.json`. Everything in this document works from your own
module against an unmodified `github.com/odvcencio/gotreesitter`.

The in-tree workflow (README "Adding a language") is for grammars shipped
inside this repo's 200+ embedded set. You do not need it.

## The pipeline

```
grammar.js ──grammargen -js-cli──▶ temporary grammar.json
                                      │
grammar.json ──ImportGrammarJSON──────┴──▶ *grammargen.Grammar (IR)
                                          │
                              GenerateLanguageAndBlob
                                          │
                        ┌─────────────────┴──────────────────┐
                        ▼                                    ▼
             *gotreesitter.Language                     blob []byte
             (parse right now)                 (write to disk / go:embed)
                                                             │
                                              gotreesitter.LoadLanguage
                                              grammars.LoadLanguage
                                              taproot.ParseFromBlob
```

The relevant functions are all public:

| Function | Where | Purpose |
|---|---|---|
| `grammargen.ImportGrammarJSON(data []byte) (*Grammar, error)` | `grammargen/import_grammarjson.go` | Parse a resolved `grammar.json` (the output of `tree-sitter generate`) into the grammar IR. Rules, extras, conflicts, externals, inline, word, precedences, reserved sets, supertypes are all imported. |
| `grammargen.GenerateLanguage(g *Grammar) (*gotreesitter.Language, error)` | `grammargen/encode.go` | Compile the IR into runtime parse tables. |
| `grammargen.GenerateLanguageAndBlob(g *Grammar) (*gotreesitter.Language, []byte, error)` | `grammargen/encode.go` | Same, plus a serialized language blob in one pass. Blobs without `LargeStateGotos` retain the legacy gob+gzip format; map-bearing blobs use a deterministic versioned envelope. Load either form with `gotreesitter.LoadLanguage`. `...WithContext` variants exist for cancellation. |
| `grammargen.EmitGrammarGo(g *Grammar, pkgName, funcName string) ([]byte, error)` | `grammargen/emit_grammar_go.go` | Emit Go DSL source that reconstructs the grammar (useful for vendoring the grammar as reviewable Go code instead of JSON). |
| `gotreesitter.LoadLanguage(data []byte) (*Language, error)` | `load_language.go` | Deserialize a blob at runtime. The only function needed to load a pre-compiled grammar — no grammargen import, no registry. |
| `grammars.LoadLanguage(name string, data []byte)` | `grammars/embedded_loader.go` | Like the above, but also attaches any external scanner / external lex-state tables registered for `name` in the `grammars` registry. |
| `grammars.Register` / `grammars.RegisterExtension` | `grammars/registry.go` | Put your language into the shared registry: file-extension detection, markdown fence aliases, highlight queries. |
| `taproot.Parse` / `taproot.ParseFromBlob` | `taproot/taproot.go` | One-call front-end: language generation + caching + parse + syntax-error formatting. |

## Step 1 — get a Grammar IR

### Option A: import grammar.json

`tree-sitter generate` writes the fully resolved grammar to
`src/grammar.json`. That file — not `grammar.js` — is the canonical input;
it has no `require()` calls or helper functions to interpret.

```go
data, err := os.ReadFile("src/grammar.json")
if err != nil {
    log.Fatal(err)
}
g, err := grammargen.ImportGrammarJSON(data)
if err != nil {
    log.Fatal(err)
}
```

### Option B: resolve grammar.js with Tree-sitter

Use `-js-cli` when the repository does not contain a resolved `grammar.json`.
This explicit mode needs Tree-sitter 0.26 or newer on `PATH`.

> **Warning:** This command evaluates `grammar.js` and its imports as
> JavaScript. Use it only with code that you trust.

```sh
go run ./cmd/grammargen doctor \
  -js-cli ./grammar.js \
  -sample ./examples/example.txt

go run ./cmd/grammargen emit \
  -js-cli ./grammar.js \
  -bin ./language.bin
```

The command asks Tree-sitter to generate only its structured files. It imports
the temporary `grammar.json` through `ImportGrammarJSON`, then removes it.

The grammar's external token declarations stay in the imported grammar. You
must still register a compatible Go external scanner for runtime parsing.

The older `-js` flag remains a best-effort pure-Go importer. It does not execute
JavaScript, but it cannot resolve all helpers or `require()` calls.

### Option C: write the Go DSL directly

For small DSLs there is no need for a JavaScript grammar at all. The
builder functions live in `grammargen/grammar.go`: `NewGrammar`, `Define`,
`Str`, `Pat`, `Sym`, `Seq`, `Choice`, `Repeat`, `Repeat1`, `Optional`,
`Token`, `ImmToken`, `Field`, `Prec`, `PrecLeft`, `PrecRight`,
`PrecDynamic`, `Alias`, plus combinators like `CommaSep1` and `SepBy1`, and
grammar-level setters `SetExtras`, `SetConflicts`, `SetExternals`,
`SetInline`, `SetWord`, `SetSupertypes`.

The rest of this guide uses this toy key/value config language:

```go
package kvconf

import "github.com/odvcencio/gotreesitter/grammargen"

// Grammar returns the kvconf grammar. First rule defined = start rule.
func Grammar() *grammargen.Grammar {
    g := grammargen.NewGrammar("kvconf")

    g.Define("document", grammargen.Repeat(grammargen.Sym("entry")))

    g.Define("entry", grammargen.Seq(
        grammargen.Field("key", grammargen.Sym("identifier")),
        grammargen.Str("="),
        grammargen.Field("value", grammargen.Sym("_value")),
    ))

    // Leading "_" makes a rule hidden (no node in the tree).
    g.Define("_value", grammargen.Choice(
        grammargen.Sym("number"),
        grammargen.Sym("string"),
    ))

    g.Define("identifier", grammargen.Pat(`[A-Za-z_][A-Za-z0-9_]*`))
    g.Define("number", grammargen.Pat(`[0-9]+`))
    g.Define("string", grammargen.Pat(`"[^"]*"`))
    g.Define("comment", grammargen.Token(grammargen.Pat(`#[^\n]*`)))

    // Extras are skipped/attached anywhere: whitespace pattern + comments.
    g.SetExtras(grammargen.Pat(`\s`), grammargen.Sym("comment"))

    return g
}
```

## Step 2 — generate and parse

Here is a complete program: it compiles the grammar, writes the blob, and
parses a sample.

```go
package main

import (
    "fmt"
    "log"
    "os"

    gts "github.com/odvcencio/gotreesitter"
    "github.com/odvcencio/gotreesitter/grammargen"

    "example.com/kvconf"
)

func main() {
    lang, blob, err := grammargen.GenerateLanguageAndBlob(kvconf.Grammar())
    if err != nil {
        log.Fatalf("generate: %v", err)
    }
    if err := os.WriteFile("kvconf.bin", blob, 0o644); err != nil {
        log.Fatal(err)
    }

    src := []byte("name = \"fable\"\nport = 8080\n")
    tree, err := gts.NewParser(lang).Parse(src)
    if err != nil {
        log.Fatalf("parse: %v", err)
    }
    root := tree.RootNode()
    fmt.Println(root.SExpr(lang))
    // (document (entry (identifier) (string)) (entry (identifier) (number)))
    //
    // SExpr prints named nodes only (no anonymous "=", no field prefixes).
    // Fields are queried structurally: entryNode.ChildByFieldName("key", lang).
    if root.HasError() {
        log.Fatal("syntax errors in sample")
    }
}
```

Generation for a small grammar takes milliseconds; for large imported
grammars it can take seconds to minutes (see "Known limits"). That is why
you generate the blob once, at build time, and ship the blob.

The same generation is available from the CLI, including a `grammar.json`
front-end and an authoring `doctor`:

```sh
# grammar.json -> blob
go run ./cmd/grammargen emit -json src/grammar.json -bin kvconf.bin

# grammar.json -> Go DSL source (vendor the grammar as code)
go run ./cmd/grammargen emit -json src/grammar.json -go kvconf_grammar.go -pkg kvconf

# validate + generate + run embedded tests + parse a sample, with a report
go run ./cmd/grammargen doctor -json src/grammar.json -sample testdata/example.kvconf
```

(In this repo, prefix `GOWORK=off` to `go run`/`go test` commands.)

## Step 3 — load the blob at runtime

Three doors, cheapest first:

**Registry-free, grammargen-free (recommended for DSL tools).** This links
only the runtime; it does not link the ~200-grammar registry.

```go
//go:embed kvconf.bin
var kvconfBlob []byte

lang, err := gts.LoadLanguage(kvconfBlob) // load_language.go
tree, err := gts.NewParser(lang).Parse(src)
```

Or use `taproot/walk`, which adds caching and a CST walker but still needs
no registry:

```go
root, w, err := walk.ParseFromBlob("kvconf", kvconfBlob, src)
```

**taproot with generation fallback.** `taproot.ParseFromBlob` loads the
blob when present and falls back to building from the DSL when the blob is
empty or corrupt; results are cached per name:

```go
root, w, err := taproot.ParseFromBlob("kvconf", kvconfBlob, kvconf.Grammar, src)
// or, no blob at all:
root, w, err = taproot.Parse("kvconf", kvconf.Grammar, src)
```

`taproot.Language(name, build)` and `taproot.LanguageFromBlob(name, blob,
build)` are the underlying language-only entry points.

**Via the grammars registry.** Use `grammars.LoadLanguage(name, blob)`
instead of `gotreesitter.LoadLanguage(blob)` when the language has an
external scanner or lex-state table registered under `name` — it calls
`grammars.AttachLanguageSupport` for you (see `docs/external-scanners.md`).

## Distributing a language as its own Go module

This is the pawnkit counterfactual: what the fork should have been — a
small module that depends on gotreesitter and registers itself. Here is
the module layout:

```
github.com/pawnkit/gotreesitter-pawn/
├── go.mod                  // require github.com/odvcencio/gotreesitter vX.Y.Z
├── pawn.bin                // generated blob, checked in
├── grammar.json            // the source of truth the blob was generated from
├── queries/highlights.scm
├── scanner.go              // Go port of scanner.c (see docs/external-scanners.md)
└── pawn.go                 // the code below
```

```go
// Package pawn registers the Pawn grammar with gotreesitter.
// Import for side effects:
//
//	import _ "github.com/pawnkit/gotreesitter-pawn"
package pawn

import (
    _ "embed"
    "sync"

    gts "github.com/odvcencio/gotreesitter"
    "github.com/odvcencio/gotreesitter/grammars"
)

//go:embed pawn.bin
var pawnBlob []byte

//go:embed queries/highlights.scm
var highlightQuery string

//go:embed queries/tags.scm
var tagsQuery string

var (
    once    sync.Once
    lang    *gts.Language
    langErr error
)

// Language loads the embedded Pawn blob once and attaches the scanner.
func Language() (*gts.Language, error) {
    once.Do(func() {
        lang, langErr = gts.LoadLanguage(pawnBlob)
        if langErr != nil {
            return
        }
        // ExternalScanner is a public field on Language. grammargen-built
        // blobs already carry ExternalSymbols and ExternalLexStates; only
        // the scanner implementation itself lives in Go code.
        lang.ExternalScanner = NewPawnScanner(lang)
    })
    return lang, langErr
}

func init() {
    grammars.RegisterExtension(grammars.ExtensionEntry{
        Name:             "pawn",
        Extensions:       []string{".pwn", ".inc"},
        Aliases:          []string{"pawno"}, // markdown fence aliases
        GenerateLanguage: Language,
        GrammarSource:    grammars.GrammarSourceGrammargenBlob,
        HighlightQuery:   highlightQuery,
        TagsQuery:        tagsQuery,
    })
}
```

After `import _ "github.com/pawnkit/gotreesitter-pawn"`, everything the
registry powers works: `grammars.DetectLanguage("gamemodes/x.pwn")`,
`grammars.DetectLanguageByName("pawn")`, markdown fence highlighting
through the aliases, and `grammars.AllLanguages()` listing.

Semantics worth knowing (all from `grammars/registry.go`):

- **Replace-by-name.** `Register` replaces an existing entry with the same
  `Name`, so you can even shadow a built-in grammar with your own build.
  `RegisterExtension` is a thin wrapper over `Register` that adds a caching
  loader and fence aliases.
- **Extension collisions.** For file-suffix detection, the first registered
  entry owning a suffix wins (`buildExtIndex`). Built-ins register before
  your `init` runs, so a suffix already claimed by a built-in stays theirs
  unless you `Register` over that language name itself.
- **`RegisterExtension` has no `Shebangs` or `TokenSourceFactory` fields.**
  It accepts `GrammarSource` and `TagsQuery`; an empty `GrammarSource`
  defaults to `GrammarSourceGrammargen`. For the other fields, call
  `grammars.Register` directly with a full `grammars.LangEntry`. These are
  public fields, including `TokenSourceFactory func(src []byte, lang
  *gotreesitter.Language) gotreesitter.TokenSource` for hand-written token
  sources.
- **Runtime language gating.** If the process sets `GOTREESITTER_GRAMMAR_SET`
  (a comma-separated allow-list; empty/unset means allow all — see
  `grammars/language_set_runtime.go`), `Register` silently drops languages
  not in the set. If your language can be deployed into environments that
  use this variable, document that users must include your name. The
  `grammar_set_core` build tag applies a compile-time allow-list the same
  way.

## Declarative grammar.json options

gotreesitter reads one extension object from `grammar.json`, under a key
that tree-sitter's own tooling ignores:

```json
{
  "name": "pawn",
  "rules": { "...": "..." },
  "gotreesitter": { "wantsForest": true }
}
```

`ImportGrammarJSON` copies `wantsForest` into
`grammargen.Grammar.WantsForest`; generation copies it into
`gotreesitter.Language.WantsForest`; the field is gob-serialized, so it
survives the blob round trip. `ExportGrammarJSON` writes the object back
only when set, so standard grammars' JSON stays unchanged. Equivalently in
code: set `g.WantsForest = true` on the IR, or `lang.WantsForest = true` on
a loaded Language. `ExtendGrammar` inherits the flag from its base.

**What it does.** `WantsForest` opts the language into the GSS-forest GLR
fast path (`glr_forest.go`): a graph-structured-stack parse that coalesces
equivalent stack tops instead of forking full stacks. Built-in languages
get this through a curated, byte-parity-certified default map
(`builtinForestDefaults`); your language cannot join that map without a PR
— `WantsForest` is the supported alternative.

**When to enable it.** Enable it for ambiguity-heavy grammars: many
declared `conflicts`, heavy GLR forking, and deep expression nesting where
production GLR blows up on stack-equivalence checks (bash was the
motivating case). For a mostly deterministic LR grammar it buys little.

**Risk profile, stated honestly.** By default, the forest path declines
(falls back to the production parser) unless it produces a clean, complete
tree. What you can get is a *clean but different* tree on ambiguous
inputs, because your grammar bypasses the byte-range parity certification
built-ins undergo — that trade is explicitly yours (see the
`Language.WantsForest` doc comment in `language.go`). Forest error
recovery defaults are name-keyed; `GOT_GLR_FOREST_RECOVER=1` or
`gotreesitter.SetGLRForestRecover` enables recovery globally for
experiments and tests, so validate error-bearing trees separately when you
use either. `GOT_GLR_FOREST=0` disables the forest globally at runtime;
`gotreesitter.SetGLRForestEnabled` toggles it in tests.

Verify before shipping: parse your corpus twice, once with `WantsForest` on
and once off, and diff the S-expressions.

## External scanners

If your grammar has an `externals` array, the generated Language carries
`ExternalSymbols` and a precise `ExternalLexStates` validity table
automatically. Token recognition itself still needs a Go implementation of
the `gotreesitter.ExternalScanner` interface attached to
`Language.ExternalScanner` (both public). Without one, external tokens are
synthesized only in a narrow automatic-semicolon-style fallback, and parse
quality is "partial" at best. See [external-scanners.md](external-scanners.md)
for when to write one and how.

## What still requires an upstream PR

For the core flow — compile, load, register, parse, forest opt-in, external
scanner attachment — **nothing**. Things that genuinely still need a PR:

- **Becoming a built-in**: embedding your blob in `grammars/grammar_blobs/`,
  a registry entry with quality auditing, parity CI.
- **Hand-written TokenSource registered by name**: the name-keyed factory
  registry (`registerTokenSourceFactory` in
  `grammars/token_source_factory_registry.go`) is unexported. Out of tree,
  set `LangEntry.TokenSourceFactory` on your own `grammars.Register` entry
  and call
  `Parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))`
  — that is the same mechanism the repo's own tools use (for example
  `grep/compile.go`).
- **Import shape hints**: `applyImportGrammarShapeHints` in
  `grammargen/import_grammarjson.go` switches on the grammar *name* to
  apply per-language generation hints (`BinaryRepeatMode`,
  `ExactPrefixStates`, and similar settings). The good news: every one of
  those hints is also a public field on `grammargen.Grammar`, so you can
  set them yourself after `ImportGrammarJSON` returns. A PR is needed only
  to make a hint automatic for everyone importing your grammar by name.
- **Forest error-recovery default**: `languageWantsForestRecover`
  (`glr_forest.go`) is a name switch over byte-verified built-ins.
- **C-recovery parity certification defaults**
  (`Language.CRecoveryCostCompetitionEnabledByDefault`) — capability
  metadata is computed for generated languages, but the default-on
  certification is curated.

## Known limits: grammargen state budgets

Verified against `grammargen/lr.go` and `grammargen/assemble.go` at v0.25.0:

- **Runtime state IDs are uint32.** `StateID` was widened from uint16
  specifically for large grammars (COBOL generates ~67k states). The only
  hard cap on state count is uint32 max (`lr.go`, "Cap at uint32 max").
- **The precise external-scanner LR(1) builder has a 20,000-state budget**
  (`preciseStateBudget`, override with
  `GOT_LR_PRECISE_EXTERNAL_STATE_BUDGET`). Exceeding it — or exceeding
  65,535 item sets on that path — is *not* an error: generation
  transparently rebuilds with the DeRemer/Pennello LALR pipeline. You lose
  LR(1) precision (possible parse differences in scanner-adjacent states),
  not the build. Grammars with more than 5,000 productions skip the
  precise builder outright, as do grammars with 24 or more external
  tokens, unless `Grammar.PreferPreciseExternalLexStates` is set.
- **LALR LR0 budgets are opt-in and fail hard.** `GOT_LALR_LR0_STATE_BUDGET`
  / `GOT_LALR_LR0_CORE_BUDGET` (unset means unlimited) abort generation
  with `build LR tables: LALR LR0 state budget exceeded (...)`. Use them in
  CI to catch a grammar change that explodes the automaton, rather than
  discovering it as a multi-minute build.
- **Parse-action group indexes are uint16.** Table cells index into
  `ParseActions`, so a grammar needs to stay under 65,535 *distinct* action
  groups. Semantic deduplication in `buildParseTables`
  (`grammargen/assemble.go`) keeps even Markdown (50k+ productions, 78k+
  raw groups) under the limit. Generation fails with a `parse action
  group` uint16 table-limit error if dedup is insufficient.
- **No built-in generation timeout.** Pathological grammars can take a long
  time. Wrap generation with
  `grammargen.GenerateLanguageAndBlobWithContext(ctx, g)` and a deadline.

Scale reality check: pawnkit's real tree-sitter-pawn parser has 6,818
states, 333 symbols, 128 tokens, and 5 external tokens — comfortably inside
every budget above. You need a COBOL-class grammar before state budgets
become your problem.

## Blob provenance discipline

This is hard-learned; treat it as policy.

- A blob without `LargeStateGotos` uses the legacy gob+gzip format. A blob
  with a non-empty `LargeStateGotos` map uses a deterministic versioned
  envelope containing the gzip payload and a sorted map trailer. Always
  load either form with `gotreesitter.LoadLanguage` (or
  `grammars.LoadLanguage`) instead of assuming the bytes begin with a gzip
  header. The underlying gob data tolerates field drift silently: fields
  added since the blob was written decode as zero values, and removed
  fields are skipped. A stale blob usually still *loads* — and then
  misparses or loses features (a pre-0.20.8 blob has `WantsForest ==
  false` forever, and older blobs lack `ZeroWidthTokens`,
  `ConflictPolicies`, and other later fields).
  `Language.CompatibleWithRuntime()` only checks the tree-sitter ABI
  version (`LanguageVersion`, where 0 means unknown and compatible); it
  does **not** detect engine/blob skew.
- Therefore: **blobs are not portable across engine vintages. Regenerate
  the blob from `grammar.json` with the exact gotreesitter module version
  your binary links.** Check in the `grammar.json` next to the blob, and
  make regeneration a one-command script:

  ```sh
  go run github.com/odvcencio/gotreesitter/cmd/grammargen emit \
      -json grammar.json -bin pawn.bin
  ```

  Run it whenever you bump the gotreesitter dependency, and diff parse
  output over your corpus as the acceptance test.
- Decode paths differ slightly: `gotreesitter.LoadLanguage` and the
  `grammars` loader both run `InferGeneratedRepeatAuxMetadata`, but only
  the `grammars` loader applies its additional repair passes and scanner
  attachment. Load through one door consistently.

## Appendix: gaps (for maintainers)

This is an honest list of what an out-of-tree author still cannot do
cleanly, kept here so the docs and the backlog agree:

1. **No scanner-skeleton generation.** `grammargen` knows the externals
   list, but there is no `emit -scanner-skeleton` command to produce a Go
   `ExternalScanner` stub with the token-index constants and symbol
   resolution boilerplate from `docs/external-scanners.md`. Every author
   hand-writes the same 60 lines.
2. **TokenSource-by-name registry is unexported**
   (`grammars/token_source_factory_registry.go`). The workaround is
   documented above (`LangEntry.TokenSourceFactory`); exporting a
   `RegisterTokenSourceFactory` function would remove the need to
   re-`Register` the whole entry.
3. **The error-mode token source capability is package-private.**
   `errorModeLexingTokenSource` (`parser_api.go`) uses an unexported
   method, so third-party token sources cannot declare C-equivalent
   error-mode lexing even if they implement it. See external-scanners.md,
   contract (c).
