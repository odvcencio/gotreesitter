# B16.1 Swift recovery witness matrix

Status: complete

Source revision: `61f5f2ba05c79f8145db934a95ba2318ff256389`

Use Revision R1 of `spec.campaign.v7` as the authority.

This receipt covers one-language Swift evidence for issues [#586](https://github.com/odvcencio/gotreesitter/issues/586) and [#576](https://github.com/odvcencio/gotreesitter/issues/576).
It keeps correctness evidence separate from performance evidence.

## Locked inputs

- Grammar repository: `https://github.com/alex-pinkus/tree-sitter-swift`
- Grammar lock commit: `41d6e5fe811ec94229ee71771174a8cce558dfee`
- C runtime: `0.25.1@f5afe475deb7c0bae6407fb776c76824f717bb61`
- C grammar artifact SHA-256: `2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c`
- Go route: production route, with `GTS_ADMISSION_CANDIDATE=0`

## Correctness and telemetry receipt

Run the Go and locked-C witnesses in one Docker container.
The container ran one test process with one test worker.

| Witness | Bytes | Source SHA-256 | Go tree SHA-256 | C tree SHA-256 | Result | Full span |
| --- | ---: | --- | --- | --- | --- | --- |
| #586 FloatingPointToString | 104681 | `ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc` | `ec51c633a3f99515cc0cd1c0cff435a44ddc7db8e83705977d28f78bdfb0fc0e` | `ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d` | Go error / C error | yes |
| #576 CollectionAlgorithms | 24056 | `1aae0051b0bfb50e17c7ac94961ee7cab7332367dcc16e827d2482be7a2dc5a1` | `79cf919059aa656c0d01a9a9f01d658e98e4e13d174de4bbf6560def035cb285` | `132d332f511f12735d80e846f52ec1fddf5f3d0dcd7a097779640a7710497487` | Go error / C error | yes |
| Same-language clean control | 15 | `e7d7fe1c6039c1a9d12e23c3c997369c94daf40c244b22c4b174ffc437ee74f9` | `aaee5a3b2142b81d29e96896bedb9a51fca1c42ca98b83b6c9c6198d20740b0e` | `aaee5a3b2142b81d29e96896bedb9a51fca1c42ca98b83b6c9c6198d20740b0e` | Go clean / C clean | yes |

The two error witnesses preserve the known upstream Swift grammar mismatch.
The clean control has exact Go and C tree identity.

The B16 runtime facts are:

| Witness | Recovery entries | Strategy 1 elections | Cost competitions | Cost walks | Error nodes | Error span | Retry passes | Peak live versions | Memo tier / entries |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| #586 FloatingPointToString | 394 | 1635 | 743866 | 1487732 | 378 | 104680 | 4 | 32 | temporary / 262144 |
| #576 CollectionAlgorithms | 29 | 99 | 6203 | 12406 | 29 | 10977 | 0 | 11 | standard / 16384 |
| Same-language clean control | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | initial / 128 |

The #586 retry reason is `best_retry_result_requires_merge_width`.
Both error witnesses record zero error-mode tokens and zero scanner resynchronizations.
The largest observed reduction-attempt peaks are 11 for #586 and 9 for #576.
The largest observed missing-token trial peaks are 15 for #586 and 11 for #576.

These facts support the generic recovery-cost cohort.
They do not support a Swift scanner change or a Swift-specific parser repair.

## Isolated performance receipt

Run each benchmark in a separate Docker container.
Use one explicit benchmark iteration with `GOMAXPROCS=1`.
Disable the admission candidate route.
Set the recovery timeout to two minutes.

| Witness | `ns/op` | `B/op` | `allocs/op` | Memo bytes/op | Maximum resident set size |
| --- | ---: | ---: | ---: | ---: | ---: |
| #586 FloatingPointToString | 12747857551 | 924653488 | 2826965 | 6291456 | 600640 KiB |
| #576 CollectionAlgorithms | 81483990 | 46425792 | 25626 | 393216 | 584800 KiB |

These are baseline measurements.
They are not a before-and-after comparison and earn no fleet performance credit.
The resident set includes the loaded Swift grammar.

## Commands and artifacts

Correctness command:

```text
bash cgo_harness/docker/run_parity_in_docker.sh --no-build --repo-root /tmp/gotreesitter-r1-b16-witness --label b16-swift-telemetry-cgo-committed -- "cd /workspace/cgo_harness && /usr/bin/time -v go test . -tags treesitter_c_parity -run '^TestB16SwiftRecoveryTelemetryCOracle$' -count=1 -parallel 1 -timeout 5m -v"
```

Artifact: `harness_out/docker/20260811T232146Z-b16-swift-telemetry-cgo-committed`

The Go diagnostic command used the same Docker runner and ran `TestSwiftRecoveryTelemetryWitnesses`.

The #586 performance artifact is `harness_out/docker/20260811T232210Z-b16-swift-586-perf-committed`.
The #576 performance artifact is `harness_out/docker/20260811T232232Z-b16-swift-576-perf-committed`.

All three runs completed without an out-of-memory kill or a wall timeout.

## Boundary and next step

This receipt closes B16.1.
It does not close B16.2 or admit performance credit.

Use the #586 facts to classify the first generic recovery cohort.
Keep the retry, cost-walk, and memo signals separate until a mechanism explains both time and memory.
Run the next candidate against the same Swift witnesses and a non-Swift error control.
