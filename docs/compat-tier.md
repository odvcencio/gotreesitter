# The C-faithful result-compatibility tier

The root package contains a visible `parser_result_<language>*.go` tier.
It is post-parse compatibility scaffolding: where the parser does not yet
reproduce a C tree-selection or materialization behavior through a general
engine mechanism, a bounded pass reconciles the returned tree with the C
oracle.

This page is a readable guide. The mechanically checked source of truth is
[`testdata/result_compat_ownership_v1.json`](../testdata/result_compat_ownership_v1.json).
Do not update dispatcher coverage, ownership, or retirement state here
without updating that registry.

Each registry entry has a stable ID, functions and files, languages, purpose,
authoritative owner, witnesses, a retirement condition, coverage fields for
production/compact/forest/incremental/C-oracle routes, status, and optional
receipt references. A retired entry remains in the registry with its commit
and receipt references; deleting the historical record is not retirement.

## Ownership

Compatibility functions describe symptoms. Their `authoritative_owner`
records the earliest subsystem that must eventually produce the C-faithful
result:

| Owner | Responsibility |
|---|---|
| `scheduler_action_semantics` | action ordering, recovery, and work execution |
| `derivation_election_selection` | ambiguity and winning-derivation choice |
| `materialization` | node, field, alias, trivia, and span construction |
| `scanner_checkpoint_state` | external-scanner state and restoration |
| `incremental_edit_reuse` | edit invalidation and subtree reuse |
| `public_compatibility` | compatibility intentionally retained at an exported API boundary |

The route fields use a closed vocabulary enforced by the registry test:

- live dispatcher, predicate, and generic entries use
  `shared_result_compatibility_tail` for production/compact/forest/incremental
  and `curated_single_grammar_parity` for the C oracle;
- retired production and incremental routes use `retired_exact_receipt`;
- retired compact and forest routes use `retired_exact_receipt` or
  `retired_exact_or_fail_closed_receipt`;
- the C oracle uses `retired_exact_receipt` or
  `retired_known_divergence_receipt`.

Each retired entry must include a retirement commit and a receipt reference.

These are evidence labels, not claims that all five engines have independent
implementations. The shared-tail value means that route reaches the same
post-parse normalization tail.

## Why it exists

Two GLR parsers can accept the same input and still return different trees
under ambiguity, error recovery, aliasing, or extra/trivia attachment.
gotreesitter's parity target is byte-exact agreement with the selected C tree,
including error shapes and recovered spans. The cgo-backed suites under
`cgo_harness/` provide that oracle.

The tier stays internal because it operates on arena and node internals before
the tree is returned. Moving it into a package with exported plumbing would
make the exported API surface worse without changing its ownership.

## Editing and validation

When adding, moving, or retiring compatibility code:

1. Update the JSON registry in the same change.
2. Keep dispatcher languages, called functions, ownership, witnesses, routes,
   and retirement evidence explicit.
3. For retirement, keep the entry and set `status` to `retired`, with
   `retired_commit` and at least one `receipt_refs` item.
4. Give every live arm a current firing witness, or file a defect for the gap.
   A witness shows that the arm returns `checked >= 1`, `run >= 1`, and
   `nodes_visited > 0` through the production parse path with
   `GTS_DISPATCHER_CENSUS=1`. The census does not change parser output when it
   is disabled.
5. Cover each arm with a small real-source corpus before you use rewrite
   frequency for retirement or attribution.
6. Run the focused registry gate:

```sh
go test . -run '^TestResultCompatibilityOwnershipRegistry$' -count=1
```

The test parses `parser_result_compat.go`, `parser_result_helpers.go`, and
`parser_api.go` with the Go AST. It fails when a dispatcher arm or predicate
is unregistered, the exact COBOL predicate language set drifts, a generic pass
changes without a registry update, a post-finalization language arm or call
changes, or a live registered function disappears from its declared files.
The COBOL check requires the canonical nil-guard AND parenthesized two-equality
OR AST. The fixpoint check counts clause-local call occurrences and rejects
normalization in default clauses or outside registered cases. The gate also
enforces kind/status combinations, route vocabulary and semantics, required
metadata, and referenced witness paths.

## Rules for compatibility code

- A rewrite may widen a producer-owned span but may not shrink it. The rule is language-neutral.
- A display-name or pair-level metadata match alone does not admit a caller-built or custom artifact to native child shaping. Such an artifact opts in only with the explicit native capability and the exact metadata receipt.
- The compact scheduler admits a zero-width extra only when an external scanner produces it and the scanner checkpoint or parser state advances.
- Reopen a retired entry if a future witness for its language needs a rewrite to match locked C, or differs on any production, compact, forest, incremental, or locked C route. For Ninja and Ledger, also reopen if a future witness rewrites a node.
- Retire the Perl arm only after the engine emits the grouped shape for `push LIST => ...` without it, and after the Perl parity and route receipts include the fat-comma witnesses.

This document is hand-maintained explanatory text, not generated output. The
JSON registry and the focused Go test are the regeneration/validation
contract.
