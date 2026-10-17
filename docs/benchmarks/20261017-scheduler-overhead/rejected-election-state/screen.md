# Election-state storage screen — 2026-10-17

**Rejected for runtime promotion.** Splitting the election record reduces its
retained storage, but establishes no compact Go throughput gain and slows the
owned YAML stream. The runtime stays unchanged by this screen.

The effective baseline is `87997c2fa33d0789e057827f031eabcdaec88ca5`.
A full-source overlay freezes all 3,503 tracked Go files. The separately pinned
candidate is retained as a patch; no state-split engine source is committed.

## Representation and correctness

The old election record is **264 bytes**, measured with the pinned compiler.
An earlier source comment incorrectly said 280 bytes. The prototype keeps a
120-byte active state, an 80-byte cold checkpoint pair, and the existing
authoritative scheduler token. Total retained election storage falls
**264 → 200 bytes (−24.24%)**. Summary/owned binding copies shrink
**264 → 120 bytes**. These are representation measurements, not heap-allocation
or throughput improvements.

The public receipt record remains unchanged and is projected when observed or
retained. The election state vector remains available for EOF quiescence.
Nested binding restores the complete prior record after callbacks, errors and
panics; stale cold checkpoints cannot appear in summary/owned projections.
Checkpoint authentication, callback order and route decisions remain intact.

## Twenty paired comparisons per language

Runs use the repository randomized runner, Go 1.25.14, `GOMAXPROCS=1`, CPU 6,
6 GiB Docker, `GOMEMLIMIT=5GiB`, `-count=1`, `750ms`, and `-benchmem`.
Baseline/head order alternates by seed, with one process per seed and both
benchmark locks held. YAML seeds are **28101–28120**; Go seeds are
**28201–28220**. All 440 timed rows are retained.

| Path | Time direction | p | Result |
| --- | ---: | ---: | --- |
| Go rewrite | −0.14% | .947 | Inconclusive |
| Go query | +0.20% | .192 | Inconclusive |
| Go language | +0.28% | .221 | Inconclusive |
| Go large | −0.82% | .157 | Inconclusive |
| Go R4 compact | +0.33% | .211 | Inconclusive |
| Default Go full | +0.10% | .565 | Inconclusive |
| Default Go first edit | −0.09% | .968 | Inconclusive |
| Default Go no edit | +2.41% | <.001 | Control; bypasses the changed operation |
| Ordinary YAML attempt plus fallback | −0.10% | .867 | Inconclusive |
| Owned YAML R4 | −0.29% | .192 | Inconclusive |
| Owned YAML 8 KiB stream | **+0.56%** | **<.001** | **Regression** |

Allocation counts remain exact on every path. No-edit remains 0 B/op with zero
allocations. The half-byte movement in Go language's byte median is rounding
sized and supplies no memory claim. The no-edit control's timing movement is
recorded without attributing it to election-state copies.

## Qualification and limits

All **412 ordinary plus two owned-YAML counter rows** remain exact. Full core
tests pass normally, with phase-zero-A, races and 386. Focused scheduler,
immutable-golden, owned-binding, nested-restoration and EOF tests pass.
Go/YAML edit sessions and the diagnostic-owned YAML session pass all 72 steps,
including allocation-free no-edit reparses. Locked-C fixture checks pass for
Go/YAML, with additional owned-stream, ownership, EOF-recovery and contextual
relex cases.

Full 206-grammar, strict real-corpus and standalone large-RSS qualification are
not run for this rejected source. No counter exception, routing change,
allowlist change or graduation follows. Raw samples, benchstat output,
measurement/source pins and the candidate patch preserve the negative result.
