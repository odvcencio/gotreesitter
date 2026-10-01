# Fresh parse memory: selected checkpoint change, 2026-10-01

Retain bounded scanner checkpoint chunks from `60ef79513`. Withdraw field-header packing from `69af4dfd7`: it saves TypeScript 1 MiB allocation volume (163,112,600 → 160,556,696 B/op) but produces a new RSS ceiling failure. The paired baseline peaks at 351.041 bytes/input byte; the packed candidate reaches 458.345, above the unchanged 400 limit. Its C# 1 MiB RSS median also rises 6.04%, despite lower B/op.

Eighty additional untraced baseline processes stay below 400. Twenty checkpoint-only processes also stay below 400, peaking at 352.932 bytes/input byte. A traced baseline diagnostic crosses the ceiling on repetition 22; tracing changes timing, so it does not establish an untraced baseline exemption. The [screen receipt](measurements/fresh-memory-selected-20261001/header-screen.json) keeps all high-water marks and distinguishes that diagnostic.

The rollback restores ordinary Go field slices and their original ownership tests. It removes the withdrawn header helper and its unit tests as a complete feature rollback. No parser invariant, memory gate, expected result, census pin, or threshold is removed or changed. The public Node remains 104 bytes and field metadata returns to 48 bytes on amd64.

The selected engine is byte-for-byte identical to the checkpoint-only revision. Its original ten checkpoint-language edit sessions and ledgers passed; the new TypeScript cold/warm locked-C digest preflight and focused metadata/checkpoint tests pass. The helper race/386 checks, root census (756/756), 12 API tag sets, engine/environment guardrails, and CI package-plan guard also retain passing evidence. The full five-language continuation will add independent 20-seed timing and 20-process paired RSS measurements here.

The [VM report](fresh-memory-20261001.md) and [packing comparison](fresh-memory-workstation-20261001.md) describe the historical rejected packing candidate, rather than the selected implementation. Node and arena retention remain with the fresh-speed lane (#1404).
