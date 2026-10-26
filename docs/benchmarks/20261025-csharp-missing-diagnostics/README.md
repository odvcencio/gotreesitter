# C# retained-MISSING diagnostic hashes

This test-only follow-up starts at `d82ca38e6077d62dcc210caeb09bd52b34a89cab`. It changes four Go diagnostic hash literals, with two distinct values, for `a0-jsontextreader` and `malformed-missing-body`. Runtime code, source witnesses, C identities, C expectations and route expectations are unchanged.

Exact pre-change source overlays and current source run the same four witnesses through all five routes in isolated Docker. The comparison contains **20 rows, 17 returned trees and three explicit forest declines** on each side. All **nine previously C-exact results** remain exact. The eight changed returned-tree digests belong to the two existing C-gap witnesses; their canonical digests with only HasError omitted remain identical. No accepted route is lost or added.

The preprocessor node now agrees with C's HasError flag, so the two first-divergence records advance to the pre-existing deeper parameter-field mismatch (`name` versus C's `type`). Its known C gaps remain explicit. The original pinned test passes against saved files without overlays. The adjudication processes deliberately report the known gaps as failures; they are retained as evidence, not reclassified as C parity passes. This is not C# graduation evidence or a performance improvement.

[Comparison](comparison.json), [source hashes](source-identities.json), [raw evidence](evidence.tar.gz) and [checksum](evidence.sha256) preserve exact controls and outputs. This follow-up does not change a ledger baseline, require a new counter exception, promote a private parser prototype, or authorize a main merge. The earlier owner exceptions and all graduation/merge holds remain in effect.
