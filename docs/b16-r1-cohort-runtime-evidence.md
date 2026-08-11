# B16.3 generic recovery cohort evidence

## Purpose

Attach runtime facts to the existing real-corpus scan.
Use these facts to test the C0f recovery hypothesis across a generic cohort.
Do not change parser routing or grant performance credit.

## Activation

Set `GTS_PERF_SCAN_RUNTIME_EVIDENCE=1` for the scan process.
The setting remains off by default.
The scan records `config.runtime_evidence=true` when the setting is active.

The scan enables recovery telemetry and arena accounting only for this lane.
The timed full-parse samples do not capture or serialize these facts.
The retained classification parse captures them after parsing completes.

## Receipt shape

Each retained full-parse classification can contain `classification.runtime`.
The record has three sections:

- `recovery`: recovery entries, strategy-one elections, cost competitions,
  cost walks, error nodes, retry attempts, selected retry rung, scanner
  resynchronization, and live-version counts.
- `parse`: parser-loop timing, allocation counters, materialization timing,
  final-node count, memory growth, and the configured memory budget.
- `arena`: node capacity, live-node, capacity-waste, and selected byte counts.

The record preserves zero values. A zero value means that the instrument saw
no event. An absent section means that its instrument did not run.

## Cohort interpretation

Classify a row from direct runtime facts, not from its file path or language.
Use the following evidence order:

1. Verify the authenticated corpus lock and source digest.
2. Compare the Go and C deep-tree identities.
3. Check the Go parse class and full-span status.
4. Check recovery entry, error-node, retry, memo, and materialization facts.
5. Compare wall time, allocation, and resident-set receipts.

The C0f error share identifies a large non-clean timing cohort.
It does not prove that recovery caused every row in that cohort.
Credit a recovery mechanism only when the runtime facts and locked-C parity
support the same generic class.

## Next gate

Collect the selected-rung distribution across the generic recovery cohort.
Include Swift issues #586 and #576 as outside-validated witnesses.
Include clean controls from the same languages when available.
Require exact locked-C tree parity for every admitted witness.
Reject language-name, source-hash, driver-local, and new blob-SHA exceptions.

Do not use this lane to select a retry skip or a recovery fast path.
Make that decision only after the cohort receipt closes and Phase 2 gates pass.
