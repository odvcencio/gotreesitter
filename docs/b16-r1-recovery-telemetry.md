# B16.0 recovery telemetry

Status: implementation tranche

This tranche records missing recovery facts for one completed parse attempt.
It does not change parser routing, grammar admission, or recovery decisions.

Enable the facts with `gotreesitter.EnableRecoveryRuntimeTelemetry(true)`.
Disable the facts after the diagnostic run.

The default path does not allocate the recovery telemetry sidecar.
The default path does not write the new counters.

Read the latest facts with `Parser.DebugRecoveryRuntimeStats()`.
The result describes the most recent completed parse attempt on that parser.

| Field | Unit | Reset and meaning |
| --- | --- | --- |
| `RecoveryEntryCount` | calls | Count `cHandleError` entries in this attempt. |
| `Strategy1ElectionCount` | calls | Count strategy-1 recovery elections. |
| `RecoveryCostCompetitionCount` | calls | Count recovery cost comparisons after header checks. |
| `RecoveryCostWalkCount` | calls | Count non-clean recovery cost walks. |
| `RecoveryCostWalkNanos` | nanoseconds | Sum the measured recovery cost walk time. |
| `ErrorNodeCount` | nodes | Count final error and missing nodes. |
| `ErrorSpanBytes` | bytes | Report the envelope from the first error start to the last error end. |
| `RetryPassCount` | passes | Report the parser retry count when this attempt began. |
| `RetryReason` | stable text | Report the reason for the retry that started this attempt. |
| `ErrorModeTokenCount` | tokens | Count engine-generated error-mode lookaheads. |
| `ScannerResyncCount` | events | Count custom token-source resynchronizations. |
| `LiveVersionCount` | versions | Report live parser versions at finalization. |
| `PeakLiveVersionCount` | versions | Report the largest live version set seen during recovery. |

Counters reset at the start of `parseInternal`.
Counters increase monotonically during one attempt.
The implementation does not saturate counters before `uint64` overflow.

Use existing runtime fields for stack iterations, merges, materialization,
allocation, stop reasons, and memory-budget sources.
Use `Tree.RecoveryNodeMemoRuntime()` for memo tier and collision facts.

The focused witness covers a KDL recovery input and a Go clean control.
The test checks the recovery entry, error shape, and live-version facts.
