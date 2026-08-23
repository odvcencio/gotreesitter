# G18 drop-cohort certificate design

Revision v20 binds wrap, cap, allocation, and per-election receipt checks to real `Core` and `Parser` instances.
It also freezes the one-shot missing-certificate suppression seam and authenticated Slice B compatibility wrappers.

## Decision scope

G18 covers the converged-split no-action family. This family owns eight active profile flags.

The family ranking is:

| Rank | Producer family | Flags | Grammars |
| ---: | --- | ---: | --- |
| 1 | Converged-split no-action | 8 | Go, Erlang, Haskell, JavaScript, Bash, Perl, Ada, Python |
| 2 | Primary acceptance | 6 | Kotlin, Apex, Meson, Perl, Ada, Python |
| 3 | Strategy-2 error region | 1 | HTML |

The denominator stays at 15 flags across 12 grammars. G18 does not change a runtime profile.

## Private certificate admission activation

Keep certificate admission disabled by default. Keep every runtime profile flag
unchanged. Add one private test-only activation seam on the root parser:

```text
DiagnosticEnableDropCohortCertificateAdmissionForTest() func()
```

The seam returns a restore function. Activate it only on the candidate parser
for one test parse. Run the restore function before the parser enters a cached
second parse. The same `Parser` type may implement the seam in every build.
The disabled state, rather than interface absence, selects the existing fallback.
Snapshot telemetry on the candidate parser before its disabled parse.

Do not consume a certificate when activation is disabled. Do not consume a
certificate when the certificate is missing or invalid. Each decline requires
an unchanged route counter and fallback plus one; these outcomes are exclusive.
The restored cached parse uses the same exact counter rule.
A production parser must expose a literal primitive-only session method. It
returns run, snapshot, close, and error values. The closures share one Core.
The provider must return non-nil run, snapshot, and close closures. The test
guards close with one local once gate and may call it repeatedly. The snapshot
returns owner, generation, and canonical receipt bytes. The RED
fails at the missing production method. Test-local named interfaces, constant
snapshots, and standalone Core constructors are forbidden.
Do not substitute ordinary scheduler elections or parser result fields.
Snapshot this API before and after bounded execution, and require exact equality.
The runner must ignore the
activation state. A non-candidate parser and production route must also ignore
it. The Kotlin future control enables the seam before parsing and
restores it with idempotent cleanup.

The missing-certificate fallback test may call one test-only seam once:

```text
DiagnosticSuppressDropCohortCertificateForTest()
```

This call suppresses only the positive certificate for that test. Keep the
call out of the positive, invalid-certificate, non-candidate, and diagnostic
tests. Keep corruption tests separate from the missing-certificate case.

Slice C must provide three distinct exported, test-only compatibility methods:

```text
G18AdoptUpdatedReductionSiblingOwned(
    SchedulerTransactionToken, source, head, rank, lineage, set,
    set_blended, converged, resurrection, DropCohortProducerMutationClass,
) (adopted, error)

G18ReconcileGenericConflictOutputsOwned(
    SchedulerTransactionToken, source, outputs,
    DropCohortProducerMutationClass,
) (kept, adopted, error)

G18CanonicalizeOwned(
    SchedulerTransactionToken, DropCohortProducerMutationClass,
) error
```

Each method must validate and forward the active `SchedulerTransactionToken`
and the intended producer mutation class. The packet uses direct compile-time
interfaces with these exact signatures. It uses no reflection and no arity
fallback. Conflict reconciliation passes its class through the sibling-adoption
path and records one class only. A missing method is a runtime RED seam, not a
compile-time substitute and not a token-validation bypass.
The linear and mapped canonicalizer tests use the same owned adapter and pass
their distinct mutation classes. They never call canonicalization scratch
directly for authentic telemetry.

The current fallback characterization runs with activation disabled and
requires the existing fallback and unchanged verifier telemetry. The future
direct-route contract differs only by enabling this private seam. It then requires the direct route and complete
producer and verifier telemetry. Neither test changes a grammar flag, profile,
grammar allowlist, or grammar blob.

The design applies to parser data only. It does not use grammar names, source text, or blob hashes.

