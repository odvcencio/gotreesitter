# Supported languages

206 grammars ship in the registry. All 206 produce error-free parse trees on smoke samples. Run `go run ./cmd/parity_report` for current status.

- 119 external scanners (hand-written Go implementations of upstream C scanners)
- 7 hand-written Go token sources (authzed, c, cpp, go, java, json, lua)
- Remaining languages use the DFA lexer generated from grammar tables

## Parse quality

Each `LangEntry` carries a `Quality` field:

| Quality | Meaning |
|---|---|
| `full` | All scanner and lexer components present. Parser has full access to the grammar. |
| `partial` | Missing external scanner. DFA lexer handles what it can; external tokens are skipped. |
| `none` | Cannot parse. |

`full` means the parser has every component the grammar requires. It does
not guarantee error-free trees on all inputs — grammars with high GLR
ambiguity may produce syntax errors on very large or deeply nested
constructs, because of parser safety limits (iteration cap, stack depth cap,
node count cap). These limits scale with input size. Check
`tree.RootNode().HasError()` at runtime.

<details>
<summary>Full language list (206)</summary>

`ada`, `agda`, `angular`, `apex`, `arduino`, `asm`, `astro`, `authzed`, `awk`, `bash`, `bass`, `beancount`, `bibtex`, `bicep`, `bitbake`, `blade`, `brightscript`, `c`, `c_sharp`, `caddy`, `cairo`, `capnp`, `chatito`, `circom`, `clojure`, `cmake`, `cobol`, `comment`, `commonlisp`, `cooklang`, `corn`, `cpon`, `cpp`, `crystal`, `css`, `csv`, `cuda`, `cue`, `cylc`, `d`, `dart`, `desktop`, `devicetree`, `dhall`, `diff`, `disassembly`, `djot`, `dockerfile`, `dot`, `doxygen`, `dtd`, `earthfile`, `ebnf`, `editorconfig`, `eds`, `eex`, `elisp`, `elixir`, `elm`, `elsa`, `embedded_template`, `enforce`, `erlang`, `facility`, `faust`, `fennel`, `fidl`, `firrtl`, `fish`, `foam`, `forth`, `fortran`, `fsharp`, `gdscript`, `git_config`, `git_rebase`, `gitattributes`, `gitcommit`, `gitignore`, `gleam`, `glsl`, `gn`, `go`, `godot_resource`, `gomod`, `graphql`, `groovy`, `hack`, `hare`, `haskell`, `haxe`, `hcl`, `heex`, `hlsl`, `html`, `http`, `hurl`, `hyprlang`, `ini`, `janet`, `java`, `javascript`, `jinja2`, `jq`, `jsdoc`, `json`, `json5`, `jsonnet`, `julia`, `just`, `kconfig`, `kdl`, `kotlin`, `ledger`, `less`, `linkerscript`, `liquid`, `llvm`, `lua`, `luau`, `make`, `markdown`, `markdown_inline`, `matlab`, `mermaid`, `meson`, `mojo`, `move`, `nginx`, `nickel`, `nim`, `ninja`, `nix`, `norg`, `nushell`, `objc`, `ocaml`, `odin`, `org`, `pascal`, `pem`, `perl`, `php`, `pkl`, `powershell`, `prisma`, `prolog`, `promql`, `properties`, `proto`, `pug`, `puppet`, `purescript`, `python`, `ql`, `r`, `racket`, `regex`, `rego`, `requirements`, `rescript`, `robot`, `ron`, `rst`, `ruby`, `rust`, `scala`, `scheme`, `scss`, `smithy`, `solidity`, `sparql`, `sql`, `squirrel`, `ssh_config`, `starlark`, `svelte`, `swift`, `tablegen`, `tcl`, `teal`, `templ`, `textproto`, `thrift`, `tlaplus`, `tmux`, `todotxt`, `toml`, `tsx`, `turtle`, `twig`, `typescript`, `typst`, `uxntal`, `v`, `verilog`, `vhdl`, `vimdoc`, `vue`, `wat`, `wgsl`, `wolfram`, `xml`, `yaml`, `yuck`, `zig`

