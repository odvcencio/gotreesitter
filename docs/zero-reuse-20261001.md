# Zero-reuse receipt, 2026-10-01

The pinned first edit now has zero subtree reuse in **80/206** grammars,
down from **83/206** at `9148a96db` after #1402. The historical 81/206 count
used a different revision. This comparison uses the same committed samples
and the first insertion from edit-session seed 4242 on both revisions.

| Grammar | Reused bytes | Parsed tokens | New nodes | Complete Go edit, median | Median paired Go/C |
| --- | ---: | ---: | ---: | ---: | ---: |
| Lua | 0 → 2,025 | 385 → 0 | 956 → 0 | 3.776 ms → 0.809 ms | 15.21× → 3.48× |
| Nickel | 0 → 2,088 | 296 → 0 | 982 → 0 | 3.234 ms → 0.758 ms | 37.18× → 8.36× |
| Starlark | 0 → 2,049 | 199 → 0 | 456 → 0 | 2.293 ms → 0.891 ms | 7.33× → 2.96× |

Each timing uses 20 independent shuffle seeds, `GOMAXPROCS=1`, `-count=1`,
`-benchtime=750ms`, and `-benchmem` through `run_randomized_benchmarks.sh`.
Revisions alternate first position. Complete edits include `Tree.Edit`,
incremental parsing, and release; each seed measures Go-C-C-Go. The C oracle
uses runtime `f5afe475deb7c0bae6407fb776c76824f717bb61` (0.25.1), binding
v0.25.0, and the unchanged grammar lock.

The VM is busy. The after-edit Go/C ranges across seeds are 2.26–5.08× for
Lua, 6.14–10.65× for Nickel, and 2.48–3.33× for Starlark. Full-parse timing
changes are below 5%; they are not evidence of a full-parse improvement.
Full-parse allocation rises 32 B/op: 2.55%, 2.00%, and 1.63%, respectively.
The read-bound anchor adds private tree metadata and prevents an insertion
and its inverse from repeatedly increasing the proof bound. Complete-edit
allocation falls from 1,570 to 712 B/op, 2,352 to 1,160 B/op, and 2,168 to
656 B/op. All measured no-edit reparses allocate zero bytes and objects.

The new mechanism authenticates run-length edits inside an existing terminal.
A scanner-owned ASCII class proves that scanner outcomes and payloads stay
the same after coordinate projection. The existing bounded lexer proof still
compares raw token decisions and keyword selection. Admission requires a clean
accepted parse, one frontier, a non-fragile terminal, an interior ASCII edit
on one line, and source-compatible result rules. Unknown cases use fresh
parsing. Ordinary stateful scanner reuse stays disabled. No language-name
comparisons, graduation changes, gate thresholds, or existing pins change.

Correctness passed in Docker, one grammar per process: exact locked C trees
for initial and edited samples; 96 insertion/inverse pairs; malformed edits;
generated files at 32 KiB, 137 KiB, and 1 MiB; and 72-step fresh-Go invariant
sessions for each fixed grammar. Exhaustive valid-symbol masks cover scanner
payload witnesses. Oversized tokens exhaust the unchanged proof budget and
fall back; Starlark string content remains a nonterminal fallback. All 412
counter-ledger rows pass the unchanged 2% gate.

Nickel's baseline edit session stalled at step 42 in `Tree.Edit`. A dependency
predicate visited the same error descendants twice per level. Removing the
duplicate traversal preserves the predicate and lets all 72 steps finish in
0.76 seconds. The depth-16 fixture requires 131,071 node-predicate visits
before and 17 after. Padding, lookahead, lazy-child, and nested missing-token
regressions pass.

Rejected candidates remain outside the implementation:

- Checkpoint/prefix reuse slowed JSONnet complete edits 19.8% and Properties
  85.6% in five-seed diagnostic runs. ReScript also had an initial locked C
  field mismatch.
- R's fixed-width raw-string closer violates the proposed unconditional
  scanner certificate. Its counterexample is retained as a test.
- Gitcommit's stateless opt-in increased candidate-route tokens 155 → 300
  and nodes 407 → 792, failing the unchanged ledger. The opt-in was reverted.

The baseline zero set groups into 66 scanners without checkpoints, four
checkpointed scanner opt-outs, eight unproven recovery frontiers, one forest
fallback, one explicit language opt-out, and three grammar/fixture cases.
No pinned first-edit zero row stops because of a budget. Stateful scanner
certification remains the largest unfinished group. Broader structural edits
and graduation remain separate work.

METRIC: zero-reuse rows | 83/206 -> 80/206 | 9148a96db..sol61/zero-reuse | pinned first edit, seed 4242

METRIC: reused bytes Lua/Nickel/Starlark | 0/0/0 -> 2025/2088/2049 | 9148a96db..sol61/zero-reuse | pinned first edit, seed 4242

Detailed samples and machine-readable summaries are in
[the receipt directory](receipts/zero-reuse-20261001/summary.json).
