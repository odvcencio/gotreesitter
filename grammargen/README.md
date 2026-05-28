# grammargen

`grammargen` is the pure-Go grammar compiler used by gotreesitter. It turns a
grammar definition into a `*gotreesitter.Language`, a serialized `.bin` blob, a
tree-sitter-compatible `parser.c`, or generated Go DSL source.

The authoring surface is intentionally input-neutral:

- Go DSL grammars built with `NewGrammar`, `Define`, `Seq`, `Choice`, `Token`,
  `Field`, `PrecLeft`, and related constructors.
- Resolved upstream `grammar.json` files, which are the preferred import format
  for tree-sitter grammars.
- `.grammar` files, a compact ecosystem-agnostic grammar format that parses
  into the same IR and can emit Go DSL.

`grammar.js` import also exists, but `grammar.json` is usually more reliable
because helper functions, `require()` calls, and JavaScript evaluation have
already been resolved by tree-sitter.

## Authoring Commands

Use `doctor` when changing a grammar. It validates, generates parser tables,
runs embedded tests when present, optionally parses a sample, and suggests the
next command.

```sh
go run ./cmd/grammargen doctor calc -text '1+2*3'
go run ./cmd/grammargen doctor -json /tmp/grammar_parity/go/src/grammar.json -sample sample.go
go run ./cmd/grammargen doctor -grammar ./mini.grammar -text '123'
go run ./cmd/grammargen doctor calc -text '1+2*3' -conflicts 3
go run ./cmd/grammargen doctor calc -text '1+2*3' -format json
```

Use `parse` when you want quick sample-to-tree feedback:

```sh
go run ./cmd/grammargen parse calc -text '1+2*3'
go run ./cmd/grammargen parse -grammar ./mini.grammar -stdin
go run ./cmd/grammargen parse calc -text '1+2*3' -format sexpr
go run ./cmd/grammargen parse calc -text '1+2*3' -format json
```

Use `emit` to write artifacts from any supported input:

```sh
# gotreesitter blob
go run ./cmd/grammargen emit go -bin grammars/grammar_blobs/go.bin

# Go DSL source from a resolved grammar.json
go run ./cmd/grammargen emit \
  -json /tmp/grammar_parity/go/src/grammar.json \
  -go grammargen/go_grammar.go \
  -pkg grammargen \
  -func GoGrammar

# Go DSL source from a .grammar file
go run ./cmd/grammargen emit -grammar ./mini.grammar -go ./mini_grammar.go -pkg grammargen

# Resolved grammar.json from any supported input
go run ./cmd/grammargen emit -grammar ./mini.grammar -json-out ./mini.grammar.json

# tree-sitter parser.c
go run ./cmd/grammargen emit calc -c /tmp/parser.c

# inferred highlight query
go run ./cmd/grammargen emit calc -highlight
```

For golden parse snapshots, write the current tree once, then compare future
runs against it:

```sh
go run ./cmd/grammargen parse calc -text '1+2*3' -write-expect ./calc.sexpr
go run ./cmd/grammargen parse calc -text '1+2*3' -expect ./calc.sexpr
```

`parse -strict` exits non-zero when parsing finishes with `ERROR` nodes or an
early stop condition. `doctor` treats sample parse errors as gate failures by
default.

The legacy flag surface still works:

```sh
go run ./cmd/grammargen -validate calc
go run ./cmd/grammargen -report calc
go run ./cmd/grammargen -grammar ./mini.grammar -go ./mini_grammar.go
```

For grammars that benefit from local LR(1) state splitting, pass `-lr-split`:

```sh
go run ./cmd/grammargen doctor go -lr-split -sample sample.go
go run ./cmd/grammargen emit go -lr-split -bin grammars/grammar_blobs/go.bin
```

## Go DSL

The first defined rule is the start rule. Names beginning with `_` are hidden
rules. String rules create literal tokens, pattern rules create regex terminals,
and `Token` groups a rule into one lexer token.

```go
func MiniExprGrammar() *Grammar {
	g := NewGrammar("mini_expr")

	g.Define("program", Sym("expression"))
	g.Define("expression", Choice(
		PrecLeft(1, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("+")),
			Field("right", Sym("expression")),
		)),
		PrecLeft(2, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("*")),
			Field("right", Sym("expression")),
		)),
		Sym("number"),
		Seq(Str("("), Sym("expression"), Str(")")),
	))
	g.Define("number", Token(Repeat1(Pat(`[0-9]`))))
	g.SetExtras(Pat(`\s`))

	g.Test("precedence", "1 + 2 * 3", "")

	return g
}
```

An embedded test with an empty expected S-expression only checks that parsing
finishes without `ERROR` nodes. Fill in the expected S-expression when a rule's
exact tree shape should be locked down.

Common grammar-level settings:

- `SetExtras(...)`: whitespace, comments, or other extra tokens.
- `SetConflicts(...)`: declared ambiguity groups that should keep GLR
  alternatives.