## Locked denominator and scorecard

The locked denominator is 15 flags across 12 grammars.

The eight G18 route groups cover 13 locked-C target cases. The remaining seven flags stay in the existing primary and strategy families.

Use this scorecard tuple:

`(PASS, FALLBACK, SKIP, FAIL, DEFECT, TOTAL) = (198, 3, 5, 0, 0, 206)`

The total is `198 + 3 + 5 + 0 = 206`. Keep the denominator and the scorecard unchanged at the RED boundary.

## Current behavior

`RecordReductionLineageOwned` records an event and a branch. The scheduler then carries that set through these paths:

- The linear canonicalizer.
- The mapped canonicalizer.
- Sibling adoption.
- Conflict reconciliation.
- A dead-node history import.

`dropGenericNoActionHeads` makes the final drop decision. The current proof requires exact event and branch containment.

The current proof has two limits:

- An inline set has no arena identity.
- A branch does not identify its action and derivation.

The profile grant bypasses those limits. It does not prove parser ownership.

## Control manifest

The contract pins each source with Secure Hash Algorithm 256 (SHA-256). Source hashes authenticate tests only.

The ordered 13-case manifest digest is:

`3a659138e3aac82563e5fdf46aeb3041a685d6311f32dfd6660c0337b02dbb0d`

| Grammar | Control | Source SHA-256 |
| --- | --- | --- |
| Go | `query_compile` | `b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894` |
| Go | `rewrite` | `74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097` |
| Go | `language` | `009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a` |
| Go | `grammargen_lr` | `a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2` |
| Erlang | Macro function clauses | `796647c2730c3f3d9d88abb589d606df700c2806396a8b79769fe89f65a5bea0` |
| Erlang | Macro top-level function | `36f66cda874299c4f9de7aa86bd5653c6d18cfd218d68d249c806acb39eb046e` |
| Haskell | Smoke source | `8e46b697006a890d6629efa91e5be5ba778ecf5c1315b3dd3b2265f2549bc854` |
| JavaScript | Functions | `0bbd2cdb0a0492055e442c44b533797386ec9c8aeb7ce8a4d0f5f5a4681e3b90` |
| Bash | Converged split | `cccefdfff900acc9873a16805c244725b08d0bf85e15c7dacc3886ba8d5c7b4c` |
| Perl | Two-argument push | `08ac06c62278aa8bb26361629ac930bbbfbe5031da04a54ee3aeec4875ce0b3b` |
| Perl | Three-argument push | `7be8389f1e6981c2e1e6324357df96ffb34063e9ea6811b8c332143e76015cd1` |
| Ada | Positional array | `cce847719840bac903a5e52bd6d7b31d9f67a28353706ae3fc82ac07d0511e9b` |
| Python | Greeting function | `d29a356c8115cbf7b87f6644c6ff6f8b1fa530f7dcbd0fbc200f2ac9400827dd` |

The opposite controls are two Kotlin annotated getters:

- `7aad51909730adbec84060f5445384ff82c9517e3603fb0da86c9ac2397548b7`
- `3c745d430894c3c739f217b807f747a4bd2f521173b441e625e4ca109b773ec2`

A forced split grant makes both Kotlin trees differ from locked C. The future verifier must decline both controls.

## Current fallback characterization

The current characterization test removes one grant from an exported-field clone. It keeps certificate admission disabled and requires the existing fallback.

The test also requires these results:

- The fallback tree equals the production tree.
- The fallback tree equals locked C.
- The fallback reason matches the current proof failure.
- The current census matches the pinned values.

This characterization is not a future admission contract. It must stay separate from the RED test.

The future test uses the same clone and source, then enables only the private
certificate-admission seam. It requires the direct route and verifier proof.
The test restores the seam before a cached second parse and requires fallback.
It also tests missing or invalid certificates, non-candidate parsers, and
diagnostic runners. Each negative case must ignore certificate admission.

The current census is:

