# Native projection through Parser.Parse: integration rejected for throughput

The native projection's earlier component gains do not establish a public API
win. Twenty shuffled pairs through `Parser.Parse` show inconclusive Go and
owned-YAML time, and large-Go allocated bytes rise **10.43%**, failing the
ratchet. This integration stays outside the runtime. The
[direct-replay component receipt](compact-native-direct-replay-receipt-20261014.md)
retains its narrower results; those percentages are not promoted or combined
with these results.

Baseline: `22554e6bafe29422f0e7fed4b10cdb3e10db13ec` (output PR #1484).
Both roles use the same private per-Parser runner and normal public candidate
entrypoint, including acceptance election, token-read-span capture, replay,
stop controls and Tree publication. The candidate adds an optional publication
strategy after any retained acceptance tree has been checked. The strategy
type starts under `internal/parsercorephase0`. No default or allowlist changes.

## Correctness and accounting

Six public-entrypoint witnesses preserve structure, fields, byte/point spans,
root coverage and error status. The four Go trees remain eligible for compact
reuse; both YAML trees remain disabled under the existing scanner gate.
The seventeen accepted intrinsic-metadata witnesses still match, including
legacy read words and compact dependency bounds. Selected records stay
36 bytes and Node stays 104 bytes. The generic-call witness retains its
existing scheduler-cap decline before projection.

| Public witness | Native constructions | Projected constructions |
| --- | ---: | ---: |
| Go `rewrite`, 5,116 bytes | 2,247 | 1,524 |
| Go `query_compile`, 20,168 bytes | 11,326 | 7,524 |
| Go `language`, 41,387 bytes | 10,917 | 7,082 |
| Go `grammargen_lr`, 235,626 bytes | 107,964 | 71,768 |
| Owned YAML R4, 1,998 bytes | 484 | 231 |
| Owned YAML EOF witness, 8,012 bytes | 1,936 | 925 |

An acceptance-hook census finds no retained primary on these six inputs.
Final construction therefore remains reachable, rather than being bypassed by
an already published reference. Existing acceptance comparisons and retained
tree ownership remain intact.

The checked constructor charges live selected backing, selected-build auxiliary
storage, node indexes, line indexes, replay scratch, traversal frames and the
arena to stop-control polling. It clears node references on return and releases
unfinished arenas on error. A forced working-set budget test returns the
existing memory-budget stop with no root or arena returned. These checks are
not full allocator/RSS, pool-retention or lifecycle qualification.

## Twenty paired samples per language

Go and owned YAML run separately, with the repository randomized runner,
alternating pair order, one process per shuffle seed, `GOMAXPROCS=1`,
`-count=1`, `-benchtime=750ms`, and `-benchmem`. All four timing files complete,
their exact source hashes match, and both Docker runs exit zero without OOM or
timeout.

| Public fixture | Before time | After time | Timing | B/op direction | Allocations |
| --- | ---: | ---: | --- | ---: | --- |
| Go 20 KiB | 12.38 ms | 12.50 ms | Inconclusive, p=.947 | +4.66%, p<.001 | Inconclusive, p=.984 |
| Go 235 KiB | 133.9 ms | 132.1 ms | Inconclusive, p=.583 | **+10.43%, p<.001** | +0.07%, p<.001 |
| Owned YAML R4 | 387.2 µs | 385.6 µs | Inconclusive, p=.779 | −0.88%, p<.001 | 111→101, p<.001 |
| Owned YAML 8 KiB | 1.515 ms | 1.511 ms | Inconclusive, p=.968 | +1.59%, p<.001 | 345→343, p<.001 |

Go's public-node reductions remain 33.57% / 33.53%; YAML's remain 52.27% /
52.22%. Those construction reductions do not pay for the extra plan, mapping,
metadata and accounting work in this design. R4 YAML's allocation reduction is
a measured component of an otherwise unqualified strategy, not a throughput
or graduation receipt. No ratchet reset or counter exception is requested.

## Reproduction and limits

Evidence: `/home/draco/.local/state/gotreesitter-evidence/20261014-native-projection-entrypoint`.
It contains overlays and exact source pins, checked-constructor and public API
tests, paired Go/YAML raw samples and benchstat output, runner scripts, and
Docker completion records. The acceptance census and earlier component
evidence remain under `20261014-native-projection-direct-replay`.

Independent C, fleet invariants, full memory, race/lifecycle and general
borrowed-tree qualification are withheld after the failed public Go screen.
Ordinary YAML still requires the ownership probe in these trials; its routing
blocker remains. Existing qualified changes in #1484 and #1485 are separate.
The release hold has no bearing on these failed optimization gates, and no
merge occurred.

The initial directory creation failed because disk space was exhausted.
Only rebuildable test binaries from completed owned experiments were removed;
their sizes/hashes are saved in the direct-replay evidence directory. Sources,
raw samples, profiles and logs remain. Setup failure is excluded from evidence.
