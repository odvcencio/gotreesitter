# Compact reuse after clean convergence

Baseline: 83b3d739f7614c72fa23f99d2d5beb0369177f9b.
Status: diagnostic prototype, not an adopted runtime change.
The Go merge and reuse-profile repairs remain in place.

## Result

A prototype completes native compact reuse for four alternating edits in
three histories. Every returned tree matches a fresh C digest.

| History | Native compact edits | Reused bytes per edit | New nodes per edit |
| --- | ---: | ---: | ---: |
| token_class_change | 4/4 | 17,625 | 1,475 |
| early_newline | 4/4 | 212,926 | 7,494 |
| newline_prefix_call_conversion | 4/4 | 53,861 | 3,054 |

The length-change and deletion histories retain legacy fallback. The
prototype does not certify all representative edits.

These are deterministic route/work observations, not performance results.
The prototype builds a reachability map and scans graph records on each
borrow. It is not suitable for adoption without a bounded, cached proof.
Later native edits also build more nodes than the repaired legacy route:
for example, newline uses 7,494 versus 1,162 reported new nodes. A native
route alone is not sufficient evidence for efficiency graduation.

## Isolation sequence

1. Allow ordinary clean multi-header dispatch while deferring reuse until
   one header remains. Keep scanner ownership and recovery checks.
   Four clean histories then decline on fork-order metadata in ancestry.
   The deletion history reaches a no-action recovery boundary.
2. Permit ordered links while retaining path-count, lineage, payload,
   scanner and adjacency checks. The four clean histories decline on a
   two-path graph node.
3. Trace reachability from the active head. In all four clean histories,
   that rejected two-path node is unreachable from the current head:

| History | Rejected node | Active head | Rejected paths/links | Reachable |
| --- | ---: | ---: | ---: | --- |
| token_class_change | 69 | 1,799 | 2/2 | no |
| same_line_length_change | 551 | 646 | 2/2 | no |
| early_newline | 116 | 374 | 2/2 | no |
| newline_prefix_call_conversion | 116 | 374 | 2/2 | no |

4. Recompute active ancestry on every borrow. Reset the graph proof cursor
   on each call so excluding nodes cannot certify them for a later head.
   Keep the global payload proof. The clean histories next decline on the
   active head itself: one path, one link, converged=true, blended=true.
5. Permit those two clean-convergence flags, but continue to reject stored
   error cost, nonempty lineage sets, nonzero lineage IDs and actual
   multiple paths. This enables the three native histories above.

The length-change head still has a lineage set with 28 entries, even though
stored error cost is zero and its scalar lineage ID is zero. The prototype
keeps that boundary closed. Deletion still reaches recovery. Neither
condition is reclassified as safe by this experiment.

The opaque-subtree C-comparison guards remain unchanged. No comparison
guesses the raw child count of an opaque borrowed payload.

## Verification

All successful diagnostic runs execute the permanent repeated-history
C-oracle test with four edits per case. The final clean-convergence run
passes all five histories, with the native/fallback split reported above.
The final length-only trace also passes.

Docker uses the pinned one-CPU lane, Go 1.25.14, GOMAXPROCS=1 and an 8 GiB
limit. There are no OOM kills or wall timeouts. One reachability trace
initially fails to compile because of a LinkID conversion; that output is
retained separately. It supplies no runtime evidence.

## Required implementation work

Replace the per-borrow map and arena scan with a bounded active-ancestry
certificate. Validate proof invalidation, transaction rollback, reset,
ownership and cancellation. Account for any added certificate storage.
Preserve rejection of active multiple paths and recovery lineage, and add
negative tests for those boundaries.

Then require native-route receipts and C parity across longer histories,
including ambiguous prefixes and clean/recovery transitions. Compare fixed
histories for time, allocation, work, retained heap and old/new overlap.
Resolve the length-change lineage set and deletion recovery separately.

## Evidence and cleanup

harness_out/compact-borrowed-ambiguity contains each exact patch and JSON
test output, including the failed build, plus Docker receipts. The patches
are experiments in sequence; do not apply them together cumulatively.

All temporary runtime changes are restored after capture. No new default
or convergence certification is adopted. Graduation and retirement remain
open.
