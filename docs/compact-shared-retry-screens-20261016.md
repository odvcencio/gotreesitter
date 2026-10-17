# Immutable YAML retry flags: allocation savings fail the throughput screen

Evidence date: 2026-10-16 UTC. Each comparison starts from
`86879374e37ef0028443121d1bf12214377653a6`.
**Reject all four private implementations.** They reduce owned allocations but
slow ordinary YAML. Runtime source, routing and graduation stay unchanged.
All merges to main remain held for 0.56.0.

The warm operation census finds 15 / 59 snapshot captures and private retry-mask
copies on owned R4 / the 8 KiB stream, zero snapshot clones, and one distinct
complete mask per fixture. The prototypes share immutable ordinary booleans;
they avoid the previously rejected packed-mask conversion. Matching compares
all booleans and restores copy into private mutable storage. Wide masks retain
a native fallback. No scanner authentication, metadata identity, callback,
validation, counter or memory-budget check is removed.

| Private source | Ordinary YAML | Owned R4 | Owned 8 KiB stream | Seed range |
| --- | ---: | ---: | ---: | --- |
| `abi-fixed` | +1.79%, p<.001 | +0.26%, p=.718 | −0.45%, p=.060 | 22901–22920 |
| `large-fallback` | +1.23%, p<.001 | +0.18%, p=.718 | −0.51%, p=.007 | 23101–23120 |
| `metadata-cache` | +1.56%, p=.002 | −0.14%, p=.799 | −0.81%, p=.004 | 23301–23320 |
| `presence-codec` | +1.93%, p<.001 | +0.41%, p=.799 | +0.11%, p=.718 | 23601–23620 |

The first source grows the owned snapshot 400→408 bytes and adds a scheduler
hint. The second restores old scheduler field offsets and uses native storage
above 128 flags. The third puts the cache in cold frozen metadata, using DFA
padding for its storage discriminator. The final nine-file source leaves the
entire raw DFA capture/restore source unchanged. It encodes the storage mode
reversibly through redundant scanner presence in the owned copy, authenticates
the decoded presence against independent metadata and the fresh contract, and
rejects flipped presence or lost mode. Scheduler / runner / DFA / owned snapshot
sizes stay **9,304 / 11,192 / 232 / 400 bytes**, with all original fields and
offsets exact. That layout result does not establish a throughput improvement.

Every source changes owned R4 **12,872→10,952 B/op** and **109→94 allocations**
(−14.92% / −13.76%). The stream changes approximately **44,313→36,760 B/op**
and **336→277 allocations** (−17.04% / −17.56%). Ordinary YAML remains
**16,760 B/op and 37 allocations**. These are real allocation savings on
rejected sources, not runtime wins. Final ordinary medians are
192,823→196,550 ns/op; owned R4 348,778→350,207; stream
1,355,496→1,357,026.5.

Each source passes focused core/root ownership, mixed-storage, mutation,
retention and memory checks, leaf normal/race/386 coverage, focused YAML C
comparisons, four canonical Go C comparisons and separate Go/YAML 48-step
edit sessions with zero no-edit allocations. All **414 ledger rows**, four Go
deep tree digests/Core.Work vectors and all 21 raw work fields remain exact.
Lookup proxy stays **7,553 / 37,640 / 37,479 / 397,227**. No counter exception
is requested. These scoped gates are distinct from fleet certification.

The repository runner uses 20 alternating shuffled pairs per source,
GOMAXPROCS=1, count 1, 750 ms, benchmem, CPU 6, 6 GiB Docker and both locks.
All source/data hashes, samples, orders, timed completion and work columns
verify. A separate identical-source control, seeds 23401–23420, finds ordinary
+0.20% (p=.314), owned +0.52% (p=.242), stream −0.37% (p=.758): all inconclusive.
This establishes no systematic path bias. Control percentages are not
subtracted from candidate results.

Buckley's completed bounded source feedback finds no concrete defect in the
first two supplied diffs, subject to packet limits. Metadata and final-codec
reviews end incomplete (`invalid_completion` or the tool-call limit), even
where the process returns zero. They provide no approval or source-execution
certificate. Review packets are removed from the workspace. Earlier constructor
and missing-helper failures are archived before the final frozen runs; their
partial checks do not qualify those earlier sources. The corrected retained
charge test checks the complete cache footprint rather than weakening its gate.

No Go timing, 206-language invariant fleet, 618-case C fleet, strict real-file
census or large-file RSS qualification runs for these rejected candidates.
Their scope does not replace the held core PR's qualified evidence. Frozen
source, pins, tests, counters, audits, review limits and benchstat remain in
local `gotreesitter-evidence/20261016-shared-frozen-retry-flags`, under the four
source names above. The next Go target follows E-A7: remove avoidable native
row-resolution indirection while preserving fresh classification authority.
