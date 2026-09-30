# Recovery dependency walk receipt

`Tree.Edit` no longer traverses a materialized recovery child twice. The node
helper already checks all missing-leaf padding and lookahead dependencies;
the stack-entry helper now returns that result directly. Pending parents and
lazy child entries keep their original checks. Output and dependency boundaries
are unchanged.

Baseline: `f9828512c`. The regression builds a 64-level materialized recovery
chain and checks both an unaffected prefix and an edit touching the missing
leaf's lookahead. A separate 16-level chain supplies the timing workload.

METRIC: recovery dependency check ns/op | 2541775.5 -> 784.9 | f9828512c -> accompanying fix | 16-level materialized recovery chain, 20 shuffle seeds
METRIC: PowerShell R4 session completion | timeout at 180 s -> 6.53 s | f9828512c -> accompanying fix | 2253-byte Xml.psm1 sample, 72 edit steps

The benchmark uses `scripts/run_randomized_benchmarks.sh`, paired alternating
baseline/head processes, seeds 1 through 20, `GOWORK=off`, `GOMAXPROCS=1`,
`-count=1`, `-benchtime=750ms`, `-benchmem`, and a pinned CPU in Docker.
Both versions allocate zero bytes and zero objects per check. The timer improves
99.97%. Algebraically, the old 16-level check visits 65,536 nodes; the new check
visits 17. This is removal of duplicate work, not an output or receipt change.

The unchanged baseline R4 test times out while alternating between
`nodeEndsBeforeEditDependency` and `stackEntryEndsBeforeEditDependency` inside
`Tree.Edit`. The fixed R4 session passes incremental/fresh equality, coverage,
ERROR/HasError consistency, and zero-allocation no-edit reparses. Docker checks
also pass for missing-node dependency ownership, edit boundaries, copied trees,
lazy entries, compact dependency publication, borrowed materialization, and
isolated race coverage. Additional R4 sessions pass for Properties, C#, Go,
TypeScript, and Julia, each in its own process.

No pins, expectations, thresholds, node layout, or parser layout change.
