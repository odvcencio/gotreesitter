# C parity boards

This document is the board index for the strict locked-C parity program.
Each board names a test, its scope, and its latest count. Update the counts
when a board moves. Counts use the C oracle 0.27.1 unless the text names
another version.

## Query semantics

Test: `cgo_harness/parity_query_semantics_test.go`
(`TestParityQuerySemantics`). Each case runs one query on one source
through both engines and compares the captures. A query the C compiler
rejects must be rejected by Go too.

Count: 103 cases, 99 agree, 4 known divergences. Three of the four break the
rule above: C rejects the query and Go compiles it. For a grammar with an ABI 15
subtype map, the 0.27.1 C query compiler requires each direct child of a
supertype pattern to be one of the supertype's listed subtypes, and reports
`Impossible pattern` otherwise. In javascript and python (both ABI 15),
`identifier` is not a listed subtype of `expression`. The check does not run
for ABI 14 grammars, so luau `(expression (identifier) @i)` still compiles
and agrees.
The 0.25.1 compiler accepted all three.
[Issue 1545](https://github.com/odvcencio/gotreesitter/issues/1545) tracks
the Go repairs for the 0.27.1 differences.

| Case | Owner board | Cause |
| --- | --- | --- |
| python `(primary_expression)` on `x = (1 +` | recovery | The compact error region absorbs terminals. C keeps the subtree it reduced before the error, so its cursor still reports the hidden `primary_expression` wrapper. |
| javascript `(expression (identifier) @i)` | C oracle 0.27.1 (issue 1545) | `identifier` is not a listed subtype of `expression`, so C 0.27.1 reports `Impossible pattern`; Go compiles it. |
| python `(expression (identifier) @i)` | C oracle 0.27.1 (issue 1545) | `identifier` is not a listed subtype of `expression`, so C 0.27.1 reports `Impossible pattern`; Go compiles it. |
| javascript `(expression . (identifier) @i)` | C oracle 0.27.1 (issue 1545) | `identifier` is not a listed subtype of `expression`, so C 0.27.1 reports `Impossible pattern`; Go compiles it. |

Semantics the Go engine now shares with the C query cursor:

- A node type is a visible or supertype named symbol. Hidden rule names and
  anonymous tokens are not node types.
- A supertype pattern is a wildcard step with a supertype requirement. The
  node must carry the supertype among the hidden wrappers reduction elided.
  `super/sub` keeps the subtype symbol and adds the requirement; the C
  subtype check applies when the grammar carries an ABI 15 map.
- A wildcard root with a concrete first child is never tested. The root
  only has to exist and not be an ERROR node.
- A wildcard never matches an ERROR node.
- An anchor before a child pattern ignores anonymous siblings, except after
  an unnamed wildcard.

Generated exact queries: `TestParityQueryExactTop50Generated`
(`cgo_harness/parity_query_top50_test.go`; it needs `GTS_PARITY_MODE=top50`
or `exhaustive`) builds queries from each top-50 language's smoke sample and
compares the captures. Count: 416 generated queries across 50 languages. Under
0.27.1, 408 agree and 8 differ: the
`adjacent_sibling_run_plus` and `adjacent_sibling_run_star` cases for sql,
haskell, objc, and json5. Those 8 are on the 0.27.1 known-differences list
(`cgo_harness/c_oracle_known_differences_test.go`, issue 1545). C 0.27.1
returns one match per run of the repeated child; Go returns only the first
run. Example: on the json5 sample `{ "key": "value" }`, the query
`(member (string)* @run)` captures `"key"` and `"value"` in C and only
`"key"` in Go.

## Highlight parity

Test: `cgo_harness/parity_highlight_test.go` (`TestParityHighlight`,
`GTS_PARITY_MODE=exhaustive`). Capture-level comparison of every bundled
highlight query.

Count: 206 languages, 199 agree, 7 skipped, 0 tolerance entries. The
test skips a language when the C compiler rejects its bundled highlight
query. C reports each error position as line:column in that query.

| Language | C error | Rejected by C |
| --- | --- | --- |
| hurl | `Invalid field name key` at 72:12 | 0.25.1 and 0.27.1 |
| kotlin | `Invalid node type $` at 373:3 | 0.25.1 and 0.27.1 |
| mojo | `Invalid node type break` at 99:4 | 0.25.1 and 0.27.1 |
| scala | `Invalid node type end` at 164:4 | 0.25.1 and 0.27.1 |
| swift | `Invalid node type nil` at 117:2 | 0.25.1 and 0.27.1 |
| fsharp | `Impossible pattern` at 352:3: `(long_identifier (identifier) @type.builtin)` under `(_type ...)` | 0.27.1 only |
| solidity | `Invalid syntax` at 90:58: the trailing anchor in `(struct_expression type: ((expression(identifier)) @type .))` | 0.27.1 only |

Go still compiles the fsharp and solidity queries. Under the 0.25.1 oracle
only the first five rows are skipped; the other two skips come from the oracle
change.

## Supertype map

Test: `cgo_harness/parity_supertype_map_test.go`
(`TestParitySupertypeMap`). Compares `Language.SupertypeSymbols` and
`SupertypeChildren` with `ts_language_supertypes` and
`ts_language_subtypes`. `GTS_PARITY_SUPERTYPE_MAP_STRICT=1` fails the test
on any divergence.

Count: 69 grammars with supertypes, 36 agree, 33 diverge, 0 without a C
reference. The count is the same under 0.25.1 and 0.27.1.

Two causes are visible in the report:

- Blob-decoded maps list entries from the C table under shifted symbol
  numbers, so a supertype gains a stray symbol such as `end` and loses its
  last real subtype.
- The grammargen map (go and others) lists production right-hand sides:
  an aliased subtype appears as its target symbol, a hidden right-hand side
  is not expanded to its visible members, and alias-only productions add
  anonymous tokens.

The query compiler validates `super/sub` against this map, so the board
gates that feature. The shipped blobs are pinned by identity, so a rebuilt
map in grammargen and the blob decoder is a separate change.

## Recovery

Test: `cgo_harness/parity_recovery_board_test.go`
(`TestParityRecoveryBoard`). Each case parses one malformed source on the C
oracle and on the Go default, compact, and production routes, then compares
the trees node by node. `GTS_PARITY_RECOVERY_STRICT=1` fails the test on any
divergence of the default route. The compact route delegates recovery to the
production port on every case, so the three route columns agree.

Count: 79 cases, 44 agree on the default route. With `GOT_C_RECOVERY=all`,
52 agree; the difference from the default route is JavaScript, which agrees
on 12 of 17 cases. The compact and production route columns of the board
report the same counts. The table shows the default route.

| Language | Cases | Agree |
| --- | --- | --- |
| c | 8 | 8 |
| go | 10 | 6 |
| java | 8 | 5 |
| javascript | 17 | 4 (12 with the C recovery port) |
| json | 6 | 6 |
| python | 14 | 4 |
| rust | 10 | 6 |
| typescript | 6 | 5 |

Rules the port now shares with C:

- An absorbed leaf carries no error bit. C gives a leaf an error cost only
  when it is missing (`ts_subtree_error_cost`), and that holds for an
  unlexable-byte ERROR leaf too. Only the ERROR container is erroneous.
- A keyword stays a keyword when the parse state has an action for it or
  reserves it; otherwise the lexer returns the word token, whether or not
  the word token has an action (`ts_parser__lex`). The parser then reports
  the error on the word token, as C does.
- A missing leaf is a relevant child and takes an inherited field
  (`ts_node_field_name_for_child` skips extras only).
- An accepted version stays out of the stack merge. C removes an accepted
  version from the pool, so a strategy-1 fork that reaches the same state
  and position at end of input still acts on the end symbol and competes
  as its own tree (json `[1, 2,` keeps the `extra` bit on its ERROR).
- An ERROR node keeps the field a hidden child gave its spliced children
  (`ts_node_field_name_for_child` descends through hidden nodes), so c
  `int f( { return 1; }` reports `type:` on the primitive type inside the
  ERROR.

Languages that stay on the legacy recovery path, each behind a measured
witness:

| Language | Witness | Cause |
| --- | --- | --- |
| cpp | `TestCppMalformedClassFunctionDefinitionRecovery` | The port inserts a MISSING `::` where C skips a token; C's skip costs 601, the missing leaf 610. |
| javascript | `TestW5JavaScriptFamilyTransientErrorGate` | The port exceeds the incremental replace ceiling at the start of a 20 KiB file by about 2.8 times (81244 new nodes against 29400); the forks created after the error do not merge again. |
| julia | `TestJuliaTrailingCommaAssignmentTupleCompatibility` | The scanner emits a zero-width identifier that hides the error C reports. |
| html | `TestExternalLexStatesRecoveryElectionOptOutInventory` | The external lex election ledger keeps html opted out; no board case measures html yet. |

Divergence classes that remain:

1. The python root repair (`repairPythonRootNode`,
   `collapsePythonRootFragments`) dissolves an extra ERROR wrapper. The
   result nodes that reach the root builder for `return\n)\n` are
   `module(return_statement, ERROR[extra](")"))`, which is the C tree; the
   published tree has a bare `)`.
2. The stack summary after `do_all_potential_reductions` differs. For rust
   `fn f() { foo(1 2); }` C records state 1442 at depth 1 (the state after
   `foo(1` once `1` is reduced with any lookahead) and elects it for `)`,
   which wraps only `2`. The port's summary holds state 139 at depth 2 or
   state 1507 at depth 1, so it wraps `1` or picks another state. The same
   class decides c, java, and javascript `foo(1 2)`. The C parser logger
   (`Parser.SetLogger` in the Go binding) prints C's choices
   (`recover_to_previous state:1442, depth:2`); use it beside the Go GLR
   trace.
3. Version order and tie-breaks. C keeps tied erroneous versions apart until
   accept and then prefers the later one (`ts_parser__select_tree`), and a
   merged head resolves the tie at the next reduce the same way. Witness:
   python `print(1, 2` (C keeps the `print` keyword path, the port keeps
   the identifier path).
4. Nested recovery at end of input. For typescript `type T = { a: ;` C
   recovers the skip version to state 1 at depth 8 and builds one flat
   ERROR (cost 1215, extra). The port elects from a resumed version at
   depth 6 and nests the earlier `a :` ERROR inside (cost 1716), so the
   flat end-of-input wrap without the extra bit wins.
5. Version merging after recovery. C merges forks that reach the same state
   and position, which bounds the work after an error; the port keeps them
   apart, which is the JavaScript incremental-edit cost.
6. Strategy selection with missing tokens (cpp witness above) and the
   remaining shape divergences in go, rust, and typescript.
7. Silent recovery. Some malformed sources parse on the Go routes with no
   ERROR node and no error bit (python `[...] ifsystem() != "Windows"`
   builds a `call` with a stray identifier child). C reports an ERROR.
   The incremental invariant gate records two such sites in
   `testdata/incremental_allowlist.json`.
