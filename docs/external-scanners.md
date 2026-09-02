# When you need an external scanner — and how to write one in Go

External scanners are the escape hatch tree-sitter grammars use when a
regular lexer cannot recognize a token. The grammar's `externals` array
names the tokens, and hand-written scanner code recognizes them with full
context. In upstream tree-sitter that code is `src/scanner.c`; in
gotreesitter it is a Go type that implements `gotreesitter.ExternalScanner`,
attached to `Language.ExternalScanner`.

This document is in two parts: first, deciding whether you need a scanner
at all (most grammars that think they do, don't); second, the Go porting
contract, including behavioral requirements this project learned the hard
way while chasing byte-exact parity with C tree-sitter.

Companion doc: [authoring-languages.md](authoring-languages.md) for the
grammar → blob → runtime pipeline itself.

## Part 1 — the decision

### You do NOT need a scanner for

- **Keyword versus identifier.** Declare a `word` token (`"word": "identifier"`
  in grammar.json, `g.SetWord("identifier")` in the DSL) and write keywords
  as plain strings. Keyword extraction handles the overlap.
- **Precedence and associativity.** `prec`, `prec.left`, `prec.right`,
  `prec.dynamic` (DSL: `Prec`, `PrecLeft`, `PrecRight`, `PrecDynamic`)
  resolve expression-grammar ambiguity in the tables. Dangling-else,
  cast-vs-call, and operator towers all stay in-grammar.
- **Comments and whitespace.** These are `extras`. gotreesitter compiles a
  whitespace pattern extra into DFA skip transitions and shifts visible
  extras (like `comment`) anywhere.
- **Plain strings and numbers.** `token(seq('"', /[^"]*/, '"'))` and similar
  patterns cover these. A string with a fixed delimiter and escape rules is
  regular.
- **C-style preprocessors.** tree-sitter-c parses `#if`/`#define`/`#include`
  entirely in-grammar: directives are line-oriented rules, and `\\\n`
  splices are part of token patterns. Having a preprocessor is not, by
  itself, a reason to write a scanner — Pawn needed scanner help only for
  two *recovery* cases, covered below.

If your grammar is in one of these buckets, stop here: every scanner you
don't write is corpus parity you don't have to re-verify.

### You DO need a scanner for

Write a scanner for context-sensitive or unbounded-lookahead lexing — cases
where the identity of the next token depends on arbitrary distance or on
state the LR automaton cannot carry:

- **Indentation blocks** (Python, YAML): a stack of indent widths compared
  on each newline. This needs serialized mutable state.
- **Heredocs** (bash, Perl, Ruby): the parser chooses the closing delimiter
  at open time and matches it arbitrarily later.
- **Nested string interpolation** (Elixir, JavaScript templates): string
  content tokens whose validity depends on interpolation depth.
- **Delimiter-matched raw strings** (Rust `r###"..."###`, C# raw strings):
  count the delimiter at open, then match the same count at close.
- **Newline significance / optional semicolons**: Go's automatic-semicolon
  rule and Pawn's "lax mode" (statements may end at the line break) are the
  canonical cases. The terminator token exists only when the *next* line
  does not continue the statement — pure lookahead-past-the-token territory.
- **Contextual token disambiguation**: Pawn's `public OnFoo(<Bar>)` callback
  signature opener `<` versus relational `i < 10` uses the same character
  for a different token, decided by what follows.
- **Zero-width / sentinel tokens**: these tokens consume nothing but tell
  the parser "a block ended here" (implicit end tags in HTML, dedents, the
  terminators below).
- **Bounded recovery for messy constructs**: consume a known-unparseable
  region as one opaque token so the rest of the file parses. Pawn's
  `#define` header recovery works this way.

### Worked case study: Pawn's five externals

pawnkit's tree-sitter-pawn declares exactly five external tokens
(`grammar.js` `externals`), with a deliberately narrow, **stateless**
`scanner.c` — a good first port, because there is no serialization to get
wrong:

| Token | What it does | Why the lexer can't |
|---|---|---|
| `_callback_signature_start` | Recognizes `<` as the opener of a callback signature. Consumes `<`, marks the token end there, then *looks ahead without consuming*: optional whitespace, then `>`, `_`, `%ident`, or an identifier, then `>`, and — when there was content — requires that the line does not immediately end (EOF, `\n`, `;`, or a comment running to end-of-line all reject). | Distinguishing signature `<` from relational `<` needs lookahead through an identifier, whitespace, comments, and the following construct. |
| `_statement_line_terminator` | Pawn's lax mode: a statement ends at a newline. Skips horizontal whitespace, comments, and `\`-newline line splices first; at EOF, terminates; at `\n`, consumes it, marks the end, then keeps scanning forward — if the next non-trivia character is `{` or a continuation character (`(`, `[`, `.`, `,`, `?`, `:`, operators…), it rejects, because the statement continues. | The token's existence depends on the first meaningful character of the *next* line. Classic unbounded lookahead. |
| `_directive_line_terminator` | Ends a preprocessor directive: skip spaces/tabs/CR; EOF terminates; `\n` is consumed and the end marked after it. | Line-oriented terminator shared by all directives; kept external so it composes with the two recovery tokens. |
| `_unsupported_define_header` | Bounded recovery: when a `#define` header has a shape the grammar doesn't model, consume it byte-by-byte up to whitespace, `/`, or end-of-line, marking the end as it goes, and hand the parser one opaque token. | Recovery: without it, an exotic `#define` shatters into a cascade of ERROR nodes. |
| `_unsupported_macro_parameter_list` | Same idea for macro parameter lists: consume a balanced-paren region (tracking nesting depth, and quoted strings with escapes) until the matching close paren or end-of-line, and only succeed when it actually saw something unsupported. | Balanced-delimiter matching with embedded strings is beyond regular lexing; doing it in the parser would poison the grammar with junk rules. |

Note the shape: two disambiguation/terminator tokens driven by lookahead,
one line-terminator, and two recovery tokens. Zero bytes of persistent
state — the C `create`/`serialize` functions are no-ops. Keep your
externals list this honest.

## Part 2 — porting a scanner to Go

### The interface

From `language.go`:

```go
type ExternalScanner interface {
    Create() any
    Destroy(payload any)
    Serialize(payload any, buf []byte) int
    Deserialize(payload any, buf []byte)
    Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool
}
```

Here is the lifecycle, as the runtime actually drives it
(`parser_dfa_token_source.go`):

- The runtime calls `Create` when it initializes a token source for a
  parse; the returned payload is passed to every other method. Use a
  pointer to a concrete struct; scanners conventionally panic on a wrong
  payload type.
- The runtime calls `Destroy` when it closes or resets the token source.
- `Serialize(payload, buf) int` writes scanner state into `buf` and returns
  the byte count. The runtime hands you a 4096-byte buffer
  (`externalScannerSerializationBufferSize`) — larger than C's 1024 — and
  snapshots state around retries and, when checkpoints are enabled, **after
  external tokens**. Keep `Serialize` allocation-free and cheap.
  `Deserialize` restores from a snapshot, which may be empty; treat an
  empty snapshot as a reset to initial state.
- `Scan` returns true if it recognized a token. On true, the runtime reads
  the result from the lexer (symbol + span). On false, the runtime
  discards position effects and restores the serialized start state by
  default. Implement `FailureStateRetainingExternalScanner` only when the
  failed-scan state change is required by the next scan.

### The dual numbering contract (differs from C!)

Two different number spaces appear in `Scan`, and they are not the same
space — unlike C, where `result_symbol` takes the same enum used to index
`valid_symbols`:

- `validSymbols[i]` is indexed by **external token index**: `i` is the
  position in the grammar's `externals` array, which is also the index
  into `Language.ExternalSymbols` (grammargen preserves this order —
  `registerExternalSymbols` in `grammargen/normalize.go` walks
  `Grammar.Externals` in order).
- `ExternalLexer.SetResultSymbol(sym Symbol)` takes the **language symbol
  ID** — that is, `lang.ExternalSymbols[i]`, not `i`.

Every scanner in this repo therefore keeps two constant sets (see
`grammars/svelte_scanner.go` for the pattern): token indexes for gating,
and symbol IDs for results. Resolve the symbols from the Language at
construction time, rather than hard-coding them.

### The lexer API

From `external_lexer.go`, with C equivalents:

| Go | C | Semantics |
|---|---|---|
| `Lookahead() rune` | `lexer->lookahead` | Current rune; `0` at EOF. |
| `Advance(skip bool)` | `lexer->advance(lexer, skip)` | Consume one rune. `skip=true` moves the token *start* forward (whitespace exclusion) and — exactly like C — does **not** move the token end; `MarkEnd` is the only way to set the end. |
| `MarkEnd()` | `lexer->mark_end(lexer)` | Set token end = current position. |
| `SetResultSymbol(sym Symbol)` | `lexer->result_symbol = ...` | See numbering contract above. |
| `Column() uint32` | `lexer->get_column(lexer)` | 0-based column at the cursor. |
| `HasPreviousBytes(text string) bool` | (no C equivalent) | True if the bytes immediately before the cursor equal `text`; used to guard content tokens when merged parser states expose them too broadly. |
| `AdvanceSpaces(skip bool) int`, `AdvanceUntilNewline(skip bool) int` | (helpers) | Bulk equivalents of repeated `Advance` for ASCII-space runs / to-end-of-line runs. |

You must not reinvent these span rules — they mirror C `ts_lexer` exactly,
and the comments in `external_lexer.go` document why:

- If `Scan` returns true and you never called `MarkEnd`, the end defaults
  to the current cursor, including cursor movement from `Advance(true)`.
- If you `MarkEnd` and then `Advance(true)` past the mark, the token
  becomes **zero-width at the mark**, and the parser re-positions there,
  so it lexes the skipped bytes again on the next call. This is how
  YAML-style and terminator tokens work; it is deliberate, not a bug.

### A faithful Go port of Pawn's scanner (condensed)

```go
package pawn

import gts "github.com/odvcencio/gotreesitter"

// External token indexes: order of grammar.json "externals".
const (
    tokCallbackSignatureStart = iota
    tokStatementLineTerminator
    tokDirectiveLineTerminator
    tokUnsupportedDefineHeader
    tokUnsupportedMacroParameterList
    tokCount
)

// PawnScanner is stateless, like upstream scanner.c.
type PawnScanner struct {
    syms [tokCount]gts.Symbol // external index -> language symbol ID
}

func NewPawnScanner(lang *gts.Language) *PawnScanner {
    s := &PawnScanner{}
    copy(s.syms[:], lang.ExternalSymbols)
    return s
}

func (s *PawnScanner) Create() any                           { return nil }
func (s *PawnScanner) Destroy(any)                           {}
func (s *PawnScanner) Serialize(payload any, buf []byte) int { return 0 }
func (s *PawnScanner) Deserialize(payload any, buf []byte)   {}

// Stateless scanners can safely opt into both fast paths.
func (s *PawnScanner) SupportsIncrementalReuse() bool    { return true }
func (s *PawnScanner) PreservesStateOnScanFailure() bool { return true }

func (s *PawnScanner) Scan(payload any, lx *gts.ExternalLexer, valid []bool) bool {
    if valid[tokCallbackSignatureStart] && scanCallbackSignatureStart(lx) {
        lx.SetResultSymbol(s.syms[tokCallbackSignatureStart])
        return true
    }
    if valid[tokStatementLineTerminator] && scanStatementLineTerminator(lx) {
        lx.SetResultSymbol(s.syms[tokStatementLineTerminator])
        return true
    }
    if valid[tokDirectiveLineTerminator] && scanDirectiveLineTerminator(lx) {
        lx.SetResultSymbol(s.syms[tokDirectiveLineTerminator])
        return true
    }
    // ... the two recovery tokens follow the same shape ...
    return false
}

// scanDirectiveLineTerminator: skip spaces/tabs/CR; EOF terminates; a
// newline is consumed (as skip) and the end marked after it — a zero-width
// terminator, exactly like the C original.
func scanDirectiveLineTerminator(lx *gts.ExternalLexer) bool {
    for lx.Lookahead() == ' ' || lx.Lookahead() == '\t' || lx.Lookahead() == '\r' {
        lx.Advance(true)
    }
    if lx.Lookahead() == 0 { // EOF: see contract (b) below
        lx.MarkEnd()
        return true
    }
    if lx.Lookahead() != '\n' {
        return false
    }
    lx.Advance(true)
    lx.MarkEnd()
    return true
}

// scanCallbackSignatureStart shows the mark-then-look-ahead idiom: the token
// is exactly "<"; everything after the MarkEnd is validation only.
func scanCallbackSignatureStart(lx *gts.ExternalLexer) bool {
    if lx.Lookahead() != '<' {
        return false
    }
    lx.Advance(false)
    lx.MarkEnd()
    // ... whitespace-skip, then accept ">", "_", "%ident", or an identifier,
    // then ">", then reject if the line effectively ends (EOF/'\n'/';'/line
    // comment) — consuming freely, because the mark already froze the span.
    // Port the C control flow 1:1; do not "simplify" the reject conditions.
    ...
    return true
}
```

Gate every branch on `valid[...]`. The runtime computes `validSymbols` from
the grammar tables — for grammargen-built languages, through the
`Language.ExternalLexStates` table (built automatically when the grammar
has externals; it mirrors C's `ts_external_scanner_states`), unioned
across all live GLR stacks when the parse has forked (`SetGLRStates`).
Returning a symbol that is not valid in the current state is
undefined-behavior territory: at best it gets pruned, at worst it triggers
an error cascade.

### Wiring the scanner to the Language

The simplest way, registry-free, is to assign the public field:

```go
lang, err := gts.LoadLanguage(pawnBlob)
...
lang.ExternalScanner = NewPawnScanner(lang)
```

If you distribute through the `grammars` registry
(`grammars.RegisterExternalScanner(name, s)` +
`grammars.RegisterExternalLexStates(name, states)`, then
`grammars.LoadLanguage(name, blob)` / `grammars.AttachLanguageSupport`),
note one caveat verified against `grammars/embedded_loader.go`: for a
language whose name has **no embedded reference blob in this repo**, the
attach path (`AdaptScannerForLanguage`) can bind your scanner only if it
implements

```go
ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner
```

(the `languageBoundExternalScanner` hook). Without it, the adapter tries to
load the in-repo reference blob to remap symbol IDs, and for an
out-of-tree name that lookup panics. Implement the hook (return
`NewPawnScanner(lang)`) or skip the registry and assign the field. If you
need to move a scanner between two Languages with different symbol
numbering, use the public remapper:
`gotreesitter.AdaptExternalScannerByExternalOrder(sourceLang, targetLang)`.

### Optional capability interfaces

- `IncrementalReuseExternalScanner` (`SupportsIncrementalReuse() bool`):
  declare true only if reusing subtrees across edits is safe for your
  state. Stateless scanners: yes. Returning false is an explicit opt-out;
  not implementing the interface is also treated as uncertified and fails
  closed. Python returns true after complete checkpoint restoration; Mojo and
  Starlark still return false.
- `CheckpointedExternalScanner` (`UsesExternalScannerCheckpoints() bool`):
  declare true only when every non-empty `Serialize` result is a complete,
  collision-free encoding of the payload values that can affect a later scan.
  A reachable state may return zero when it cannot be represented exactly;
  zero means "checkpoint absent" and reuse fails closed at that boundary. The
  empty payload itself must have a non-empty encoding so it is distinguishable
  from absence. Reuse compares the live start state and restores the recorded
  end state. SQL, CMake, HTML, and Python use this route; Mojo and Starlark
  record checkpoints but still opt out of reuse. The shared HTML-family tag
  encoding is exact and fails closed when depth, custom-name length, or buffer
  capacity cannot represent the complete state; Svelte still opts out pending
  certification of its additional raw-text and expression-block behavior.
  Python counts indentation with the `uint16` counter used by the locked
  `tree-sitter-python` commit `26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64`.
  Its checkpoint bytes are ephemeral snapshots; compact materialization copies
  them into tree sidecars before the parser-core store is released. Do not
  retain checkpoint IDs or buffer pointers across that boundary.
- `IncrementalPrefixFrontierExternalScanner`
  (`RequiresIncrementalPrefixFrontierProof() bool`): require a fresh parse
  after a changed-length or changed-point edit before the second top-level
  sibling. Use this only when scanner state depends on the old reduction
  frontier. Python enables it for indentation ownership. Other certified
  checkpoint scanners keep their existing reuse contracts.
- `ErrorTreeIncrementalReuseExternalScanner`
  (`SupportsIncrementalReuseFromErrorTree() bool`): an optional narrower gate
  for a scanner whose clean-tree checkpoints are certified but whose recovery
  ownership is not. HTML currently returns false, so changed edits over an
  error-bearing old HTML tree take a fresh-parse fallback.
- `CheckpointlessExternalScannerReuse`
  (`AllowsIncrementalReuseWithoutCheckpoint() bool`): an exceptional second
  proof that a checkpoint-enabled scanner can safely reuse a node whose
  checkpoint is absent. Most stateful scanners must not implement it; absence
  then fails closed. CMake is the current built-in consumer.
- `FailurePreservingExternalScanner` (`PreservesStateOnScanFailure() bool`):
  declare true if `Scan` returning false never mutated the payload — this
  lets the runtime skip defensive state snapshots on the hot path.
- `FailureStateRetainingExternalScanner` (`RetainsStateOnScanFailure() bool`):
  declare true only when a failed scan changes serialized state that the next
  scan must read. The runtime records that actual end state at an internal
  token boundary. Swift uses this route for its carried trivia rune. Retention
  takes precedence if a scanner reports both failure capabilities. Each
  alternate retry starts from the original snapshot. If all retries fail, only
  the terminal failed attempt remains live.

### Incremental-reuse certification matrix

This matrix records the production contract for every external scanner in
the built-in registry. It is derived from the registration files under
`grammars/z_subset_scanner_register_*.go` and the runtime gate in
`languageSupportsIncrementalReuse`:

- **certified reuse** means the registered scanner implements
  `IncrementalReuseExternalScanner` and returns true. A changed edit may
  enter old-tree subtree reuse, subject to the normal dirty-span,
  fragility, byte-equality, and scanner-boundary gates.
- **bounded reuse** records a narrower certified route. Dart is source-bounded
  to 256 KiB; larger sources fall back with
  `dart_large_external_scanner_unsupported`. HTML is clean-old-tree bounded;
  error-bearing old trees fall back with
  `external_scanner_error_tree_unsupported`.
- **fallback (explicit opt-out)** means the scanner implements the
  interface and returns false.
- **fallback (uncertified)** means the scanner does not implement the
  interface. For either fallback status, once the preliminary
  token-invariant leaf fast path declines, `ParseIncremental` ignores the
  old tree and runs the same production full-parse and retry path as
  `Parse`. `ParseIncrementalProfiled` reports `ReuseUnsupported=true`,
  `ReuseUnsupportedReason="external_scanner_unsupported"`, no old-tree
  reuse route, and no reused bytes or subtrees.

The token-invariant leaf exception is intentionally narrow: on a production
tree, a same-length edit that independently reauthenticates the same leaf
token may return before the scanner gate. That is not certification for
general scanner-dependent subtree reuse. Likewise, scanner checkpoints do
not certify a scanner by themselves: CMake, SQL, HTML, and Python opt in,
while Mojo, Starlark, and Svelte explicitly opt out. Custom
`TokenSource` implementations have their own
`IncrementalReuseTokenSource` contract and are outside this registry matrix.
The Markdown scanner is certified for incremental reuse. The Markdown Inline
scanner remains uncertified. Changed-token or shape-changing edits can still
take the production full-parse fallback when dirty-span, fragility,
byte-equality, scanner-boundary, or error-tree gates reject reuse.

PowerShell's scanner is stateless: it recognizes one zero-width statement
terminator from the current lookahead and valid-symbol set, carries no payload,
and has no stateful here-string handling. It therefore uses the same stateless
quiescence proof as Go rather than the checkpoint route. Its focused
certification crosses changed-length edits at multiple positions and requires
incremental trees to match fresh production parses. The representative 137 KiB
near-top insert currently ratchets a conservative 5% reused-byte floor: scanner
admission is solved, but source-wide non-leaf ownership and stale-boundary
rejections remain a separate performance frontier rather than an O(edit)
claim.

The same capability proof now applies to Cue, D, Elixir, and Erlang. Each
scanner has a nil payload, empty serialization, and reads only local lookahead
plus parser-provided valid symbols. Their shared fresh-oracle matrix covers
changed-length insert/delete/replace edits at the start, middle, and end of a
4 KiB clean corpus, plus a 137 KiB macro witness with measured reuse floors.
Because that wave exposed stale top-level reduction ownership in Cue, newly
admitted stateless scanners additionally require an exact recorded pre-goto
frontier before whole-sibling transfer; legacy scanner lanes retain their
existing measured contract until the broader ownership proof is complete.

The next stateless wave admits Gleam, Move, Tcl, and WGSL through the same
matrix and ownership gate. Their 137 KiB witnesses ratchet conservative reuse
floors of 3%, 5%, 32 bytes, and 45%, respectively; these are correctness and
admission floors, not claims that the remaining ownership rejection frontier
is performant.

AWK, KDL, Nix, Squirrel, and Uxntal now exercise the same scanner proof through
the GSS forest route. Forest reuse no longer has a language-name admission
list: clean forest trees must have a quiescence-proven scanner class, and every
top-level transfer must match the exact `PreGotoState` that owned the original
reduction. The focused matrix crosses three changed-length edit classes at the
start, middle, and end of 4 KiB fixtures plus a 137 KiB witness, comparing full
incremental tree serialization with a fresh parse.

Stateful forest admission uses the same capability rule rather than a second
language list: the scanner must opt into both incremental reuse and complete
checkpoints. Forest tokenization requires a non-empty start and end snapshot at
every reachable non-EOF token boundary. If either serialization is absent, the
forest declines with `scanner_checkpoint_unavailable`; automatic routing falls
back to production and the diagnostic forest entry point returns the decline.
The checkpoint store independently rejects one-sided records, so two
unrepresentable states can never authenticate by comparing equal empty byte
slices. A length-changing HTML witness proves useful old-tree reuse and deep
fresh-tree equality, while a synthetic checkpoint scanner that always returns
an absent snapshot proves the generalized route fails closed.

The same proof and matrix also admit EditorConfig, Fennel, Fish, GN, Janet,
Julia, Less, Liquid, Pkl, Racket, TableGen, and Yuck. Their macro witnesses
carry measured floors from 20% to 90% where ownership reuse is broad;
Fish retains a 32-byte floor because its scanner is certified but top-level
ownership rejection still dominates. The low floor is recorded as a parser
performance residual, not inflated into a scanner-safety blocker.

Comment, Dhall, DTD, Foam, Godot Resource, Kconfig, Odin, and RON complete the
next stateless certification wave. Their scanners carry nil payloads, emit
empty checkpoints, and base every scan solely on local lookahead and the
parser-provided valid-symbol set. The shared fresh-tree matrix covers
insert/delete/changed-length replacement at three positions in clean 4 KiB
fixtures plus a representative 137 KiB edit. Macro reuse floors are 3%, 60%,
90%, 10%, 15%, 16 bytes, 10%, and 70%, respectively. Kconfig's low floor is a
parser ownership/performance residual; it is not a scanner-safety exception.

SQL's state proof is explicit: the payload is either empty or exactly one
active PostgreSQL dollar-quote tag. The empty state has a one-byte marker;
representable non-empty states serialize as the tag plus a trailing NUL. A tag
larger than the runtime's 4096-byte checkpoint buffer remains valid SQL and is
accepted during full parsing, but serializes to zero so any affected reuse
boundary fails closed. Failed scans do not mutate the tag. The certification
matrix is exercised at roughly 20 KiB and 137 KiB across clean/recovered
inputs, three changed-length edit classes, and three positions, with an opt-in
1 MiB catastrophe lane. That lane proves fresh equality, accepted EOF, and
source-linear resource ceilings; it does not claim O(edit). The latest 1 MiB
run reached about 1.07 GiB maximum RSS (an earlier run reached about 1.39 GiB),
so allocation/RSS scaling remains an open performance residual rather than
part of #430's sound-admission closure.

HTML's state proof is the complete open-tag stack. The inherited wire format
retains its two-count header, but serialization now preflights the entire state
and returns zero instead of truncating excessive depth, custom names longer
than 255 bytes, or a stack that exceeds the 4096-byte checkpoint buffer.
Deserialization rejects unequal counts, invalid tag types, truncated entries,
and trailing bytes. Failed scans preserve the tag stack. Clean-tree
certification crosses insert/delete/changed-length replace edits at the start,
middle, and end of a roughly 4 KiB stateful document plus a representative
137 KiB edit, requiring actual old-tree reuse and deep equality with a fresh
parse. A recovery witness exposed unresolved error-tree ownership, so that
route explicitly fails closed pending its own certification rather than
silently widening HTML admission.

| Language | Changed-edit contract |
|---|---|
| `agda` | fallback (uncertified) |
| `angular` | fallback (uncertified) |
| `arduino` | fallback (uncertified) |
| `astro` | fallback (uncertified) |
| `awk` | certified reuse |
| `bash` | fallback (uncertified) |
| `beancount` | fallback (uncertified) |
| `bicep` | fallback (uncertified) |
| `bitbake` | fallback (uncertified) |
| `blade` | fallback (uncertified) |
| `c_sharp` | fallback (uncertified) |
| `caddy` | fallback (uncertified) |
| `cairo` | fallback (uncertified) |
| `cmake` | certified reuse |
| `cobol` | fallback (uncertified) |
| `comment` | certified reuse |
| `cooklang` | fallback (uncertified) |
| `cpp` | fallback (uncertified) |
| `crystal` | fallback (uncertified) |
| `css` | certified reuse |
| `cuda` | fallback (uncertified) |
| `cue` | certified reuse |
| `d` | certified reuse |
| `dart` | bounded reuse (up to 256 KiB) |
| `dhall` | certified reuse |
| `disassembly` | fallback (uncertified) |
| `djot` | fallback (uncertified) |
| `dockerfile` | fallback (uncertified) |
| `doxygen` | fallback (uncertified) |
| `dtd` | certified reuse |
| `earthfile` | fallback (uncertified) |
| `editorconfig` | certified reuse |
| `elixir` | certified reuse |
| `elm` | fallback (uncertified) |
| `erlang` | certified reuse |
| `fennel` | certified reuse |
| `firrtl` | fallback (uncertified) |
| `fish` | certified reuse |
| `foam` | certified reuse |
| `fortran` | fallback (uncertified) |
| `fsharp` | fallback (uncertified) |
| `gdscript` | fallback (uncertified) |
| `gitcommit` | fallback (uncertified) |
| `gleam` | certified reuse |
| `gn` | certified reuse |
| `go` | certified reuse |
| `godot_resource` | certified reuse |
| `hack` | fallback (uncertified) |
| `haskell` | fallback (uncertified) |
| `haxe` | fallback (uncertified) |
| `hcl` | fallback (uncertified) |
| `hlsl` | fallback (uncertified) |
| `html` | bounded reuse (clean old trees) |
| `janet` | certified reuse |
| `javascript` | certified reuse |
| `jsdoc` | fallback (uncertified) |
| `jsonnet` | fallback (uncertified) |
| `julia` | certified reuse |
| `just` | fallback (uncertified) |
| `kconfig` | certified reuse |
| `kdl` | certified reuse |
| `kotlin` | fallback (uncertified) |
| `less` | certified reuse |
| `liquid` | certified reuse |
| `lua` | fallback (uncertified) |
| `luau` | fallback (uncertified) |
| `markdown` | certified reuse |
| `markdown_inline` | fallback (uncertified) |
| `matlab` | fallback (uncertified) |
| `mojo` | fallback (explicit opt-out) |
| `move` | certified reuse |
| `nginx` | fallback (uncertified) |
| `nickel` | fallback (uncertified) |
| `nim` | fallback (uncertified) |
| `nix` | certified reuse |
| `norg` | fallback (uncertified) |
| `nushell` | fallback (uncertified) |
| `ocaml` | fallback (uncertified) |
| `odin` | certified reuse |
| `org` | fallback (uncertified) |
| `perl` | fallback (uncertified) |
| `php` | fallback (uncertified) |
| `pkl` | certified reuse |
| `powershell` | certified reuse |
| `properties` | fallback (uncertified) |
| `pug` | fallback (uncertified) |
| `purescript` | fallback (uncertified) |
| `python` | certified reuse |
| `r` | fallback (uncertified) |
| `racket` | certified reuse |
| `rescript` | fallback (uncertified) |
| `ron` | certified reuse |
| `rst` | fallback (uncertified) |
| `ruby` | fallback (uncertified) |
| `rust` | certified reuse |
| `scala` | fallback (uncertified) |
| `scss` | certified reuse |
| `sql` | certified reuse |
| `squirrel` | certified reuse |
| `starlark` | fallback (explicit opt-out) |
| `svelte` | fallback (explicit opt-out) |
| `swift` | fallback (uncertified) |
| `tablegen` | certified reuse |
| `tcl` | certified reuse |
| `teal` | fallback (uncertified) |
| `templ` | fallback (uncertified) |
| `tlaplus` | fallback (uncertified) |
| `toml` | certified reuse |
| `tsx` | certified reuse |
| `typescript` | certified reuse |
| `typst` | fallback (uncertified) |
| `uxntal` | certified reuse |
| `vhdl` | fallback (uncertified) |
| `vue` | fallback (uncertified) |
| `wgsl` | certified reuse |
| `wolfram` | fallback (uncertified) |
| `xml` | fallback (uncertified) |
| `yaml` | fallback (uncertified) |
| `yuck` | certified reuse |

## Hard-learned behavioral contracts

These four contracts came out of this project's C-parity work. They apply
mainly when you implement a **full custom `TokenSource`** (parser_api.go:
`Next() Token`, returning a zero-`Symbol` token at EOF) instead of, or in
addition to, an external scanner — but (a) and (b) also affect scanner
authors.

**(a) Emit extras; never skip silently.** If the grammar declares
whitespace as an extra token, your token source must *emit* it as that
extra symbol, not swallow it. Here is a real incident: a hand-written
token source skipped horizontal whitespace instead of emitting the
grammar's `_whitespace` extra. C shifts whitespace extras, which advances
the parse position, so when error recovery re-lexed, the Go anchor sat one
byte behind C's and the ERROR spans diverged. The fix and its rationale
are preserved as a comment in `grammars/authzed_lexer.go` ("Emit it the
same way instead of silently skipping, so error recovery re-lexes at the
true content byte"). For external scanners, the same rule applies
differently: use `Advance(skip=true)` only for bytes the grammar genuinely
treats as skippable in that context, and remember that skip never moves
the marked end.

**(b) EOF must mirror C: no accept at EOF without a matched token.** At end
of input with nothing matched, return the zero-`Symbol` EOF token at the
EOF position (`lexer.go` does exactly this). Do not promote a partial
match that never reached an accepting state, and do not fabricate a
terminator the grammar didn't ask for. This repo shipped a fix titled
"mirror C tree-sitter behavior for EOF without accept," because getting it
wrong flips end-of-file reductions and changes the last node of every
tree. For external scanners: `Lookahead() == 0` means EOF, and returning
true there is correct only for genuine zero-width EOF-terminator tokens
(Pawn's terminators, dedents), with `MarkEnd` placed deliberately.

**(c) Error-mode lexing.** C's `ts_parser__lex` re-lexes at the recovery
frontier with the most permissive lex mode — `LexModes[0]`, the
ERROR_STATE mode — and the faithful C recovery port expects the same:
after `SetParserState(0)`, tokens should carry error-mode identity. The
built-in DFA token source honors this. The runtime discovers the
capability through the `errorModeLexingTokenSource` interface in
`parser_api.go` (`lexesErrorModeAtErrorState() bool`), but state this
honestly: that method name is unexported, so **an out-of-tree token source
cannot currently declare the capability**, even if it implements the
behavior (Go's unexported interface methods are package-scoped). Here is
what happens instead (`parser_recover_c.go`,
`cRecoverCustomSourceEligibleFor`): if your source supports `SkipToByte`,
the grammar has usable lex tables, **and it has no external
scanner/symbols**, the engine substitutes its own DFA in error mode and
resyncs your source afterward. Otherwise recovery decisions can diverge
from C on error-bearing inputs. Until the marker is exported, implement
`SetParserState(0)` to mean most-permissive lexing anyway (it is the
correct behavior), support `SkipToByte`, and test error inputs against
the C oracle rather than assuming they behave correctly.

**(d) Parser-state plumbing for TokenSource implementers.** The parser
feeds context through optional, structurally matched methods (all
exported names, so out-of-tree types satisfy them):

- `SetParserState(state StateID)` — the runtime calls this before lexing
  each token with the primary live stack's state; state selects the lex
  mode and, for scanners, the `ExternalLexStates` row. State 0 is the
  error state (see (c)).
- `SetGLRStates(states []StateID)` — when multiple GLR stacks are live,
  this carries the full set of stack-top states. Compute external-token
  validity as the **union** across them — this is exactly what the
  built-in source does — then let the parser prune. The set clears (or
  narrows to a single state) when the fork collapses.
- `SkipToByte(offset uint32) Token` / `SkipToByteWithPoint(offset uint32,
  pt Point) Token` — jump to a byte offset and return the first token at
  or after it. Incremental subtree reuse requires this method
  (`IncrementalReuseTokenSource`, `SupportsIncrementalReuse() bool`), and
  recovery resync uses it too. It must be deterministic: skipping to
  offset N and then calling `Next` repeatedly must yield the same stream
  as `Next`-ing from the start past N.
- If the parse uses `Parser.SetIncludedRanges`, an included-range filter
  wraps your source and forwards `SetParserState`/`SetGLRStates`/
  `SkipToByte`/error-mode queries to you (`included_ranges.go`).
  Implement the methods on the base source, and the wrapper composes for
  free.

## Before you ship a scanner port

- [ ] Run your grammar's full corpus through both the C parser and the Go
      port, and compare S-expressions **byte-exact** — not "no errors,"
      exact. tree-sitter-pawn keeps its corpus under `test/corpus/`; treat
      that as the oracle.
- [ ] Test EOF edges specifically: a file ending exactly at your token,
      ending in whitespace, ending mid-construct, an empty file, and a
      file with only a BOM.
- [ ] Test **error inputs**, not just clean ones. Recovery is where (a),
      (b), and (c) show up, and it is the least-tested path in every port.
- [ ] If the scanner holds state: confirm `Serialize` → `Deserialize` →
      `Serialize` reaches a fixed point, and confirm the state fits 4096
      bytes at your worst nesting depth.
- [ ] If any token can be zero-width: confirm the parser makes progress on
      adversarial inputs (the runtime caps consecutive zero-width tokens,
      but hitting the cap means your validity gating is wrong).
- [ ] Gate every `Scan` branch on `validSymbols`; never return a symbol
      whose index was not valid.
