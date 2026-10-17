# Compact graph-link storage screen

Evidence date: 2026-10-16 UTC. Baseline:
`14f255bac37e64670b90b3120534b505ffeb6a1e`.
The experiment stays outside the checkout. **Reject the layout:** it preserves
the scoped correctness checks but makes every measured compact Go and YAML
case slower. All merges to main remain held for the 0.56.0 release.

## Representation and hypothesis

The existing graph link occupies 32 bytes: two 64-bit scalars, three 32-bit
identities and 32-bit flags. A read-only census of the four canonical Go files
finds 148,367 retained links in the largest file. Only 12,719 of those links
have both a nonzero score and a nonzero branch order.

The prototype stores a 24-byte link. Its scalar word holds the score or order
directly; when both are nonzero, it names an immutable 16-byte pair. Two private
flag bits identify the encoding. Semantic flags are checked before encoding,
then recovered exactly. Signed scores and unsigned orders retain all 64 bits;
node, subtree and link identities retain all 32 bits. In particular, a nonzero
order without the order-presence flag remains valid, as in the original code.

For the largest census, `24 × links + 16 × pairs` projects 3,764,312 logical
bytes against 4,747,744 bytes before: 20.71% less link storage. This projection
excludes capacity, transient records and recovery copies. It is not an RSS
measurement or a throughput result.

The integration includes pair lengths in transaction marks, nested rollback
and identity reuse; reset and retention release; EOF recovery cloning and
isolation; reserve calculations; and length- and capacity-based accounting.
Before pair storage grows, the scheduler checks gross new allocation bytes
while the existing backing array remains charged. Rejected growth publishes
no pair or link.

## Correctness and deterministic checks

Before timing, Docker checks pass for the full parser-core package suite, EOF recovery
configuration, race suite, phase-zero-A instrumentation and focused 386 tests.
The codec tests include full-width extrema, 2,000 seeded cases per architecture,
invalid flags and pair references. Lifecycle tests cover budget rejection,
inner commit followed by outer rollback, identity reuse and recovery-copy
isolation.

All four canonical Go trees retain their deep digests and complete `Core.Work`
pins. All 21 raw work-count fields match the baseline. Raw table-lookup counts
remain 7,582 / 37,662 / 37,652 / 400,160 for rewrite, query, language and large
Go respectively. The YAML sample-work check and all benchmark work and route
columns also stay exact. No ledger reset or counter exception is requested.

Focused locked-C checks pass for the four Go files and the existing YAML
ownership, document-stream, quoting and EOF-recovery witnesses. Separate Go
and YAML candidate edit sessions each pass 48 steps; no-edit allocations stay
zero. These checks do not certify the whole grammar registry.

Lifecycle and memory-stop checks pass. The adversarial memory-budget witness
records a peak of 1,104,104 bytes, against the existing bound of 1,153,433;
the earlier native receipt records 1,091,836. The higher peak is retained as a
cost. There is no new 1 MiB RSS qualification for this rejected source.

The first test adapters needed repairs: expected byte totals still assumed
32-byte links; malformed-link errors needed their original wording; and the
phase-zero-A falsifier needed direct malformed-record injection rather than
the checked appender. Original assertions and error-order checks remain.
Failed attempts are archived separately from the passing source.

## Completed twenty-pair comparison

Both sides run under Docker with one CPU, CPU 6, a 6 GiB memory limit,
`GOMAXPROCS=1`, both repository locks, one count per process, 750 ms per
benchmark and `-benchmem`. The repository randomized runner alternates roles
across Go seeds 20201–20220 and YAML seeds 20301–20320. Audits check source
hashes, all samples, execution order and work columns before `benchstat`.

| Case | Before | Prototype | Parse-time change |
| --- | ---: | ---: | ---: |
| Go rewrite | 1.835545 ms | 1.963141 ms | +6.95%, p<.001 |
| Go query | 8.813172 ms | 9.394270 ms | +6.59%, p<.001 |
| Go language | 9.188399 ms | 9.927926 ms | +8.05%, p<.001 |
| Large Go | 94.277597 ms | 101.506838 ms | +7.67%, p<.001 |
| Ordinary YAML attempt and fallback | 193.653 µs | 197.912 µs | +2.20%, p<.001 |
| Diagnostic-owned YAML R4 | 350.624 µs | 365.082 µs | +4.12%, p<.001 |
| Diagnostic-owned YAML 8 KiB stream | 1.366216 ms | 1.430228 ms | +4.69%, p<.001 |

Positive changes mean more parse time. Allocation counts stay
21 / 129 / 243 / 1,716 for Go and 37 / 109 / 336 for YAML. Ordinary YAML
bytes fall 16,760→15,992 (−4.58%); owned YAML stays 12,872 / 44,313.
Go rewrite/query bytes stay 1,776 / 138,725; language rounds
284,629→284,630, and large Go moves 3,251,743→3,251,712.

The default full/edit/no-edit controls move −0.54% / +0.33% / −2.44%
(p=.005/.410/<.001). They bypass this compact representation, so these
directions receive no attribution. Their allocations stay 6 / 5 / 0 and
bytes stay 1,159 / 394 / 0.

The compiler reports costs of 187 for unpacking and 101 for the original
shape validator, above the inlining budget of 80. Extra decoding around
validation is a plausible contributor to the slowdown; this comparison
does not isolate its share. A separate ordinary-link validation experiment
must establish its own correctness and timing evidence.

## Decision and evidence

Keep the current runtime and routing. Do not promote the smaller layout or
count its projected storage saving as a runtime win. This rejected screen
does not close [E-A2](https://github.com/odvcencio/gotreesitter/issues/1314)
or [E-A7](https://github.com/odvcencio/gotreesitter/issues/1319), and receives
no broader fleet or graduation qualification.

Frozen sources, attempts, qualification logs, both twenty-pair raw outputs,
`benchstat` reports and audits are under
`20261016-tagged-link-storage/{integration,qualification}` in
`/home/draco/.local/state/gotreesitter-evidence/`. The independent native
census is `20261016-link-occupancy`.
