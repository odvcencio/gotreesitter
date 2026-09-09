# Cached compact reuse ancestry certificate

Parent: 8b1a54b781a2abfbfcbc356de15f8f81bff31c7a.
The core now computes one validity flag per newly validated graph node.
A valid single-link node requires a valid earlier parent. The active head
must be valid; discarded ambiguous nodes receive false flags without
rejecting a different clean head.

This replaces the prototype's per-borrow reachability map and arena rescan.
The scheduler's existing single-version gate remains in place. Broader
native dispatch is not enabled by this commit.

## Proof and lifecycle

The certificate is a bool slice aligned with the existing node-proof
cursor. Each new node is visited once by that cursor. Payload validation
retains its existing prefix proof. Clean converged/blended flags do not
invalidate a newly computed certificate when error cost, lineage set and
lineage ID are zero. Multiple paths, recovery discontinuities and invalid
active ancestry remain rejected.

Mutations of a previously proven unsafe payload or lineage retain sticky
proof invalidation. Rollback truncates the certificate to the saved node
cursor. Reset clears its length. Decline and oversized-retention release
paths drop its storage.

StorageBytes includes the certificate's used bool elements. FootprintBytes
includes its capacity. This is one byte per element, not a packed bitset.
The test measures the exact used/capacity contribution and verifies that
ResetReleasingRetention releases it.

## Verification

The full parser-core suite passes after the change. Coverage includes
mutation invalidation, cancellation during partial proof construction,
rollback, reset, ownership and existing proof-scaling tests.

New tests establish that an inactive ambiguous seed cannot poison the clean
active head, while selecting that ambiguous seed still fails. A separate
test exercises clean convergence. A rollback/storage test checks that no
unpublished certificate survives and verifies allocation accounting.

The discarded-branch scaling test performs 512 and 1,024 borrows, inserting
a fresh inactive ambiguous node before each borrow. It observes exactly
2 polls per borrow at the existing 128-work-unit polling interval. A rescan
of the growing prefix would trigger more polls. The original linear-chain
and reduction-chain scaling checks also pass.

The first inactive-node fixture accidentally reused the active seed.
That failed run is retained. The corrected fixture creates a distinct seed
and fails on the original implementation. The corrected red test runs in
the retained 346117d6 checkout; its three relevant core source files are
byte-identical to this change's parent. It passes with the certificate.

## Native-dispatch control

With only the saved temporary multi-header dispatch control added, the
cached certificate reproduces four native compact edits with fresh-C
digest parity for token_class_change, early_newline and the prefix witness.
Length changes still decline on the lineage boundary; deletion still reaches
recovery. The compact edit matrix, repeated same-width test and returned-
reuse profile regression pass in the same run.

After removing the temporary dispatch control, the default-route selection
passes again. The core implementation is retained; the dispatch experiment
is saved only as evidence.

These tests establish deterministic proof-work behavior and route parity
for the stated cases. They do not establish a wall-time speedup or a full
retention gate. Native histories still rebuild more nodes than the repaired
legacy path on later edits. The next comparison must measure that tradeoff
before broader dispatch adoption.

## Evidence and next work

harness_out/compact-ancestry-certificate contains corrected and initial red
tests, full-suite results, the native control, final default-route checks,
the exact dispatch patch and Docker receipts. Runs use Go 1.25.14,
GOMAXPROCS=1, CPU 18 and an 8 GiB container limit; no OOM or wall timeout
occurs.

Next: paired fixed-history time/allocation/work measurements with the cached
certificate and broader dispatch; longer native histories and recovery
transitions; old/new overlap and retained capacity. Length-change lineage,
deletion recovery and the other graduation/retirement gates remain open.
