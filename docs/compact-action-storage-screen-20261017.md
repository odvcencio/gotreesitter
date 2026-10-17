# Action-row storage experiments, 2026-10-17

Two singleton-row storage experiments are rejected. Both preserve the checked
parser behavior and deterministic counters, but both slow compact Go. Neither
changes the runtime, routing, allowlists, or graduation status.

The baseline is `a03d0842994a85aa26cf494c8c67404e1e64e999`. Each experiment
starts independently from that same source. Their percentages are separate
comparisons and must not be added.

| Compact fixture | Packed singleton time change | Full inline singleton time change |
| --- | ---: | ---: |
| Go rewrite | +3.84%, p<.001 | +1.60%, p<.001 |
| Go query compiler | +3.53%, p<.001 | +1.73%, p<.001 |
| Go language | +3.31%, p<.001 | +1.39%, p<.001 |
| Go large grammar generator | +3.38%, p<.001 | +0.93%, p=.006 |
| Go R4 | +3.12%, p<.001 | +0.65%, p=.009 |
| YAML owned R4 | +2.62%, p<.001 | +0.49%, p=.414 |
| YAML ordinary attempt plus legacy fallback | +2.89%, p<.001 | +0.31%, p=.301 |
| YAML owned 8 KiB document stream | +4.18%, p<.001 | +0.75%, p<.001 |

Positive values mean slower. The inline YAML R4 changes are inconclusive.
These are candidate-versus-baseline time deltas, not compact/legacy or Go/C
ratios. Ordinary YAML still falls back; the route counters are unchanged.

The packed variant stores canonical singleton actions in eight bytes, with
full-record fallback for actions whose fields cannot be represented exactly.
It enlarges the row header from 32 to 40 bytes and decodes actions into
caller-local storage. The full inline control preserves the complete 20-byte
`Action` in the header and returns its original pointer directly, avoiding
that decoding. Its header is 56 bytes. Both keep the public row handle at
eight bytes, copy caller-owned actions, preserve bounds behavior, and leave
wide action rows on their original representation.

A single cold Go language-table allocation probe is 95,472 to 86,600 bytes for
the packed variant and 95,424 to 104,792 bytes for the inline control. These
are individual diagnostic probes, not statistical memory wins. All warm
benchmark allocation counts are unchanged. Small whole-byte rounding in
reported B/op is retained in the raw samples. The cold packed reduction does
not outweigh its measured warm-parse slowdown. The inline result also rules
out simply removing decode work as a sufficient improvement for this design;
it does not isolate the cost of each layout feature.

Each comparison uses the repository randomized runner, 20 shuffled pairs,
alternating before/after order, 750 ms per benchmark, `-count=1` per process,
`-benchmem`, `GOMAXPROCS=1`, Go 1.25.14, CPU 6, and a 6 GiB Docker container.
Go seeds are 29201–29220 (packed) and 29401–29420 (inline); YAML seeds are
29301–29320 and 29501–29520. Each experiment has 440 timed rows. Default Go
full/edit/no-edit benchmarks are controls; their shifts are unattributed and
are not claimed as improvements. No-edit controls remain at 0 B/op and
0 allocs/op.

Before timing, both candidates pass focused core, phase-zero instrumentation,
race and 386 checks; root scheduler, ownership and table-footprint checks;
Go, YAML and owned-YAML edit-session invariants; locked-C Go/YAML fixtures;
and the YAML EOF/contextual regressions. All 412 standard counter rows and
two owned-YAML rows are exactly equal before and after in both experiments.
No ledger exception is requested. Rejected candidates did not undergo the
full 206-language qualification or an RSS graduation gate.

[Raw evidence](benchmarks/20261017-action-storage-rejections/) includes the
baseline source hashes, candidate patches, randomized samples, benchstat
outputs, audits, scripts, diagnostic shims, and hashed validation-log tails.
The patches have zero context and replay with `git apply --unidiff-zero` on
the pinned baseline. The original absolute overlay files and complete logs
remain in `/home/draco/.local/state/gotreesitter-evidence/20261017-packed-single-actions`
and `/home/draco/.local/state/gotreesitter-evidence/20261017-inline-single-actions`.
Archived diagnostic Go files use `.go.txt` so the evidence adds no Go
packages. Restore their `.go` suffix when reconstructing the original evidence
directory. The source manifest records all 3,506 baseline Go files. The private overlay
binds every baseline path to that frozen snapshot and applies only the
candidate patch plus the two diagnostic shims to the candidate role.

The next target is repeated ownership publication in the existing compact
scheduler. A smaller action record alone has not provided a throughput win.