| Control group | Elections | Scalar | v1 | v2 | Class 3 | Spill | Maximum branch | Stop |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Go query and rewrite | 3 | 2 | 2 | 2 | 0 | 1 | 1 | Later election lacks coverage |
| Go language | 4 | 3 | 4 | 3 | 1 | 0 | Branch mismatch |
| Go grammar generator | 1 | 0 | 1 | 0 | 1 | 0 | 1 | Branch mismatch |
| Erlang macros | 1 | 0 | 1 | 0 | 1 | 1 | 1 | Branch mismatch |
| Haskell smoke | 1 | 0 | 1 | 0 | 1 | 0 | 1 | Branch mismatch |
| JavaScript functions | 1 | 0 | 1 | 0 | 1 | 1 | 1 | Branch mismatch |
| Bash source | 3 | 2 | 3 | 2 | 1 | 1 | 1 | Branch mismatch |
| Perl pushes | 1 | 0 | 1 | 0 | 1 | 1 | 1 | Branch mismatch |
| Ada array | 1 | 1 | 1 | 1 | 0 | 0 | 0 | F4 history veto |
| Python greeting | 2 | 1 | 2 | 1 | 1 | 1 | 1 | Branch mismatch |
| Kotlin getters | 1 | 0 | 1 | 0 | 1 | 1 | 1 | Required decline |

## Certificate identity

Create one certificate arena for each `Core` instance. Give the arena a nonzero owner capability.

Allocate the owner when `Core.New` creates the instance. Use one process-wide atomic monotonic counter.

Concurrent `Core.New` calls must receive unique owners. Test 32 concurrent real `Core` instances.

Run the owner test with the race detector in Docker. Keep this gate separate from parser parity.

Run the concurrent owner contract with the Go race detector in Docker after implementation.

Reject `Core.New` before the owner counter can wrap. Do not derive the owner from a pointer.

Keep the owner stable for the life of the `Core` instance. Give each scheduler session a nonzero epoch.

Increase the arena epoch in these cases:

- A new scheduler session starts.
- `Core.Reset` starts a reused session.

Reject the session before an epoch can wrap. Do not reuse epoch zero.

Increase a nonzero cohort sequence for each split action. Reject the action before its sequence can wrap.

The certificate ID is this tuple:

`(arena owner, arena epoch, cohort sequence)`

Match the owner before any certificate lookup. Then match the epoch and sequence.

Count every owner-check attempt in `owner_checked_lookups`. Increment the counter before the owner comparison. Include successful local proofs, foreign-owner rejections, and reset-stale rejections. Do not count certificate-store reads.

Do not access an inline value, a spill entry, a map, or an interner before the owner matches.

Two arenas can have equal epochs, equal sequences, and equal payloads. Their certificates are still different.

Do not copy a certificate between parser arenas.

## Real Core contract surface

The RED contract binds a future API on the real `Core` instance. A test-only model cannot satisfy this contract.

Keep every `Core` method signature within the internal package boundary. Use
fixed-width primitive vectors, handles, digests, byte slices, and internal
`core.Head` values. Keep JSON snapshots and convenience adapters in the root
tests. The compile contract must prove that an internal provider can satisfy
the root adapter without importing the root package.

Use these operations and exact primitive argument classes:

```text
DiagnosticDropCohortArenaIdentityForTest() (uint64, uint64)
DiagnosticDropCohortSnapshotForTest() []byte
DiagnosticDropCohortOwnerWrapProbeForTest() (uint64, uint64, error)
DiagnosticDropCohortEpochWrapProbeForTest() (uint64, uint64, error)
DiagnosticDropCohortSequenceWrapProbeForTest() (uint64, uint64, error)
DiagnosticDropCohortLimitsForTest() (uint16, uint16)
DiagnosticDropCohortSetLimitsForTest(uint16, uint16) error
DiagnosticDropCohortBeginForTest(uint16, uint32, [7]uint64) ([3]uint64, error)
DiagnosticDropCohortWriteForTest([3]uint64, core.Head, uint16, [14]int64, [32]byte, []byte) error
DiagnosticDropCohortFinalizeForTest([3]uint64) error
DiagnosticDropCohortMarkUnprovedForTest([3]uint64) error
DiagnosticDropCohortRollbackForTest([3]uint64) error
```

The scheduler test surface binds opaque handles to real summary-receipt headers. It then calls the real drop method:

