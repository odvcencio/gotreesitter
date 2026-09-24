# Issue 1061 recovered-root election receipt

## Scope

This change elects material paths inside one winning recovery group.
It retains a group's identity through grammar forks and clears it after acceptance.
It leaves the production parser unchanged.

## Locked oracle

The PHP source is `<?php namespace ; ?>`.
Its SHA-256 value is `669f317dd185ce38c907f47d7ef88de5c81d13f2f1db6de7649b672646095ec4`.
The locked C runtime is `f5afe475deb7c0bae6407fb776c76824f717bb61`.
The current PHP grammar lock is `3fda2fb9577166c6399834917f9844f30370beea`.
The issue names the earlier grammar commit `3f2465c217d0a966d41e584b42d75522f2a3149e`.
That earlier grammar artifact has SHA-256 value `21c389f42e95270b956df8906784320174ef1e72afca08f525ef23c2023a3ee9`.
The current grammar artifact has SHA-256 value `1daea60ac1ee31227b8e1ed3cbd76b841435fe693e95af65cc61dad447d27891`.

Locked C returns `(program (php_tag) (ERROR) (empty_statement) (text_interpolation (php_end_tag)))`.
The root spans bytes 0 through 20, has an error, and has four children.
The deep digest is `ccda45c83ece81066dc557d0742945c929b17508e67b8884a64583e4958d09fd`.
The direct cost fixture selects the 609-point absorber over the 610-point missing insertion.

| Control | Production | Compact request | Locked C comparison |
| --- | --- | --- | --- |
| PHP recovery | Exact | Explicit fallback | Returned tree exact |
| Scala `(y; ` recovery | Exact | Routed | Exact |
| Meson `message('hello')` clean parse | Exact | Routed | Exact |

The PHP compact request falls back before acceptance.
The guarded synthetic end-of-file reduction sees two runnable recovery heads.
An exploratory run admitted both heads and returned a different tree.
That run placed `namespace` in a declaration and placed the error on `;`.
Its recovery costs were 601 and 610, rather than the issue's 609 and 610.
The branch retains the fail-closed guard until the earlier recovery path matches C.

## Direct gates

| Gate | Result |
| --- | --- |
| Two recovered paths with different costs | Lower cost wins. |
| Equal costs with different precedence | Higher precedence wins. |
| Equal positive costs and precedence | Later path wins. |
| Six merged absorber paths | One election selects the last path. |
| Different error regions under equivalent heads | Five distinct regions remain available. |
| One recovery group with grammar fork arms | All arms reach one election. |
| Two recovery groups with fork arms | Groups compare once; winning paths remain. |
| Clean ordinary ambiguity | Recovery election count stays zero. |
| Invalid source, cap, or group cost | The frontier remains unchanged. |
| Finished election | Losing headers and recovery authority clear. |

The direct acceptance receipt records one `RecoveryRootSelections` event.
The two-group receipt records one `RecoveryLineageSelections` event.
The six-path fixture records five physical merges.

## Docker checks

All heavy checks ran under `/home/draco/.local/state/nightwatch/gts-docker.lock`.

| Check | Result | Maximum resident set size |
| --- | --- | ---: |
| Focused tagged root tests | Pass | Not measured separately |
| Focused core recovery tests | Pass | Not measured separately |
| PHP locked-C witness | Pass with explicit fallback | 1,098,492 KiB |
| Scala locked-C recovery control | Pass with compact route | 256,144 KiB |
| Meson locked-C clean control | Pass with compact route | 256,692 KiB |
| Isolated PHP grammar corpus | 6/25 deep matches; suite reports mismatch | 1,281,964 KiB |

The PHP corpus result measures generated grammar parity and does not certify compact recovery.
The focused PHP witness verifies the current locked C identity, tree, ranges, flags, order, and digest.
The isolated PHP diagnostic log has SHA-256 value `0ed65e6c5a867e7507586eddfbe126f4ccf52863cdbe7d03cb1863033ff8b3cd`.

Reproduce the isolated oracle controls with these commands:

```sh
flock /home/draco/.local/state/nightwatch/gts-docker.lock bash cgo_harness/docker/run_parity_in_docker.sh --run '^TestPackage3PHPRecoveredRootLockedC$'
flock /home/draco/.local/state/nightwatch/gts-docker.lock bash cgo_harness/docker/run_parity_in_docker.sh --run '^TestPackage2ScalaFalsifierPhysicalMergeMinimal$'
flock /home/draco/.local/state/nightwatch/gts-docker.lock bash cgo_harness/docker/run_parity_in_docker.sh --run '^TestMesonAcceptanceElectionLockedCParity/command$'
flock /home/draco/.local/state/nightwatch/gts-docker.lock bash cgo_harness/docker/run_single_grammar_parity.sh php
```

## Performance

The repository's randomized benchmark script ran 20 shuffled seeds before and after the change.
Each process used one processor, one count, a 750 ms benchmark time, and memory reporting.

| Benchmark | Before time | After time | Before and after bytes | Allocations |
| --- | ---: | ---: | ---: | ---: |
| Full Go parse | 4.792 ms | 4.564 ms | About 1.229 KiB per operation | 8 |
| Go single byte edit | 119.7 µs | 114.7 µs | 386 bytes per operation | 5 |
| Go no-edit parse | 4.071 ns | 4.087 ns | 0 bytes per operation | 0 |

`benchstat` reports a 4.76% lower full-parse time and a 4.13% lower edit time.
The no-edit time has no significant change.
The recovery-only scope does not support attributing these timing differences to this change.

The before log has SHA-256 value `2219457ca0a9bd2b4da3e44a14dca24a37a8ab5caeec87bbdcff718208f78252`.
The after log has SHA-256 value `e510f0d67f8be452fd56c16d3212d45d55ed7224366b4b152743eb5783d0894d`.
Both logs contain 60 benchmark rows and end with `status: complete`.

The 512 KiB Go full-parse probe parsed 524,308 bytes and published 299,011 nodes.
It accepted without an error, crash, timeout, or out-of-memory failure.
Its maximum resident set size was 111,092 KiB.

Reproduce the performance and memory runs with these commands:

```sh
GOWORK=off bash scripts/run_randomized_benchmarks.sh --output /tmp/gts-1061-before.txt --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$' --require-benchmarks BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
GOWORK=off bash scripts/run_randomized_benchmarks.sh --output /tmp/gts-1061-after.txt --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$' --require-benchmarks BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
flock /home/draco/.local/state/nightwatch/gts-docker.lock bash cgo_harness/docker/run_parity_in_docker.sh -- "cd /workspace && GOWORK=off go build -o /tmp/gts-1061-large-file ./cmd/issue454bench && GOMAXPROCS=1 /usr/bin/time -v /tmp/gts-1061-large-file go 512 full 1"
```