</details>

## Query API

| Feature | Status |
|---|---|
| Compile + execute (`NewQuery`, `Execute`, `ExecuteNode`) | supported |
| Cursor streaming (`Exec`, `NextMatch`, `NextCapture`) | supported |
| Structural quantifiers (`?`, `*`, `+`) | supported |
| Alternation (`[...]`) | supported |
| Field matching (`name: (identifier)`) | supported |
| `#eq?` / `#not-eq?` | supported |
| `#match?` / `#not-match?` | supported |
| `#any-of?` / `#not-any-of?` | supported |
| `#lua-match?` | supported |
| `#has-ancestor?` / `#not-has-ancestor?` | supported |
| `#has-parent?` / `#not-has-parent?` | supported |
| `#is?` / `#is-not?` | supported |
| `#any-eq?` / `#any-not-eq?` | supported |
| `#any-match?` / `#any-not-match?` | supported |
| `#select-adjacent!` | supported |
| `#strip!` | supported |
| `#set!` / `#offset!` directives | parsed and accepted |
| `SetValues` (read `#set!` metadata from matches) | supported |

All shipped highlight and tags queries compile (`156/156` highlight, `69/69` tags).

## Adding a language

### Optional native Lean grammar

The Lean 4 grammar is an opt-in package during its default-fleet graduation.
It is authored with grammargen's Go domain-specific language (DSL). It does
not import another tree-sitter grammar.

```go
import _ "github.com/odvcencio/gotreesitter/grammars/lean"
```

The import registers `.lean`, `lean`, and `lean4` detection. Direct users can
call `lean.Language()` instead. The package includes highlights, outline tags,
and a Go scanner for nested comments.

Lean modules can extend their parser at runtime. The fixed grammar gives core
declarations stable nodes and preserves extension-specific lines as
`custom_command` nodes.

> Adding a language to your own project does **not** require forking this
> repo or following the in-tree steps below. See
> [docs/authoring-languages.md](authoring-languages.md) for the
> out-of-tree pipeline (grammar.json → blob → `LoadLanguage` /
> `RegisterExtension`) and
> [docs/external-scanners.md](external-scanners.md) for external
> scanners in Go. The steps below are for grammars embedded in this repo.

1. Add the grammar repo to `grammars/languages.manifest`
2. Refresh pinned refs in `grammars/languages.lock`:
   `go run ./cmd/grammar_updater -lock grammars/languages.lock -write -report grammars/grammar_updates.json`
3. Generate tables: `go run ./cmd/ts2go -manifest grammars/languages.manifest -outdir ./grammars -package grammars -compact=true`
4. Add smoke samples to `grammars/smoke_samples.go` and `grammars/parse_support_test.go`
5. Verify: `go run ./cmd/parity_report && go test ./grammars/...`

## Grammar lock updates

- `grammars/languages.lock` stores pinned refs for grammar update + parity automation.
- `cmd/grammar_updater` refreshes refs and emits a machine-readable report.
- `.github/workflows/grammar-lock-update.yml` opens scheduled/dispatch update PRs.
- Hand-written scanner ports can also declare `ExternalScannerSpec` metadata
  with upstream source hashes and external-token names. When a grammar update
  changes `src/scanner.c` or the external-token list, treat it as scanner
  work: update the Go scanner binding/port before replacing generated blobs.
  A grammar JSON-only change with unchanged externals can usually follow the
  normal `grammar.json -> grammargen Go DSL -> blob -> parity` path.

Manual refresh:

```sh
go run ./cmd/grammar_updater \
  -lock grammars/languages.lock \
  -allow-list grammars/update_tier1_core100.txt \
  -max-updates 10 \
  -write \
  -report grammars/grammar_updates.json
```
