# Owner review: C and EDS missing-token correctness

Status: **approved by the owner on 2026-10-25**: “Approve these four correctness exceptions.” The [approval record](owner-approval.json) pins the decision and four ledger values. Only those four guarded values are updated; the 2% threshold is unchanged. [Scoped validation](ledger-validation.json) reproduces all four values; all eight existing ledger guard-test records pass. The [full hosted ledger gate](https://github.com/odvcencio/gotreesitter/actions/runs/40214468808/job/121232961633) subsequently passes all 206 grammars / 412 rows at commit 66cd77478. All merges to main remain held.

The [v1 design R3](../../v1-design.md#r3-counter-ledger-changes) says: “Agents do not approve ledger exceptions.” The owner approved these four guarded changes on the default route's first pinned-session edit:

| Fixture | Guarded counter | Before | After | Change |
|---|---|---:|---:|---:|
| C, first edit | block splices | 148 | 24 | −83.78% |
| EDS, first edit | new nodes | 4806 | 4930 | +2.58% |
| EDS, first edit | maximum live versions | 1 | 2 | +100% |
| EDS, first edit | reused bytes | 9873 | 9664 | −2.12% |

These are owner-approved correctness costs. Before the change, C's fresh and incremental edited trees have 21 root children where C has 14. Both now match C exactly. EDS's original and edited Go trees incorrectly report `section.HasError()=false` where C reports true; both now match fresh and incremental C across all axes and deep digests. The counter-equivalent private probes reproduce the exact collector sequence before any tree walk, C setup, additional parse or logging can affect the first edit. Four permanent regressions cover these results in separate processes.

The complete changed rows are in [counter-deltas.json](counter-deltas.json). C tokens improve **320→16**, nodes **1274→125**, and the aggregate reused-byte counter **1340→2020**. Its multi-version token share changes **165625→500000 ppm**; the corresponding observed token counts fall **53→8**. Maximum versions stay 8. EDS tokens rise **2593→2627** (+1.31%) and block splices **1028→1084** (+5.45%). These directional changes remain explicit even when they do not trip a guarded threshold.

All other **410 fleet rows** are identical. Go R4 and all 24 complete Go operation records are identical. The 412 invariant sessions, 618 C fixture checks, focused memory/race/publication gates and final-source checks pass. Go/YAML paired timing does not establish a compact throughput change. No C/EDS timing improvement is claimed.

The original C sample is 2049 bytes, SHA `ccdf9a3671f4f291c653fce2ee409557cadddc893494c112ce49a1cf9c139917`; its first edit inserts at byte 207. EDS is 28828 bytes, SHA `e874774d9088e4605cffedaf3af14d98aa1a68f76e24748194468b5fe3300226`; its first edit inserts at byte 11101. Source, session, edited-input and artifact hashes are retained in the archive.

A private Go-only alternative avoids these counter changes but leaves the C and EDS parity defects. It is not the proposed implementation. The earlier [Go assignment exception](../20261025-go-graduation-recovery/counter-review.md) remains a separate pending owner decision.
