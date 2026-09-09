# Forest hidden-node pruning investigation

The forest route still fails five of the 25 locked C Go conflict fixtures.
Production and compact pass all 25. This investigation does not graduate
the forest or change its runtime behavior.

## Observed loss of an alternative

For `package p;var _=F[int](1,2)\n`, C selects an indexed function with
two call children. Forest selects a generic call with three children.

Instrumentation at the hidden-node merge records two `_expression`
alternatives at bytes 16–27 with the same predecessor, symbol, score,
and error cost. Their visible call subtrees differ. The merge keeps the
first. Thus equal predecessor, symbol, and span do not prove equal
visible descendants. The old source comment claimed this equality.

This identifies a loss point. It does not prove that removing pruning
alone reproduces C scheduling or election.

## Rejected repairs

All runs use the same 25 Go fixtures, C oracle, and three routes.
Each trial keeps production and compact at 25 passes.

| Forest policy | Pass | Fail |
| --- | ---: | ---: |
| Existing policy | 20 | 5 |
| C raw comparator at hidden score/error ties | 13 | 12 |
| Preserve raw-distinct hidden links; use C comparator in link ranking | 13 | 12 |
| Preserve raw-distinct hidden links; retain existing ranking | 13 | 12 |

Each trial fixes four original two-argument failures but introduces
one-argument call versus type-conversion failures. The parenthesized
fixture still fails. All three runtime patches were saved and reverted.
They did not pass correctness, so no performance campaign was run.

## C behavior and next boundary

The pinned oracle is go-tree-sitter v0.25.0. In its `src/stack.c`,
`stack__subtree_is_equivalent` checks symbol, padding, size, child count,
extra status, and external scanner state. For equivalent subtrees with
the same predecessor, `stack_node_add_link` replaces only for higher
dynamic precedence. Equal precedence keeps the existing link.

The final tree comparator in `src/subtree.c` orders symbol IDs, child
counts, and child subtrees. That final comparator is not a substitute
for intermediate merge policy.

The next repair must account for C reduction arrival order and merge
equivalence. It must retain the existing work and link caps and pass
both generic conflicts and repeat workloads. Preserving all raw shapes
can consume scarce links on repeat regroupings. A visible-child envelope
alone is also insufficient: two call nodes can have equal spans and
different descendants.

## Evidence

Base: `46b22d2a30b8bbf094f727dc476d189f226cad28`.
Remote evidence: `harness_out/forest-generic-election/`.

- `trace.log` and `trace.patch`: observed alternatives.
- `premature-c-order.patch` and `premature-c-order-parity.jsonl`.
- `preserve-raw-with-ranking.patch` and `preserve-raw-parity.jsonl`.
- `preserve-only.patch` and `preserve-only-parity.jsonl`.

Command inside the isolated Go container:

```sh
cd /workspace/cgo_harness
CGO_ENABLED=1 go test \
  -tags gts_parsercorephase0,treesitter_c_parity . \
  -run '^TestGoCleanConflictOrderLockedC$' -count=1 -json
```

No caps, memory limits, route contracts, or runtime statements changed
in the retained source edit. The edit removes the false equivalence
claim and links this evidence.
