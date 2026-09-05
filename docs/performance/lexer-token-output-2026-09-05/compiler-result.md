# Caller-owned token: compiler result

The candidate removes the six Token transfers between the contiguous scanner and each converted caller.
It adds one completed-token transfer inside the scanner for successful and skip results.
Those paths therefore remove five complete transfers, or 440 copied bytes per raw scan.
Failed and invalid-state paths remove six complete transfers, or 528 copied bytes per raw scan.
These counts exclude initialization, outer returns, and write-barrier work.
They describe generated instructions, not measured speed.

## Inputs and method

Inspect existing binaries with anchored `go tool objdump -s` expressions.
No source changes, builds, or tests ran during this inspection.
Both binaries use Go 1.25.14 for linux/amd64, with `gts_parsercorephase0`.

- Baseline: `/tmp/gts-authenticated-edit-optimization-20260905/baseline.test`
- Baseline SHA-256: `80a9594c3ab311dce059f036385e913d80e5de9eeb036ca0ba38c49a6387c532`
- Candidate: `/tmp/gts-lexer-token-output-20260905/candidate.test`
- Candidate SHA-256: `7e0338eda3b32b6af9cca00374ef66c0f573dc15f5e330a3a9778611b09979ac`

In both binaries, `runtime.duffcopy` starts at `0x4835e0`.
Entry `0x48391a` copies 80 bytes.
Each complete Token transfer also copies an eight-byte head through scalar instructions.
The complete Token size is therefore 88 bytes.
Entry `0x4838aa` copies 208 bytes for Lexer.

## Removed transfers

| Boundary | Baseline | Candidate |
| --- | --- | --- |
| Contiguous scanner to shared wrapper | Two Token transfers at `0x6e664a` and `0x6e668a`. | No transfers after `scanContiguousInto` at `0x6e64d5`. |
| Shared wrapper to its return slot | Two Token transfers at `0x6e672a` and `0x6e676a`. | `scanInto` returns only the acceptance boolean. |
| Shared scan to proof helper | Two Token transfers at `0x9d9cf0` and `0x9d9d2a`. | No transfers after `scanInto` at `0x9d9a55`. |
| Shared scan to `nextWithFrontier` | Two Token transfers at `0x6e5b0a` and `0x6e5b4a`. | No transfers after `scanInto` at `0x6e5ae0`. |
| Shared scan to `canLexAt` | Two Token transfers at `0x6e60ea` and `0x6e612a`. | No transfers after `scanInto` at `0x6e603a`. |

The proof helper's successful Token return remains at `0x9d9b6a`.
Its private Lexer copy remains at `0x9d998a`.
Its two returned Lexer copies remain at `0x9d9b8d` and `0x9d9bb0`.
Ordinary next-token returns and recovery returns also retain value copies.

## New scanner transfer and write barriers

The accepted-token path retains its constant-template initialization at `0x6e6fea`.
It adds one complete transfer from that local token to caller storage at `0x6e70ca`.
The skip path adds one complete transfer at `0x6e6f4e`.
The failed and invalid-state paths clear caller storage directly.

Pointer-bearing output assignments introduce guarded write barriers:

- Invalid state: guard `0x6e6708`, conditional `runtime.wbZero` at `0x6e6720`.
- Failed scan: guard `0x6e6dd5`, conditional `runtime.wbZero` at `0x6e6dec`.
- Skip result: guard `0x6e6f03`, conditional `runtime.wbMove` at `0x6e6f20`.
- Accepted token: guard `0x6e707b`, conditional `runtime.wbMove` at `0x6e7096`.

The baseline scanner writes into its known return slot without these barriers.
The candidate's barriers are a real compiler tradeoff and require performance measurement.

## Included ranges

The included-range scanner retains its value-returning implementation.
`scanInto` copies that returned token twice, at `0x6e644a` and `0x6e64ca`.
The output assignment adds a barrier guard at `0x6e645d` and conditional `runtime.wbMove` at `0x6e6480`.
The previous shared wrapper performed four transfers on this route.
Converted callers also avoid their two former receipt transfers.
Do not describe this route as free of token copies.

## Allocation and frame evidence

All three converted callers pass addresses of local stack slots in register R8:

- `nextWithFrontier`: `SP+0xa8`, loaded at `0x6e5ad5`.
- `canLexAt`: `SP+0x40`, loaded at `0x6e6035`.
- Proof helper: `SP+0x38`, loaded at `0x9d9a50`.

None of the five inspected candidate methods calls `runtime.newobject` or `runtime.mallocgc`.
No new token heap allocation is visible in these call paths.
This is narrow assembly evidence, not a claim about every transitive parser allocation.

| Method | Baseline frame reservation | Candidate frame reservation |
| --- | ---: | ---: |
| Shared scan wrapper | 304 bytes | 216 bytes |
| Contiguous scanner | 160 bytes | 344 bytes |
| Proof helper | 520 bytes | 352 bytes |
| `nextWithFrontier` | 352 bytes | 344 bytes |
| `canLexAt` | 320 bytes | 152 bytes |

These sizes come from each method's `SUBQ` instruction.
They exclude caller argument areas and stack growth.
The contiguous scanner frame grows because its output literals now use local storage.

## Raw evidence

The `compiler-*.txt` files beside this report contain bounded, single-symbol disassembly.
The `compiler-baseline-*` files preserve the five baseline methods.
The remaining files preserve the five candidate methods and `runtime.duffcopy`.
