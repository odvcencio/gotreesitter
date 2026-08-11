# B16/R1 recovery evidence brief

Status: design brief

This brief starts the first performance family after the C0f attribution receipt.
It does not change parser routing, grammar admission, or recovery behavior.

## Authority and evidence

Use Revision R1 of `spec.campaign.v7` as the campaign authority.

Use V10 epoch `20260808T202958Z-v10-full-5003ffba` as the fleet baseline.

The baseline contains 1,435 rows across 206 languages.
The measured signal contains 1,315 rows.

The C0f receipt reports these recovery-priority facts:

- Error rows own 72.2586 percent of measured Go time.
- The error cohort has a 92.6307 percent local Go gap.
- The measured Go-to-C ratio is 10.950130x by total time.
- The C0f receipt remains partially gated because several runtime facts are absent.

The 72.2586 percent share sets priority.
It does not provide performance credit.

The fleet credit gate remains:

```text
projected_saved_go_time / total_measured_signal_go_time >= 0.02
```

Select cohorts with runtime facts.
Do not select cohorts by language or file identity.

## Outside-validated witnesses

Use issue [#586](https://github.com/odvcencio/gotreesitter/issues/586) as a recovery-cost witness.
It records a Swift failure with high retry and recovery pressure.

PR #666 already provides a useful generic result on that witness:

- Time improved by 9.96 percent after the recovery-stack change.
- Allocated bytes improved by 59.65 percent.
- Mean resident set size improved by 22.09 percent.
- Generic recovery cost remains active.

Treat those values as historical evidence.
Reproduce them on the current branch before using them as a B16 receipt.

Use issue [#576](https://github.com/odvcencio/gotreesitter/issues/576) as a recovery-parity witness set.
Its umbrella includes `unsafe`, `__consuming`, `associatedtype`, optional binding, and comparison cases.

Do not add a Swift-specific parser fix.
Record each grammar or oracle divergence against the generic recovery path.

## Work order

### B16.0 — instrument the runtime facts

Add diagnostic counters without changing parser decisions.
Keep the counters off the public hot path when diagnostics are disabled.

Record these facts per completed parse and per retry attempt:

- Recovery entry count.
- Recovery cost competition count.
- Recovery cost walk count and elapsed time.
- Reduction candidate attempts and peak attempts.
- Missing-token trial attempts and peak attempts.
- Error-node count and error-byte span.
- Recovery retry count and retry reason.
- Retry stack ceiling and selected attempt.
- Scanner error-mode entries and resynchronization events.
- Recovery-node memo tier, entries, bytes, and collisions.
- Live version count and peak live version count.
- GLR stack iterations and merge counts.
- Transient parent and child materialization counts and time.
- Final tree materialization time.
- Arena, scratch, entry-scratch, and GSS allocation bytes.
- Stop reason and memory-budget source.

Use existing runtime fields when they provide the same fact.
Do not create duplicate counters with different meanings.

Publish the counter schema with units, reset scope, and overflow behavior.
Keep counters monotonic within one parse attempt.
Reset them at the top-level parse boundary.

### B16.1 — build the witness matrix

Run one language at a time in Docker.
Start with the Swift #586 witness and one representative error-cohort file.
Add one clean control from the same language when available.

For every witness, capture:

- Go and locked-C tree identity.
- Source digest and grammar digest.
- Parse class and full-span status.
- All B16 runtime facts.
- Wall time, allocations, and maximum resident set size.
- The exact command, revision, image, and environment.

Keep correctness and performance receipts separate.
Do not run a fleet scan for this tranche.

### B16.2 — classify the first generic cohort

Use observed runtime facts to assign each witness to one or more diagnostic cohorts.
Use these tentative cohort names only when the data supports them:

- Retry economy: repeated accepted-error or no-stack retry work dominates.
- Recovery cost: error-cost walks or candidate competition dominate.
- Materialization lifetime: recovery memo or transient tree work dominates.
- Scanner boundary: error-mode lexing or scanner resynchronization dominates.
- Stop overhead: a generic stop signature dominates without a correctness failure.

Do not merge cohorts because they share a language or file extension.
Do not infer a cohort from a ratio alone.

The first candidate must explain both time and memory.
The candidate must preserve the locked-C tree.

### B16.3 — formulate one generic mechanism

Write one mechanism proposal for the largest weighted cohort.
State the C behavior that the mechanism preserves.
State the Go state that the mechanism removes, shares, or bounds.

Define the change as a small, reversible tranche.
Keep the change generic across grammars.

Reject the proposal when it needs any of these conditions:

- A language-name branch.
- A blob digest admission exception.
- A driver-local exception.
- A new grammar profile grant.
- A changed locked-C result.
- An unbounded memory structure.
- A default-off feature without a fusion or table-replacement design.

### B16.4 — validate the candidate

Run the smallest parity suite that covers the changed recovery path.
Run the Swift witness in isolation.
Run the recovery benchmark with randomized order.

Use these stable settings for every comparison:

- `GOMAXPROCS=1`.
- One process per explicit shuffle seed.
- Twenty shuffle seeds.
- `-count=1`.
- `-benchtime=750ms`.
- `-benchmem`.

Use `scripts/run_randomized_benchmarks.sh` for the primary benchmark comparison.
Do not use fixed-order output as comparison evidence.

Keep the candidate only when all gates pass:

- Locked-C tree identity remains exact.
- The target cohort improves in `ns/op`.
- Allocated bytes do not regress materially.
- Allocation count does not regress materially.
- Maximum resident set size does not regress.
- No crash, stop, out-of-memory failure, or retention failure appears.

Record rejected candidates as tombstones with the failed gate.

### B16.5 — earn fleet credit

Project saved Go time from measured runtime facts.
Use the C0f signal denominator.

Admit performance credit only when the projected saving reaches two percent.
Then rerun the affected cohort and the full C6f epoch.

The affected cohort must hold or improve every C6f class metric.
The sealed C0e board remains a separate regression gate.

## Acceptance receipt

Close B16.0 when the counter schema and reset rules are documented.

Close B16.1 when the witness matrix contains the Swift and control receipts.

Close B16.2 when every admitted cohort has a runtime-fact predicate.

Close B16.3 when one generic mechanism has a parity-preserving design.

Close B16.4 when the randomized comparison and isolated parity gates pass.

Close B16.5 only after the projected saving reaches two percent and the C6f epoch holds.

Every receipt must include:

- The source revision.
- The source and grammar digests.
- The exact command.
- The result class.
- The C parity result.
- The performance result.
- The memory result.
- The rejected alternatives.

## Stop conditions

Return to the brief when the counters cannot distinguish recovery from materialization.

Return to A8 when the first C and Go live-version divergence remains unexplained.

Return to T1 when the top-20 clean cards expose a separate generic mechanism.

Do not spend B16 time on broad clean-path tuning.
Do not spend B16 time on new blob-SHA certificates.
Do not spend B16 time on the kill ledger.

## Current decision

Start B16 design now because C0f confirms that the error cohort owns 72.2586 percent of measured Go time.

Hold performance credit until the missing runtime facts and the two percent projection gate close.