```text
DiagnosticBindDropCohortReferencesForTest([][3]uint64, []uint16) error
DiagnosticDropGenericNoActionHeadsForTest([]int) (string, error)
DiagnosticDropGenericNoActionHeadsNonDestructiveForTest([]int) (string, uint64, [32]byte, error)
DiagnosticDropGenericNoActionHeadsVerifierStateDigestForTest() [32]byte
```

The root adapter exposes the private activation seam separately from this
Core surface. Reset the seam with its returned function after every parse.
Prove that a cached parser run after restore has no certificate admission.

The state-changing methods must mutate the next arena snapshot. A static JSON response does not satisfy this contract.

The non-destructive verifier hook must execute the real verifier, return one invocation number, and restore the exact bound headers, receipt, and certificate state. Its returned digest must equal the verifier-state digest after restoration. It must not publish telemetry.

Each wrap probe uses the production atomic counter with a scoped test value. It must accept `MaxUint64`, reject zero, return an overflow error, and restore the arena snapshot.

The limit getter must return `MaxDropCohorts = 4,096` and `MaxDropCohortMembers = 32`. The setter must apply both limits before preflight. A rejected cap reservation must leave every store and byte total unchanged.

The owner-check counter is named `owner_checked_lookups` in the snapshot. Count every owner-check attempt before comparison. Include successful local proofs, foreign-owner rejections, and reset-stale rejections. Do not count certificate-store reads.

The real verifier allocation test binds exactly two real summary headers. It warms the non-destructive verifier, then requires 1,001 valid invocations and zero allocations. The extra invocation is `AllocsPerRun` warm-up.

Use the real parser heads in every verifier call. Do not use a test-only head or certificate model.

## Action identity

Derive the action identity from the exact dispatch row. Record these values:

- The parser state.
- The lookahead symbol.
- The action ordinal.
- The action type.
- The target state.
- The reduced symbol.
- The production ID.
- The child count.
- The dynamic precedence.
- The extra flags.
- The repetition flag.
- The no-lookahead context.
- The selected dispatch class.

Use the decoded action values. Do not infer identity from the output tree.

Mutate each action field independently. Require an action identity decline for every mutation.

## Derivation identity

Intern an exact semantic derivation record. A digest alone is not an identity.

The record contains:

- The root symbol.
- The stack depth.
- The production and token payload sequence.
- The child sequence.
- The field assignments.
- The aliases.
- The byte and point checkpoints.
- The recovery, extra, missing, and external flags.

Resolve a digest collision with full record equality. Keep each interned ID local to its arena epoch.

Test two unequal records with one forced equal digest. The verifier must decline that pair.

Test two byte-equal records with that digest. The verifier must accept that pair.

## Certificate members

Map each branch to one action and one derivation:

`branch -> (action identity, derivation identity)`

The branch is diagnostic data. It cannot replace either identity.

A certificate records one of these states:

- Building.
- Complete.
- Overflowed.
- Blended.
- Unproved.

Record `expected_members` when the producer reserves the certificate. Increase `written_members` after each complete member write.

Expose the certificate only after finalization. Finalization requires equal, nonzero expected and written counts.

Reject a partial certificate. Reject a duplicate member. Do not count a duplicate as a successful write.

An overflow changes the state to overflowed. A branch conflict changes the state to blended.

An unauthenticated import changes the state to unproved. These states are terminal.

Only a complete certificate can prove a drop.

The RED contract performs each transition on a real `Core` arena. A test-only reference model cannot satisfy this contract.

## Compatible cohort merge

Merge two building certificates only when their full cohort IDs are equal. Apply these rules:

1. Match the arena owner before a lookup.
2. Reject a complete or terminal certificate.
3. Reject a partial source.
4. Reject a duplicate member.
5. Reject a shared branch with different action or derivation data.
6. Add disjoint branches to the building result.
7. Finalize only after the written count equals the expected count.

Mark the result as blended when a shared branch conflicts. Do not publish a partial merge.

Do not union certificates from different cohorts. A flat union cannot prove a drop.

## Full verifier predicate

Evaluate all requested drops as one transaction. Admit the drop only when all conditions are true:

