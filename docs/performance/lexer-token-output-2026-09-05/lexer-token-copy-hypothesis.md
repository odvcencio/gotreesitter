# Shared lexer token copies

Inspect the existing binary only. No source changes, builds, or tests ran.

- Binary: `/tmp/gts-authenticated-edit-optimization-20260905/baseline.test`
- Compiler: Go 1.25.14, linux/amd64, GOAMD64=v1.
- Build tag: `gts_parsercorephase0`.
- Source baseline: `da1150c6d6a2d581ce31f44ba4c5b8241ec431ae`.

## Confirmed copy sizes

`runtime.duffcopy` starts at `0x4835e0` and ends at `0x483960`.
Entry `0x48391a` copies five 16-byte blocks: 80 bytes.
A preceding scalar load and store copy another eight bytes at each Token transfer.
The complete Token transfer therefore copies 88 bytes in this binary.
Entry `0x4838aa` copies thirteen 16-byte blocks: 208 bytes for Lexer.

## Shared wrapper

`Lexer.scan` executes four complete Token transfers on either scan route.

| Route | Call address | Copy purpose |
| --- | --- | --- |
| Contiguous | `0x6e664a` | Copy the callee return slot to temporary storage. |
| Contiguous | `0x6e668a` | Copy temporary storage to local `tok`. |
| Included | `0x6e658a` | Copy the callee return slot to temporary storage. |
| Included | `0x6e65ca` | Copy temporary storage to local `tok`. |
| Common return | `0x6e672a` | Copy local `tok` to the wrapper return slot. |
| Common return | `0x6e676a` | Copy the same local to the same return slot again. |

Each route performs its two transfers, then both common transfers.
The two common transfers execute consecutively, without an intervening conditional branch.
Both use source `SP+0x80` and destination `SP+0x140`, including their scalar heads.
The wrapper therefore moves 352 bytes through complete Token transfers per completed scan call.
This count excludes initialization and copies in callers or callees.

## Proof helper

`tokenInvariantProbeDFALimited` calls `Lexer.scan` at `0x9d9cc4`.
It then performs two complete Token transfers at `0x9d9cf0` and `0x9d9d2a`.
A successful proof returns Token through another complete transfer at `0x9d9e4b`.
These three transfers add 264 bytes outside the shared wrapper.

The helper also copies Lexer separately:

- `0x9d9bea`: copy the live lexer into private probe storage, 208 bytes.
- `0x9d9e6e`: copy the probe to the successful return slot, 208 bytes.
- `0x9d9e91`: repeat that copy to the same return slot, 208 bytes.

The previous return-value experiment targeted this returned Lexer data.
The proposed shared scan change targets the confirmed Token transfers instead.

## Contiguous scanner

`scanContiguous` has one duffcopy call, at `0x6e704a`.
Its source uses an instruction-relative address, rather than another token on the stack.
It initializes the accepted-token template, which includes `lexerInternalDFALexed: true`.
The scanner then writes token fields directly into its return slot.
Do not classify this template initialization as another returned-token transfer.

## Experiment scope

Add an internal scan entry point that fills caller-owned Token storage.
Pass this storage through both raw scanner branches.
Use it in the proof helper and ordinary next-token path where appropriate.
Keep value-returning wrappers where compatibility requires them.

Clear every output field on every call, including invalid-state and failed scans.
Preserve cursor updates, failed-token positions, read-span recording, provenance, and included-range behavior.
Preserve all DFA transitions, scan order, acceptance rules, and proof budgets.
Check compiler output for removed transfers and pointer escapes before timing.
Run the existing correctness controls and the standard paired performance campaign.

The binary proves these copies exist. It does not prove an achievable speed improvement.
The profile's 33.07% duffcopy share includes other copy sites.
Do not assign that entire share to this candidate.

## Raw evidence

- `/tmp/lexer-scan-baseline.objdump.txt`
- `/tmp/lexer-scan-contiguous-baseline.objdump.txt`
- `/tmp/lexer-proof-baseline.objdump.txt`
- `/tmp/lexer-duffcopy-baseline.objdump.txt`

Reproduce each dump with `go tool objdump -s SYMBOL_REGEX BINARY`.
Use anchored expressions for the three methods and `runtime.duffcopy`.
