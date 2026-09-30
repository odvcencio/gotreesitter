# PowerShell EOF reuse receipt

EOF appends can borrow authenticated function prefixes on the compact route.
The stateless statement-terminator scanner permits recording zero-width read
frontiers. Replay must resume in a row with no valid external symbols, which
keeps the lexer's zero-width loop history outside the borrowed boundary.
Exact LR states, source bytes, lookahead, column, ownership, and projection
checks remain mandatory. Recovery, missing proofs, and small inputs retain
fresh fallback. No language-name comparison was added to engine code.

Receipt publication searches at most sixteen levels and offers at most five
deep function reductions. Bounded scratch and small receipt maps retain their
storage, clear every node and arena reference, and charge retained capacity to
the memory budget. Fresh parses discard the runner caches. Large maps are
discarded; small-map retention requires a new explicit opt-in each parse.
The EOF preflight also checks retained borrowed arenas, because useful receipts
can outlive the arena containing the latest root.

## Fixed workload and counters

Baseline: `f9828512c`. The implementation follows `2987c2745`. The workload
is 200 consecutive single-character `x` appends to the unmodified 15,220-byte
`test/SSHRemoting/SSHRemoting.Basic.Tests.ps1` from PowerShell revision
`c23c421068dd4967765d8d658d4ba68eb649ef4f`. Source SHA-256:
`bfa44284783f38e1508efd6a1141b9948fdb4fd9c8367b3d26278e15e0d7384f`.
Both engines time tree edit, incremental parse, and release of the prior tree.
Parser setup and initial load are outside that timer. Full parse is measured
separately. C uses locked runtime `6070dbfe`, introduced by pin `3a0e7df5`.

METRIC: PowerShell EOF session tokens | 423400 -> 214593 | f9828512c -> accompanying change | SSHRemoting.Basic.Tests.ps1, 200 x appends, compact initial tree
METRIC: PowerShell EOF session new nodes | 1662600 -> 853507 | f9828512c -> accompanying change | SSHRemoting.Basic.Tests.ps1, 200 x appends, compact initial tree
METRIC: PowerShell EOF session reused bytes | 0 -> 1409025 | f9828512c -> accompanying change | SSHRemoting.Basic.Tests.ps1, 200 x appends, compact initial tree
METRIC: PowerShell EOF session complete-operation Go/C | 789.3099x -> 694.5400x | f9828512c -> 2987c2745 plus timing snapshot b7005e5938508bf1e7faa3ff4e89ce47c70e1d313896e54145d8ac452d2e895f | SSHRemoting.Basic.Tests.ps1, 200 x appends, paired 20 seeds

The first compact edit consumes 1,265 tokens and builds 4,971 nodes while
reusing 5,876 bytes. Later edits consume 1,072 tokens and build 4,264 nodes while
reusing 7,051 bytes. Starting from a legacy tree first takes the existing fresh
route: its complete session consumes 215,638 tokens, builds 857,556 nodes, and
reuses 1,401,974 bytes. No-edit reparses allocate nothing.

## Randomized timing and tradeoffs

All comparisons use `scripts/run_randomized_benchmarks.sh`, twenty paired,
alternating seeds, `GOWORK=off`, `GOMAXPROCS=1`, one process per seed,
`-count=1`, `-benchtime=750ms`, `-benchmem`, and a pinned CPU in Docker.
Each process shuffles paired Go-C-C-Go groups. The ratio is the median of
per-seed group-average Go/C ratios. Per-key p99 includes edit, parse, and release.

| Measurement | Before | After |
| --- | ---: | ---: |
| Complete 200-key session, Go0 | 7,848,152,979 ns | 7,014,030,750 ns |
| Complete 200-key session, Go3 | 7,858,537,720 ns | 7,042,751,932 ns |
| Session Go0, B/op | 357,948,920 | 520,840 |
| Session Go3, B/op | 360,893,000 | 9,354,416 |
| Session Go0, allocs/op | 4,057.5 | 4,254.5 |
| Session Go3, allocs/op | 4,065 | 4,297 |
| Go0 p99 per keystroke | 52.155 ms | 40.983 ms |
| Go3 p99 per keystroke | 53.989 ms | 41.274 ms |
| Full parse | 37.000 ms | 40.190 ms |
| Full parse, B/op | 816 | 849 |
| Full parse, allocs/op | 15 | 15 |
| Cold session maximum RSS | 55,988 KiB | 40,536 KiB |

Session time falls 10.63%/10.38% (p=0.040/0.020). The p99 medians fall,
without a statistically significant change. C controls have no significant
change. The full-parse receipt costs 8.62% more time and 4.04% more bytes;
session allocations rise 4.84%/5.71%. These directional regressions stay below
the 10% ratchets. The additional initial-load cost is about 3.19 ms against
roughly 0.82 seconds saved in the complete typing session. Allocated session
bytes fall over 97%, and RSS falls 27.6%. The remaining Go/C ratio still fails
the existing 10x floor; this change does not graduate PowerShell.

VM load varied substantially. An earlier complete twenty-seed run measured
736.06x to 637.32x. Those numbers are separate receipts, not mixed with the
final run. The final timing snapshot precedes the borrowed-arena preflight
extension; that extension runs only when the primary arena lacks a prefix
receipt. The real-session primary path and deterministic counters are checked
again. No timing claim is made for the generated fixture's newly admitted steps.
On that same generated 200-key session, extending the preflight from the
primary arena to retained borrowed arenas reduces tokens from 266,900 to
43,600 and new nodes from 950,600 to 149,800. Reused bytes rise from 681,000
to 1,362,000. Both initial routes match fresh Go and locked C at every step;
after initial route migration, every step must reuse a prefix. The real-session
totals remain exactly 214,593 tokens, 853,507 nodes, and 1,409,025 reused bytes.

Go controls at the final field layout show no significant timing change:
full 17.29 to 17.72 ms, edit 358.9 to 355.7 us, and no-edit 17.28 to 15.90 ns.
Allocation counts remain 8/5/0. An earlier layout produced a significant 7.51%
Go edit slowdown; moving the new cache fields after existing hot fields removed
that result in the repeated twenty-seed control run.

Rejected candidates: broad publication raised full-parse allocations from
15 to 38; recursive publication with six deep offers raised them from 15 to
28; five offers without retained buffers still raised them to 17. Retaining
only projection buffers left session allocations about 44% higher. The final
bounded ownership-walk buffers remove that regression. A latency run containing
C reference-library construction inside setup was discarded and restarted
with both engines' setup outside the timer.

## Validation and remaining work

Docker checks pass for compact dependency, borrowed ownership, nested replay,
arena reset, parser layout, memory-budget, missing dependency, scanner
certification, and existing tiny-file EOF fallback suites. Isolated race checks
pass. Seventy-two-step R4 sessions pass for PowerShell, Go, C#, Properties,
TypeScript, Julia, Mojo, and Starlark, one language per process. The real and
generated typing fixtures compare every step against a separate fresh Go parser
and the locked C deep digest, check coverage and ERROR/HasError consistency,
and require zero-allocation no-edit reparses. A generated fixture keeps these
checks runnable without the external corpus.

The overseer receipt directory was absent, so the baseline was regenerated.
The exact keystrokes were unspecified; this receipt fixes them as `x` appends.
No threshold, pin, expectation, graduation allowlist, or corpus lock changes.
The 206-language census and the owner's separate 999-byte witness were not
rerun. Bash and Blade have replay certification but still need contextual
reuse proofs. PowerShell's remaining materialization, ownership traversal, and
full-parse receipt cost need further work.
