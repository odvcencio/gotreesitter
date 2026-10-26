# YAML raw checkpoint ownership and construction screen

**Hold this prototype.** Ordinary YAML can publish an immutable raw-backed tree on the 1,998-byte R4 sample and the original 8,012-byte and 32,048-byte document streams, with exact C output. Construction work falls, but the final candidate is **39.30% slower** than the current compact baseline. The default legacy route also regresses **16.40%**. No runtime source, admission allowlist, reuse authority, or main merge is promoted by this receipt.

The prototype transfers raw records and checkpoint storage into the result instead of building another complete public tree. It creates one public root header initially. It retains hidden scanner checkpoint changes, binds the exact scanner/grammar/source/version, and remains fresh-only: incomplete public checkpoint coverage leaves `CompactExternalScannerCheckpointTransferProven=false` and disables subtree reuse. No-edit remains allocation-free; edits retain fresh-result verification.

Two follow-ups preserve every identity callback while returning scanner/grammar identities by value, and reuse a newly formed exceptional child list instead of projecting it twice. Existing summaries still project each new incoming context. Closure checks, field/alias limits, recovery refusal, memory reservations, and mutation checks remain in place.

| Final R4 comparison | Baseline | Candidate | Change |
| --- | ---: | ---: | ---: |
| Compact parse and release | 192.118 µs | 267.620 µs | +39.30%, p<.001 |
| Compact allocated bytes | 5,456 | 7,104 | +30.21% |
| Compact allocations | 39 | 53 | +35.90% |
| Default legacy parse and release | 136.316 µs | 158.665 µs | +16.40%, p<.001 |
| Default legacy allocated bytes / allocations | 1,440 / 16 | 1,440 / 16 | unchanged |

Each of three screens contains 20 matched seed pairs, alternating arm order and matching shuffled workload order, using `scripts/run_randomized_benchmarks.sh`, GOMAXPROCS=1, count=1, 750ms and benchmem. All 120 processes completed. The earlier screens also regress; their compact allocation counts are 341 and 293. Their timings are independent baseline comparisons, not paired evidence of speedup between prototype versions. Timing uses Linux/amd64, Go 1.26.6, one pinned CPU and a network-disabled 4 GiB container. It excludes diagnostic instrumentation and includes no API traversal or query workload.

The separate deterministic formation comparison uses the same three actual sources and complete public walks. Every other formation counter, output count, retained-byte value, and read-work field is identical. A repeated warm walk adds no work.

| Source bytes | Raw construction edge visits | Local root references | Hidden traversal steps |
| ---: | ---: | ---: | ---: |
| 1,998 | 1,578 → 1,212 | 773 → 467 | 120 → 60 |
| 8,012 | 6,448 → 4,950 | 3,177 → 1,926 | 494 → 247 |
| 32,048 | 25,804 → 19,806 | 12,717 → 7,710 | 1,982 → 991 |

The standard YAML ledger changes only candidate full and first-edit public-node allocations: **232→1** in each. Both default-route rows and every other field are identical. No owner counter exception is used or inferred.

Selected correctness coverage passes: Core and raw-construction tests; source/scanner/callback mutation rejection; memory contracts; copy, materialization, producer reset, release and concurrent readers; two 72-step YAML edit sessions; and no-edit allocation checks. The same final runtime passes ten scoped YAML C families against both locked C 0.25.1 and pinned C 0.27.1 (`10eb5ac119eec9cb4267a8e035066473983f5f3a`). The latter uses the previously frozen private default-API adaptation of the Go v0.25.0 binding; it is not a general binding upgrade. Full tree digests agree between both oracle runs.

All 231 R4 public records match Native output. Every known parser state is independently supported by the selected GSS history: 230 post states and 209 pre states, with zero unproved states. This does **not** claim numeric identity with legacy's different valid selected history: the retained census finds 16 non-extra pre-state differences, 21 extra pre-states marked unknown, zero post-state differences, and one synthetic-root private production difference. Public structure, flags, fields and queries match; copies and materialization preserve the original lease's complete state plane.

The original ownership assertion and the first dual-contract attempt remain as failed evidence. The unchanged baseline assertion still passes. With actual checkpoint tracking enabled, the old native publisher must refuse hidden scanner state and fall back; the fresh raw path must retain ownership without granting public-sidecar completeness. Both outcomes now have exact-C checks. A C 0.27 preparation attempt stopped at a missing checksum file before tests; its corrected runs are retained separately.

The final attribution profile still places 27.57% cumulative CPU in summary capture, within reduction construction's 35.01%; those figures overlap. Checkpoint snapshot copies and version-state publication dominate remaining allocations. This evidence points to integrating alias/field output construction with reduction, and separating fresh checkpoint ownership from work imposed on the default route. It does not justify removing checks. Large-file RSS, API-consumer timing, ARM64, the full grammar fleet, the authenticated M1 baseline and graduation remain unqualified.

[Summary and all three timing tables](summary.json), [offline verification](verification.json), [compressed source/control/raw-log bundle](evidence.tar.gz), and [bundle index](contents.json.gz). The bundle inherits the immutable [raw-output evidence](../20261025-compact-raw-screen/evidence.tar.gz), SHA-256 `73fe52b4ef05a02a6741265da9b447ed3e2610338880592a6d1ff57c5f8c90fa`.

Run the offline validator from this checkout:

```sh
python3 docs/benchmarks/20261026-yaml-raw-lease-screen/validate-evidence.py --source-root "$PWD"
```

The archive contains the exact overlay maps, source backings, Docker commands, guards, benchmark runner controls, raw samples, C probes and profiles. Source guards identify 3,817 canonical files; the final runtime map contains 35 replacements. The tested canonical baseline is `810f7126fe2b231d7320359ddfe059396798fddc`; prototype code is retained only in the review bundle.
