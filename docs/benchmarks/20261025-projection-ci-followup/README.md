# Missing-token projection and CI follow-up

This bucket fixes the follow-up failures from `44c44cd3559eb83c7dc8f03d5570f96291ace9b2` without dropping its shared missing-token correctness fixes. It adds no language graduation, default switch, reuse activation, or main merge.

## Go: preserve evidence when projection hides a missing separator

The 159-byte grouped-declaration witness matched C's root error but lost the nested const declaration's error flag. Its selected raw history had been elided. Before filtering a genuine MISSING separator, Go compatibility now captures the selected child entries in the existing arena, only when history is absent. Existing histories and the generic visibility filter remain unchanged. There is no new Node/Tree field or extra full-tree walk.

The original and explicit-separator controls retain their source and locked-C digest guards. Both routes match C on all axes. Two delete/restore cycles of the const separator run through both incremental APIs: **20 checked states**, each comparing fresh/incremental Go and C, nested errors, exact digests, and zero-allocation no-edit reparses. Fourteen unit controls cover both child layouts, stale/discarded/repaired-visible negatives, old-history identity and idempotence. The clean declaration-extraction fixture now uses the explicit separators the pinned C grammar requires; the original omission remains a strict C regression.

## COBOL: repair only the source-backed class-header shape

Retained errors exposed two existing compatibility assumptions: tail cleanup erased a recoverable header, and class repair skipped it. The constrained fix requires the observed EXEC/zero-width AND/CICS geometry, genuine hidden `_AND` evidence, and absence of unrelated public ERROR/MISSING nodes. Unknown shapes retain their previous exclusions. Prefix trimming and repeated normalization retain their existing rules.

Stronger checks also exposed a pre-existing omission: the synthesized EXEC error lacked C's `Extra` flag. That flag is now set. All five fixtures match C on **symbols, fields, spans, points, Missing/Error/Extra/HasError flags and independent deep digests**, on both routes. The stronger assertion remains in the permanent suite; none of its original assertions was removed. Failed unrestricted and incorrectly positioned candidates remain in the evidence.

## Cooklang and root layout

Cooklang's existing complex-witness diagnostic hashes change only in HasError bits. Exact before/after probes cover **45 route rows / 38 returned trees**: all **19 previously C-exact results** remain exact. Shape digests excluding only HasError stay identical for the four changed rows. Seven forest declines remain explicit. The two diagnostic hash literals are updated; all source/C identities and known-divergence assertions remain. The complex witness still differs from C and this is not Cooklang graduation evidence.

Four new root test files are consolidated into existing files with their test bodies preserved. The root returns to **756/756 Go files**; the layout/API guard passes. The new Go and COBOL unit tests also live in existing root test files.

## Final saved-source qualification

The final gate compiles saved files without Go overlays in a network-disabled, one-CPU, 4 GiB Docker container, using Go 1.26.6 on Linux amd64 and locked C 0.25.1. It passes:

- **412 invariant sessions / 29,664 edit steps**, zero no-edit allocations, on 206 grammars and both routes. This includes 408 real R4 sessions and four CSV/Enforce grammar-smoke sessions.
- **618 C fixture checks**, with no skips or failures; separate strengthened Go/COBOL and C/EDS regressions also pass.
- Focused memory/lifetime checks and the separately completed combined-source race gate.
- **All 412 ledger rows and all 24 complete Go operation records identical** to the qualified source before this follow-up.

The ledger baselines and thresholds are unchanged. The earlier [four C/EDS exceptions](../20261025-reserved-missing/counter-review.md) and [Go assignment exception](../20261025-go-graduation-recovery/counter-review.md) remain unapproved. This bucket adds no counter exception. It makes no timing or RSS improvement claim; the prior timing receipt retains its own source scope.

[Verification](verification.json), [source identities](source-identities.json), [raw evidence](evidence.tar.gz) and [archive checksum](evidence.sha256) retain exact commands, source maps, failures, counters and logs. Executables, caches and private corpus payloads are excluded.

## Graduation remains held

Current R4 Go/YAML timing still exceeds the E-A worst-file gate; the complete all-206 timing exit, frozen M1 baseline, upgraded C oracle and remaining platform/per-language gates are incomplete. Native non-EOF recovery remains fail-closed. No private native recovery, instrumentation split, or dense-stack prototype is promoted here. All merges to main remain held.
