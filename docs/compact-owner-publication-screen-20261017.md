# Ownership publication screen, 2026-10-17

The authenticated no-op guard is rejected. It preserves the focused correctness
checks and every deterministic counter, but slows compact Go and gives no
supported YAML improvement. No runtime, routing, allowlist, or graduation
change is promoted.

The candidate checks the live head owner after authenticating the scheduler
token. An identical owner returns before entering the mutation wrapper. The
original validation, error poisoning, journaling and panic recovery remain on
the slow path. That extra probe is not a throughput win on these fixtures.

| Compact fixture | Time change | p |
| --- | ---: | ---: |
| Go R4 | +1.22% | <.001 |
| Go rewrite | +0.78% | <.001 |
| Go query compiler | +0.80% | .006 |
| Go language | +0.34% | .018 |
| Go large grammar generator | +0.23% | .108 |
| YAML owned 8 KiB document stream | +0.001% | .968 |
| YAML ordinary attempt plus fallback | +0.04% | .862 |
| YAML owned R4 | +0.33% | .461 |

Positive values mean slower. The large Go and all YAML changes are
inconclusive. The independent baseline is
`a03d0842994a85aa26cf494c8c67404e1e64e999`; these deltas are not compact/legacy
or Go/C ratios. All warm allocation counts are unchanged. Default Go controls
move full −0.02% (p=.989), edit +0.29% (p=.026), and no-edit +2.73% (p<.001).
Those control shifts are unattributed. No-edit stays at 0 B/op and 0 allocs/op.

The repository randomized runner completes 20 alternating pairs for each
language: Go seeds 29601–29620 and YAML seeds 29701–29720, 750 ms per benchmark,
`-count=1` per process, `-benchmem`, `GOMAXPROCS=1`, Go 1.25.14, CPU 6, and a
6 GiB Docker container. All 440 timed rows, source pins, work/reuse/route
metrics and median hard gates pass the audit. Before timing, core normal,
phase-zero instrumentation, race and 386 checks pass, along with focused root,
Go/YAML/owned-YAML edit-session invariants, locked-C fixtures, and YAML
EOF/contextual regressions. All 412 standard and two owned-YAML counter rows
are exactly equal. The rejected candidate receives no full-fleet or RSS
qualification.

Fresh CPU profiles use the unchanged baseline after timing. On the Go
language fixture, reduction construction takes 19.91% cumulative sampled CPU
and copying takes 4.26% flat. On the owned YAML stream, reduction construction
takes 19.95% cumulative, while the sparse external-provenance binary search
takes 3.37% flat. These are attribution samples, not before/after evidence;
cumulative percentages overlap and must not be added. They motivate testing
a bounded position hint that rechecks the current payload key and reads the
live provenance record, without caching an ownership or checkpoint proof.

[Evidence](benchmarks/20261017-owner-publication-screen/) retains the samples,
benchstat outputs, source patch, scripts, source/counter audit, log hashes,
and profile reports. The shared baseline source manifest and archived
`.go.txt` diagnostic shims are referenced in `replay.json`; restore their
`.go` suffix when reconstructing the original evidence directory. The
zero-context patch replays with `git apply --unidiff-zero`. Complete logs,
profile binaries, CPU samples and the absolute overlay remain in
`/home/draco/.local/state/gotreesitter-evidence/20261017-owner-publication-noop`.