1. The drop list is nonempty and does not contain every head.
2. Each index is unique and in range.
3. Each certificate uses the current nonzero arena owner.
4. Each certificate uses the current nonzero arena epoch.
5. Each certificate uses the same nonzero cohort sequence.
6. Each certificate is complete and nonempty.
7. Each expected count equals its written count.
8. No certificate is stale, overflowed, blended, or unproved.
9. At least one head survives.
10. A surviving certificate contains each dropped action and derivation pair.
11. Every dropped head passes the same predicate.

Ignore the branch only after the action and derivation identities match. Reject the complete drop when one candidate fails.

Keep the metadata recovery proof separate. A cohort certificate cannot replace that proof.

## Caps and accounting

Add `MaxDropCohorts` and `MaxDropCohortMembers` to the compact limits. Use these default hard caps:

- 4,096 cohorts per scheduler session.
- 32 members per cohort.

Bound derivation records with `MaxDerivations`. Bound record bytes with the existing memory budget.

Preflight one reservation before the first write. The reservation covers these seven stores:

- Action identity records.
- Derivation identity records.
- Certificate references.
- Certificate maps.
- Derivation interners.
- Transaction journals.
- Cohort records.

Check every count and byte addition for overflow. Check every hard cap and the parser memory budget.

Apply each diagnostic preflight cap to one retained-footprint store. Fail before writes when one requested store exceeds its cap.

Do not write any store until the complete reservation succeeds. A failed preflight leaves all stores unchanged.

Charge every reserved value before producer writes begin. Release the charge on an exact rollback.

On a later member overflow, mark the certificate overflowed. Do not publish a partial proof.

Use fixed record layouts for exact accounting:

- An action record contains the 14 signed identity values.
- A derivation header contains the digest, offset, and length.
- A reference contains the cohort handle, real head, and branch.
- A map slot contains the hash, index, and used flag.
- An interner slot contains the digest, index, and used flag.
- A journal entry contains the store, index, and old value.
- A cohort record contains the handle, state, counts, and spill flag.

Use `unsafe.Sizeof` on mirrored layouts in the RED contract. Add the derivation payload bytes to each derivation header.

Set each map and interner capacity to the smallest power of two that is at least twice the expected member count.

`StorageBytes` counts reserved logical slots after preflight. `FootprintBytes` counts the retained capacity for those reservations.

Require the exact per-store vectors and their exact totals. Do not permit an unclassified byte.

Keep the warm verifier path at zero allocations. Poll the memory budget after each bounded producer write.

## Transaction and lifecycle rules

Add these values to the scheduler transaction journal:

- The arena owner.
- The arena epoch.
- The cohort arena length.
- The next cohort sequence.
- Every certificate state.
- Every expected and written member count.
- Every accounting counter.
- The derivation interner length.
- The certificate reference writes.
- The telemetry counters.

Rollback must restore all values exactly. Do not retain a certificate from a rejected action.

Support sequential cohorts in one scheduler transaction. Give each cohort a new sequence.

Support nested cohort construction. Keep the outer certificate building while the inner certificate completes.

Do not merge nested or sequential cohorts. Finalize each cohort against its own expected count.

`Core.Reset` clears logical lengths and increases the arena epoch. It can retain allocated capacity.

A reused parser starts a new epoch. All old certificate references must fail closed.

Do not store a certificate in a materialized `Tree`. A certificate belongs to one scheduler session.

An incremental old tree cannot carry a certificate. A fresh compact attempt must build a new certificate.

A future incremental import requires authenticated reconstruction from current parser data. Until then, decline the import.

## Dead-node history imports

Import dead-node history only when the producer supplies all of these values:

- The current arena owner.
- The current arena epoch.
- The exact cohort sequence.
- A complete action identity.
- A complete interned derivation record.

Count this result as an authenticated history import. Keep the Ada F4 veto for every other history import.

An unproved import clears the certificate and sets the unproved state. It must not authorize a drop.

## Telemetry contract

Publish test telemetry with schema `gts-drop-cohort-verifier/v1`. Include these fields:

- `arena_owner`
- `arena_epoch`
- `verifier_elections`
- `verifier_proofs`
- `verifier_declines`
- `profile_bypasses`
- `action_identity_declines`
- `derivation_identity_declines`
- `authenticated_history_imports`
- `unproved_history_imports`
- `producer_writes`

Record producer writes for these paths:

- Reduction establishment.
- The linear canonicalizer.
- The mapped canonicalizer.
- Sibling adoption.
- Conflict reconciliation.
- A dead-node history import.

Publish arena telemetry with schema `gts-drop-cohort-certificate-arena/v2`. Include these fields:

- The owner and epoch.
- A count for each certificate state.
- The expected and written member totals.
- Every producer write count.
- Every reserved accounting class.
- The count of every owner-check attempt before comparison in `owner_checked_lookups`.
- The inline, spill, map, and interner read counts.

It exposes these operations on a real `Core`:

- Read the arena identity.
- Read a complete arena snapshot.
- Begin a cohort with per-store caps.
- Write a member for a real parser head.
- Finalize a cohort.
- Mark an import as unproved.
- Roll back a cohort.
- Verify one drop request with real parser heads.

The scheduler test hook binds certificate handles to valid summary-receipt headers. It then calls `dropGenericNoActionHeads` without another decision path.

The bind step copies opaque handles only. It does not validate an owner, epoch, sequence, state, action, or derivation.

Use this hook for the equal-session foreign inline, foreign spill, and reset-stale tests.

The behavior API uses primitive arrays in its method signatures. Production does not depend on test-local types.

A static JSON response cannot pass the contract. Each operation must change the next snapshot as specified.

For a foreign or stale verifier call, count the owner-check attempt before comparison. Reject the handle before any inline, spill, map, or interner read. Only the owner-check counter may increase.

Snapshot telemetry immediately before and after each real producer call. Require only that producer counter to increase by one.

Require every other producer counter to stay unchanged. Cover all six registered producer paths.

The real `Parser` route tests must read verifier telemetry. They must also match locked C.

The real `Parser` must also expose `DiagnosticDropCohortVerifierReceiptsForTest() []byte`. Decode its JSON array as one receipt per election. Each receipt contains `arena_owner`, `arena_epoch`, `cohort_sequence`, `verdict`, and one `classification` string.

Each diagnostic receipt records the full cohort ID, the verdict, and one decline reason. Production can keep aggregate counters only.

The future target controls require one proof for each future election. They do not pin the current fallback census.

They require a nonzero election count, `proofs == elections`, zero profile bypasses, and zero declines.

The Ada target requires an authenticated history import. All other target controls require zero history imports.

The two Kotlin controls must decline. Constrain each control to one election. Require `proofs + declines == elections` for each control.

Require exactly one classified identity reason in every declined receipt. Reject an unclassified or multiply classified decline.

Their combined telemetry must record both action and derivation identity declines.

## RED tests

The future RED test removes every target grant. It then uses the real parser candidate route.

Each target must produce these results:

- The candidate routes directly.
- The fallback counter does not change.
- The tree equals locked C.
- The parser publishes exact verifier telemetry.

The current characterization compares the grant-free fallback tree directly with locked C.

Current main fails before direct admission and does not publish telemetry. This failure is intentional.

The safety packet also tests these cases:

- A valid blended survivor.
- A valid unproved history import.
- A foreign inline certificate.
- A foreign spilled certificate.
- A reset-stale spilled certificate.
- Equal-epoch and equal-sequence arenas with identical payloads.
- An arena A certificate accepted in arena A.
- An arena B certificate rejected in arena A.
- Two concurrent real arenas with unique owners.
- An atomic owner counter wrap.
- A Kotlin action mismatch.
- A Kotlin derivation mismatch.
- A cohort conflict.
- An arena reset and reuse.
- An epoch wrap.
- A cohort sequence wrap.
- A cohort cap.
- A member cap.
- The exact 32-member default.
- Cohort and member cap overflow with atomic preflight.
- A partial finalization.
- A duplicate member.
- An exact rollback.
- A sequential cohort.
- A nested cohort.
- An atomic preflight for every store class.
- Exact storage and retained-footprint vectors.
- Every action identity field mutation.
- Equal-digest unequal derivation records.
- Full derivation record equality.
- Zero allocations in the real future verifier.