- `SetExternals(...)`: external scanner tokens.
- `SetInline(...)`: rules to inline during normalization.
- `SetWord(...)`: word token used for keyword extraction.
- `SetSupertypes(...)`: structural supertypes exposed in metadata.
- `Precedences`: ordered named and symbol precedence levels imported from
  `grammar.json`.

Useful DSL helpers live in `grammar.go`: `CommaSep`, `CommaSep1`, `SepBy`,
`SepBy1`, `Parens`, `Brackets`, `Braces`, `AppendChoice`, and `ExtendGrammar`.

## `.grammar` Files

`.grammar` is the ecosystem-agnostic text format. It is currently line-oriented,
so keep each rule definition on one line.

```text
grammar mini

extras = [ /\s/ ]

rule program = number
rule number = token(repeat1(/[0-9]/))
```

Run it through the same command surface:

```sh
go run ./cmd/grammargen doctor -grammar ./mini.grammar -text '123'
go run ./cmd/grammargen parse -grammar ./mini.grammar -text '123'
go run ./cmd/grammargen emit -grammar ./mini.grammar -go ./mini_grammar.go -pkg grammargen
go run ./cmd/grammargen emit -grammar ./mini.grammar -json-out ./mini.grammar.json
go run ./cmd/grammargen emit -grammar ./mini.grammar -bin /tmp/mini.bin
```

Supported top-level lines:

```text
grammar <name>
extras = [ <rule-expr>, ... ]
word = <rule_name>
supertypes = [ <rule_name>, ... ]
conflicts = [ [<rule>, <rule>], ... ]
rule <name> = <rule-expr>
```

Supported expressions:

```text
"literal"
/regex/
identifier

seq(a, b, ...)
choice(a, b, ...)
repeat(a)
repeat1(a)
optional(a)
token(a)
field("name", a)
prec(1, a)
prec.left(1, a)
prec.right(1, a)
prec.dynamic(1, a)
alias(a, name)
alias(a, "anonymous_name")
```

For large upstream grammars, resolved `grammar.json` remains the most complete
input. `.grammar` is the portable authoring format and should stay independent
of any host language syntax.

## Validation Loop

For small package-local checks, keep tests focused:

```sh
go test ./cmd/grammargen ./grammargen \
  -run '^TestJSONGenerate$|^TestGenerateWithReportCtxSkipsDiagnosticsWhenNotRequested$' \
  -count=1
```

When changing GLR, incremental, import, or parity-sensitive behavior, use the
Docker parity runners and keep runs to one grammar at a time:

```sh
# Focused package test inside Docker
bash cgo_harness/docker/run_parity_in_docker.sh \
  -- "cd /workspace && go test ./grammargen -run '^TestName$' -count=1"

# Real-corpus parity for one grammar
bash cgo_harness/docker/run_single_grammar_parity.sh typescript

# Focused grammargen real-corpus lane
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs typescript

# Focused grammargen-vs-C lane
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs typescript
```

Do not run repo-wide `go test ./...` or broad race sweeps on the host for
grammargen work. Heavy correctness, parity, and race coverage belongs in Docker
or CI, scoped to one language or one regression at a time.

## Reading the Package

- `grammar.go`: public IR and Go DSL constructors.
- `parse_grammar_file.go`: `.grammar` parser.
- `import_grammarjson.go`: resolved tree-sitter `grammar.json` import.
- `import_grammarjs.go`: best-effort `grammar.js` import.
- `normalize.go`: rule lowering, metadata, fields, terminals, and production
  construction.
- `lr.go`: LR/LALR table construction and conflict resolution.
- `lr_split.go`, `lr_split_oracle.go`: local LR(1) split support.
- `dfa.go`, `nfa.go`, `regex.go`: lexer construction.
- `encode.go`, `assemble.go`: `Language` assembly and blob encoding.
- `diagnostics.go`: validation, embedded tests, and generation reports.
- `emit_grammar_go.go`, `export_grammarjson.go`, `codegen_c.go`: artifact
  emitters.
- `parity_test.go`, `parity_real_corpus_test.go`: generated-vs-reference
  parity infrastructure.

## Troubleshooting

Start with `doctor`. It reports validation warnings, generation failures, table
sizes, conflict count, embedded test status, and sample parse status. Add
`-conflicts N` when precedence or GLR behavior needs inspection, or
`-format json` when another tool should consume the report.

Use `parse` when a grammar generates but the tree shape looks wrong. It prints
the root type, byte range, error flag, stop reason, and named-node
S-expression. Use `-format sexpr` or `-expect`/`-write-expect` for golden tree
snapshots.

For upstream imports, prefer `src/grammar.json` from a generated tree-sitter
repository. If import fails on `grammar.js`, regenerate or locate the resolved
JSON first.

External-scanner grammars need a compatible Go scanner binding in `grammars/`.
The generated grammar can expose external tokens, but scanner behavior is still
hand-written runtime code.

When corpus parity fails, narrow before changing generator behavior: one
language, one focused test, one sample if possible. Use `GTS_GRAMMARGEN_REAL_CORPUS_ONLY`,
`GTS_GRAMMARGEN_REAL_CORPUS_MAX_CASES`, and the focused Docker runners to keep
the workload reproducible and attributable.
