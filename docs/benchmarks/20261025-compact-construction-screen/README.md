# Compact construction screen

**Keep the runtime held.** Direct leaf/unary construction, reuse of normalized
field/alias children and cached pool accounting remove repeated work, but full
Go parsing still regresses against the current runtime. No engine, routing,
ledger, dependency or graduation change is included. Main merges remain held.

The candidate builds on the [immutable raw-output screen](../20261025-compact-raw-screen/README.md).
It specializes already validated leaves and unary forwarding, uses each newly
normalized exceptional child list for its replay proof, and replaces repeated
pool censuses with an atomic charge updated at ownership changes. This is not
full raw-payload/summary co-formation. Both real Go sources still require an
actual immutable lease before traversal; no selected-store substitution passes.

## Complete performance comparison

Sixty matched pairs / 120 processes use the unchanged randomized runner,
20 seeds per suite, alternating arm order, identical seeded workload order,
750 ms, count 1, GOMAXPROCS 1 and benchmem. Both arms use the same wrapper.
Linux amd64, Intel Core Ultra 9 285, Go 1.26.6; Docker has one pinned CPU,
4 GiB and no network. The baseline is `ada64d82171b2c36e95868c03b378b741f45abb7`;
only the candidate receives the frozen private runtime overlay.

| Workload | Before | After | Time change |
| --- | ---: | ---: | ---: |
| Generated Go full | 7.1059 ms | 7.6503 ms | +7.66%, p<.001 |
| Generated Go edit | 67.535 µs | 64.615 µs | −4.32%, p<.001 |
| Generated Go no edit | 3.0935 ns | 2.9275 ns | −5.37%, p<.001 |
| Ordinary Go compact | 764.233 µs | 831.927 µs | +8.86%, p<.001 |
| Ordinary Go legacy control | 638.896 µs | 633.418 µs | −0.86%, p<.001 |
| Ordinary YAML compact control | 192.977 µs | 192.775 µs | inconclusive, p=.565 |
| Ordinary YAML legacy control | 136.588 µs | 133.715 µs | −2.10%, p<.001 |

Generated full bytes fall **261,543→139,397 B/op** and allocations **22→16**.
Ordinary Go bytes rise **1,112→1,784 B/op**, while allocations fall **14→10**.
Edit bytes are 407→406 with five allocations; no-edit remains zero bytes and
allocations. YAML allocation metrics match. YAML does not use the new raw lease
in this screen, so its controls establish no YAML ownership or throughput win.
The legacy controls also move; the complete raw pairs and route ratios remain
in the receipt. This is a comparison against the current runtime, not a matched
comparison against the earlier held prototype.

## Correctness and actual work

Before timing, **83 Core families / 173 records** pass. The 19 raw-summary
families / 34 records pass in work-count and race builds. Tests include the
exact previously qualified capture implementation as a test-only reference:
summary records, replay certificates and forests match on unary, field, alias
and hidden-prefix cases. Pool tests independently census full capacities,
rejected returns, retirement, outstanding leases and concurrent readers.

Both original Go sources match exact C trees against repository C 0.25.1 and
private C 0.27.1 (`10eb5ac119eec9cb4267a8e035066473983f5f3a`). Public API,
copy/reset/reuse, capacity, source-authority, error/recovery, race and memory
checks pass. The original Cognociente fixture and all 96 assignment edit cases
remain C-exact. Four invariant sessions pass **288 edits**, with zero no-edit
allocations. No fresh-verifier bypass is included.

All four actual Go/YAML counter rows are safe: three are identical; candidate
Go full `new_nodes` falls **892→1**. Every other field matches. No owner-approved
exception is consumed by this experiment.

The unchanged generated fixtures have formation edges **51*n+8→41*n+6**, local
root references **18*n+4→9*n+2**, and hidden formation steps **2*n→n** at
n=32/128/512/1024. Ordinary Go formation edges fall **2,506→1,936**, local root
references **1,025→528**, and hidden steps **148→75**. Closure counts, public tree
digests, retained tree bytes and all subsequent read work match. Repeated full
walks add no cache or plan work. These deterministic reductions do not override
the observed full-parse slowdown.

Full-fleet, large-file RSS, consumer timing, ARM/Windows and authenticated E-A/M1
gates remain unqualified. YAML checkpoint transfer is a separate experiment.
The next construction seam is raw-parent/summary co-formation; closure checks
must still precede every nonempty gap callback. Another multi-token wrapper
alone does not remove the mandatory election work already inside the corridor.

## Evidence

The [delta archive](evidence.tar.gz) adds 165 members to the immutable
[base archive](../20261025-compact-raw-screen/evidence.tar.gz), whose SHA is pinned
in [summary.json](summary.json). The union retains all source overlays, exact
commands, source/control manifests, gate logs, four actual ledgers, work records,
120 benchmark processes and benchstat output. No binaries or credentials are
included. [verification.json](verification.json) recomputes raw sample medians,
seed coverage/order, route/fallback assertions, counter directions, C digests
and work reductions. It does not execute Go or C:

```sh
python3 docs/benchmarks/20261025-compact-construction-screen/validate-evidence.py --source-root /home/draco/work/gotreesitter
```
