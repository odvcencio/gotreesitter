# B16.2 retry economy telemetry

Status: evidence tranche

Use Revision R1 of `spec.campaign.v7` as the authority.

This receipt records which retry rung supplied the selected tree.
It does not change retry routing or admit performance credit.

## Schema

The opt-in `RecoveryRuntimeStats` fields report the retry ladder outcome.

| Field | Meaning |
| --- | --- |
| `RetryAttemptCount` | Retry pass slots consumed by the top-level parse. |
| `RetrySelectedAttempt` | Logical rung that supplied the selected tree. |
| `RetrySelectedAttemptHasError` | Error status of the selected tree. |
| `RetrySelectedAttemptFullSpan` | Full source span status of the selected tree. |

The telemetry keeps a sidecar map from each retry tree to its logical rung.
The map exists only while recovery telemetry is enabled.
The map is cleared when the retry ladder returns.

## Locked Swift witness result

Run the Go and locked-C witnesses with one test worker in Docker.

| Witness | Retry passes | Selected rung | Selected error | Selected full span |
| --- | ---: | --- | --- | --- |
| #586 `FloatingPointToString` | 4 | `initial_merge` | true | true |
| #576 `CollectionAlgorithms` | 0 | `initial` | true | true |
| Swift clean control | 0 | `initial` | false | true |

The #586 parse paid for four retry pass slots.
The final selected tree came from `initial_merge`.
The later widened and final-merge passes did not replace that tree.

The #576 error witness needed no retry pass.
The selected initial tree already covered the complete source.

Both error witnesses preserve their known Swift grammar mismatch.
The clean control keeps exact Go and C tree identity.

## Interpretation

The selected-rung fact identifies retry payment that may be avoidable.
It does not prove that later rungs are redundant across the fleet.

Do not skip a retry rung from this receipt alone.
First collect the selected-rung distribution across the generic recovery cohort.
Then compare correctness, wall time, allocation, and resident set.

Keep the candidate generic.
Do not add a Swift exception, a source hash, or a grammar-name rule.

## Commands and artifacts

Go witness command:

```text
bash cgo_harness/docker/run_parity_in_docker.sh --no-build --repo-root /tmp/gotreesitter-r1-b16-retry --label b16-swift-selected-retry-facts --memory 8g --cpus 1 --gomemlimit 6GiB -- "cd /workspace && go test ./ -run '^TestSwiftRecoveryTelemetryWitnesses$' -count=1 -v -timeout 5m"
```

Go artifact: `harness_out/docker/20260811T233432Z-b16-swift-selected-retry-facts`

Locked-C command:

```text
bash cgo_harness/docker/run_parity_in_docker.sh --no-build --repo-root /tmp/gotreesitter-r1-b16-retry --label b16-swift-selected-retry-cgo --memory 8g --cpus 1 --gomemlimit 6GiB -- "cd /workspace/cgo_harness && /usr/bin/time -v go test . -tags treesitter_c_parity -run '^TestB16SwiftRecoveryTelemetryCOracle$' -count=1 -parallel 1 -timeout 5m -v"
```

Locked-C artifact: `harness_out/docker/20260811T233549Z-b16-swift-selected-retry-cgo`

Both runs passed without an out-of-memory kill or a wall timeout.
