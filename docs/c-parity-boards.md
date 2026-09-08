# C parity boards

This document is the board index for the strict locked-C parity program
(decision 0007). Each board names a test, its scope, and the count on the
date given. Update the counts when a board moves.

## Query semantics

Test: `cgo_harness/parity_query_semantics_test.go`
(`TestParityQuerySemantics`). Each case runs one query on one source
through both engines and compares the captures. A query the C compiler
rejects must be rejected by Go too.

Counts on 2026-09-08: 103 cases, 101 agree, 2 known divergences.

| Case | Owner board | Cause |
| --- | --- | --- |
| go `(_simple_type/type_identifier)` | supertype map | The grammargen map records the aliased subtype as its target symbol (`identifier`), so the subtype check rejects the alias name C accepts. |
| python `(primary_expression)` on `x = (1 +` | recovery | The compact error region absorbs terminals. C keeps the subtree it reduced before the error, so its cursor still reports the hidden `primary_expression` wrapper. |

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

## Highlight parity

Test: `cgo_harness/parity_highlight_test.go` (`TestParityHighlight`,
`GTS_PARITY_MODE=exhaustive`). Capture-level comparison of every bundled
highlight query.

Counts on 2026-09-08: 206 languages, 204 agree, 2 skipped (hurl, mojo have
no C reference build), 0 tolerance entries.

## Supertype map

Test: `cgo_harness/parity_supertype_map_test.go`
(`TestParitySupertypeMap`). Compares `Language.SupertypeSymbols` and
`SupertypeChildren` with `ts_language_supertypes` and
`ts_language_subtypes`. `GTS_PARITY_SUPERTYPE_MAP_STRICT=1` fails the test
on any divergence.

Counts on 2026-09-08: 69 grammars with supertypes, 40 agree, 29 diverge.

Two causes are visible in the report:

- Blob-decoded maps list entries from the C table under shifted symbol
  numbers, so a supertype gains a stray symbol such as `end` and loses its
  last real subtype.
- The grammargen map (go and others) lists production right-hand sides:
  an aliased subtype appears as its target symbol, a hidden right-hand side
  is not expanded to its visible members, and alias-only productions add
  anonymous tokens.

The query compiler validates `super/sub` against this map, so the board
gates that feature. The fix is a map rebuild in grammargen and the blob
decoder; the shipped blobs are pinned by identity, so the rebuild is its
own change.

## Recovery

Test: `cgo_harness/parity_recovery_board_test.go`
(`TestParityRecoveryBoard`). Each case parses one malformed source on the C
oracle and on the Go default, compact, and production routes, then compares
the trees node by node. `GTS_PARITY_RECOVERY_STRICT=1` fails the test on any
divergence of the default route. The compact route delegates recovery to the
production port on every case, so the three route columns agree.

Counts on 2026-09-08: 78 cases, 35 agree on the default route (29 before
this round). With `GOT_C_RECOVERY=all`, 45 agree; the difference is
JavaScript, which stays on the legacy path (see below).

| Language | Cases | Agree |
| --- | --- | --- |
| c | 8 | 5 |
| go | 10 | 6 |
| java | 8 | 5 |
| javascript | 16 | 1 (11 with the C recovery port) |
| json | 6 | 5 |
| python | 14 | 5 |
| rust | 10 | 5 |
| typescript | 6 | 3 |

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
- html joins the languages that run the C recovery port by default.

Languages that stay on the legacy recovery path, each behind a measured
witness:

| Language | Witness | Cause |
| --- | --- | --- |
| cpp | `TestCppMalformedClassFunctionDefinitionRecovery` | The port inserts a MISSING `::` where C skips a token; C's skip costs 601, the missing leaf 610. |
| javascript | `TestW5JavaScriptFamilyTransientErrorGate` | The port exceeds the incremental replace ceiling at the start of a 20 KiB file by about 2.8 times (81244 new nodes against 29400); the forks created after the error do not merge again. |
| julia | `TestJuliaTrailingCommaAssignmentTupleCompatibility` | The scanner emits a zero-width identifier that hides the error C reports. |

Divergence classes that remain, in burn-down order:

1. The result root builder (`parser_result_root_build.go`) rebuilds the
   root with its own extra-folding rules. C's accept keeps the topmost
   non-extra subtree as the root and splices every other stack subtree in
   order, so a strategy-1 ERROR node keeps its `extra` bit and its wrapper.
   Witnesses: json `[1, 2,` (extra bit lost), python `return\n)\n` (the
   ERROR wrapper around `)` dissolves).
2. Version order and tie-breaks. C keeps tied erroneous versions apart until
   accept and then prefers the later one (`ts_parser__select_tree`), and a
   merged head resolves the tie at the next reduce the same way. The port
   picks the earlier fork. Witnesses: javascript `foo(1 2);`, python
   `print(1, 2`.
3. Version merging after recovery. C merges forks that reach the same state
   and position, which bounds the work after an error; the port keeps them
   apart, which is the JavaScript W5 cost.
4. Strategy selection with missing tokens (cpp witness above) and the
   remaining shape divergences in go, rust, and typescript.
5. Silent recovery. Some malformed sources parse on the Go routes with no
   ERROR node and no error bit (python `[...] ifsystem() != "Windows"`
   builds a `call` with a stray identifier child). C reports an ERROR.
   The incremental invariant gate records two such sites in
   `testdata/incremental_allowlist.json`.
