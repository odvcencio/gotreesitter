# Compact graph-link storage: draft qualification

Common graph links use 20 bytes instead of the 32-byte logical record. A separate 24-byte record preserves full-width scores, orders and unknown flags when they do not fit the common representation. This is a storage-layout change; **throughput and peak-RSS qualification remain pending**.

The runtime is based directly on main `afd16867b666dfec05f3eb28e9eee86036474532` and does not depend on held PR #1509. The [source scope](source-scope.json) pins all 49 changed Go files: 20 production files and 29 tests. The public Node stays 104 bytes on amd64 and 80 bytes on actual 386 execution; the root Go-file count stays 756. Routing, allowlists, limits and ledger baselines are unchanged.

## Completed evidence

All **412 selected work/reuse rows match exactly**, including decline reasons. The [validation archive](native-validation.tar.gz) retains `counters-before.json` and `counters-after.json`; both have SHA-256 `f56a46f0c76959e34f1a77e76b346b52f6f22406d953286cc12f24225635f243`.

| Selected node work | Before | After |
| --- | ---: | ---: |
| Go candidate full | 1,251 | 1,251 |
| Go candidate first edit | 1,553 | 1,553 |
| Go default full | 1,922 | 1,922 |
| Go default first edit | 3,810 | 3,810 |
| YAML default full | 288 | 288 |
| YAML default first edit | 480 | 480 |

Ordinary YAML's candidate route still declines at the same ownership boundary. These selected counters do not measure hidden verification time and do not establish throughput improvement.

Native Linux amd64 Docker passed 723/727 Core tests, 12/16 actual 386 tests and twenty memory-contract tests per baseline/candidate. The candidate passed 27 focused race cases. New storage tests cover wide values and unknown flags, rollback/clone isolation, 4,000 deterministic differential mutations and retained-capacity accounting.

The main-based invariant sweep now completes **412/412 sessions (29,664 edit steps)** with zero no-edit allocations. The interrupted run retains 235 completed sessions; a separate run completes the remaining 177 using the same frozen binary and source hashes. The initial container was subsequently confirmed exited with code 137 and no container OOM flag. Its incomplete log remains in the original archive. [Follow-up metadata](qualification-followup.json) and the [follow-up archive](followup-validation.tar.gz) preserve the resumed sessions.

[Qualification metadata](qualification.json), [raw native logs](native-validation.tar.gz) and [member hashes](native-validation-members.json) preserve this partial snapshot. The source snapshots remain frozen. The earlier Mac trial used the same Core change over the held incremental branch; its separate source and benchmark results are not a substitute for main-only qualification.

CI found a test-only `go vet` error in snapshot `ec89776`: the link-footprint helper copied `Core`, including its mutex. The follow-up measures the footprint with link storage temporarily removed from the test-owned Core and then restores it. Production code and frozen benchmark inputs are byte-identical. The archived native results describe the original snapshot. Compilation and the complete Docker session gate pass at `b37393e`; its [manual CI run](https://github.com/odvcencio/gotreesitter/actions/runs/39822516577) retains all 412 session logs and a verified source hash.

## Open gates

Manual full CI now runs the R4 edit-session gate in Docker for all 206 languages on both routes. Each session has its own process; the job requires all 412 sessions, 72 steps each, and zero no-edit allocations. The first repaired job passes. It records source, counter, binary, image and container evidence; artifact `12445458065` has verified SHA-256 `4dbdd1da82f177528c1d10c74c23b88862097a534a9beeb9de443c4c7093da67`.

The main-based locked-C run passes all 1,236 standard checks and large Go/YAML probes. The exact Cogno fixture passes clean insert/replace C comparisons and all incremental/fresh Go comparisons; the four malformed-delete C failures per variant remain failures. Owned YAML's two invariant sessions and selected counter rows, plus four canonical Core/Scheduler work vectors, also pass unchanged. The recovered [Mac performance experiment](mac-experiment.md) reduces memory but regresses Go throughput, and uses the held incremental runtime. Finish main-only peak-RSS and randomized timing qualification.

Cognociente's [October 21 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850) remains open. Its exact Go fixture and both public incremental APIs are included in the completed main-only C run. The known malformed-delete C mismatch, Windows ARM64 timings and broader Windows matrix remain unresolved. The separate small-document counter exception in #1511 is unapproved. This draft neither changes that exception nor clears #1509's absolute RSS hold. No compact graduation or merge is qualified by this receipt.