The producer-path tests call reduction establishment, both owned canonicalizers,
sibling adoption, conflict reconciliation, and dead-node history import.

Each future producer test requires telemetry from the real certificate arena. The current passing tests remain current-state characterizations.

## Review run set

Run these focused Docker commands from the packet worktree:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace && go test ./internal/parsercorephase0 -run '^TestG18HistoricalAlternativeSetProducerPath$' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace && go test ./internal/parsercorephase0 -run '^TestG18HistoricalAlternativeSetCertificateTelemetryRED$' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace/cgo_harness && go test . -tags cgo,treesitter_c_parity -run '^TestG18AlternativeSetCurrentFallbackCharacterization$' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace/cgo_harness && go test . -tags cgo,treesitter_c_parity -run '^TestG18AlternativeSetCertificateRED$' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace/cgo_harness && go test . -tags cgo,treesitter_c_parity -run '^TestG18AlternativeSetKotlinOppositeControls$' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace/cgo_harness && go test . -tags cgo,treesitter_c_parity -run '^TestG18AlternativeSetKotlinFutureDeclineTelemetryRED$' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace && go test . -tags gts_parsercorephase0 -run '^TestG18Current' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace && go test . -tags gts_parsercorephase0 -run '^TestG18Future' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace && go test -race . -tags gts_parsercorephase0 -run '^TestG18FutureConcurrentArenaOwnerAllocationRED$' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace && go test . -tags gts_parsercorephase0 -run '^TestG18Reference' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh --no-build -- \
  "cd /workspace && go test . -tags gts_parsercorephase0 -run '^TestG18DropPathRejectsForeignInlineRED$' -count=1"
```

The current characterization commands must pass. The future commands must fail at the RED boundary until production implements the contract.

## Memory gates

Run allocation and storage tests in the focused package gate. Run the resident set size gate after production exists.

Use five isolated runs for each target grammar. Run one grammar at a time.

Measure each run with `/usr/bin/time -v`. Record the maximum resident set size from every run.

Accept a candidate only when its maximum is within this limit:

`baseline maximum + max(1 MiB, 5 percent)`

Use the same Docker image, grammar, command, and limits for both samples. Stop on an out-of-memory result or timeout.

## Artifact binding

The corrected packet binds to Git head:

`f60aee0ae93c3f525d210e18c788560d48aa7aa0`

The Docker image identity is:

`sha256:8988c75c874ba63954c48091176cfa0380ee87f493a5b5d577f368e78921475f`

Create the path digest with this exact serialization:

```sh
sha256sum \
  cgo_harness/compact_g18_alternative_set_contract_test.go \
  docs/compact-g18-alternative-set-design.md \
  internal/parsercorephase0/g18_alternative_set_contract_test.go \
  parsercore_phase0_g18_alternative_set_contract_test.go \
  | LC_ALL=C sort -k2 \
  | sha256sum
```

The input has one GNU `sha256sum` line per path. Each line uses two spaces before the ordered relative path.

The external packet manifest records these values:

- The full Git head.
- The four individual file hashes.
- The combined path digest.
- The target manifest digest.
- The Docker image identity.
- Every command and artifact directory.
- The SHA-256 of each artifact `container.log`.
- The SHA-256 of each artifact `metadata.txt`.
- The SHA-256 of each artifact `inspect.json`.

## Production gate

Do not implement this design before an independent review accepts it. Keep all profiles unchanged at the RED boundary.

A later implementation must pass these gates:

1. Route every target without its profile grant.
2. Match locked C for every target.
3. Reject both Kotlin controls.
4. Reject all stale, foreign, blended, overflowed, and unproved certificates.
5. Allocate unique owners for concurrent real `Core` instances.
6. Preserve exact rollback, reset, reuse, and incremental behavior.
7. Preserve exact per-store storage and retained-footprint accounting.
8. Reject every action mutation and each unequal derivation record.
9. Preserve the scorecard of 198 PASS, 3 FALLBACK, 5 SKIP, and zero defects.
10. Preserve every parser memory contract.

Retire each profile flag only after its grammar passes every route gate.
