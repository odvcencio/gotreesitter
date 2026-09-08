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

Test: `cgo_harness/compact_t3_oracle_adjudication_test.go` and the T3
witness set. The compact route matches the html and JavaScript witnesses;
production diverges on all 20.

A probe on 2026-09-08 showed the wider state: of 12 malformed JavaScript
sources, 1 built the same tree on both engines; of 9 malformed Python
sources, 3 did. C keeps the subtrees it reduced before the error inside the
ERROR node, and the Go routes absorb terminals or reduce differently. This
board is the next burn-down target.
