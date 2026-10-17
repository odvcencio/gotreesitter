# Owned snapshot allocation screen

Evidence date: 2026-10-16 UTC. Baseline:
`0808dfd5fbfb1367bf4d8b80ff0b5ceb897fda3c`.
**Reject the prototype:** fewer allocation objects do not establish a
throughput improvement, while allocated bytes rise. Runtime source and routing
remain unchanged. All main merges remain held for 0.56.0.

## Attribution and ownership

Fresh native profiles use the committed canonical Go and owned-YAML benchmarks
under serialized Docker, CPU 6, `GOMAXPROCS=1` and Go 1.25.14. Each process
captures an eight-second CPU/allocation profile. These are attribution runs,
including benchmark setup, not timing comparisons or per-parse allocation
measurements. Nested cumulative percentages are not added together.

In the owned 8 KiB YAML process, snapshot construction has 54.97% of sampled
allocation bytes directly; its DFA cloning child has another 19.30%. Version
state publication accounts for 15.54%. Line attribution places 47.51 MB of
the cloning sample at the boolean retry-mask copy and 11.50 MB at the payload
copy. A separate one-iteration census observes 400-byte snapshot records,
64-byte version records and 113 retry booleans, with a source capacity of 128.
Token-start/end byte vectors are empty on these measured YAML paths.

The prototype allocates a zero snapshot record and its private retry flags in
one typed Go object. Generic 16/32/64/128-entry tails serve small masks;
larger masks retain independent ordinary allocations. Nil and empty vectors,
all values and lengths survive. No language names select the representation.
There is no unsafe conversion, pool reuse, bit packing or omitted validation.
The original clone helper remains available for other callers.

Flags are copied before the later capability callbacks can mutate scratch.
Checkpoint authentication and byte copies remain. The returned flag capacity
exposes the complete reserved tail, so existing capacity-based footprint
accounting charges it. Appending through that capacity cannot reach a different
record. Snapshot cloning and publication retain their independent copies;
live lexer restoration copies the mask into its own buffer.

## Checks before timing

Full parser-core tests, focused helper race/386 tests and the existing version
lexer ownership, mutation, binding, lifecycle and footprint tests pass in
Docker. Helper tests cover nil/empty masks, bucket boundaries, 113 entries,
the 129/1,024-entry fallback and one allocation for an eligible record/mask.
Lifecycle/memory-stop checks, focused YAML locked-C witnesses and Go/YAML
48-step candidate edit sessions also pass, with zero no-edit allocations.

Fresh native/prototype ledgers match in all 412 ordinary rows and the two
diagnostic-owned YAML rows. Four canonical Go tree/Core.Work pins and all
21 raw work-count fields stay exact. All YAML timing work/route columns match.
No counter exception or reset is requested.

## Completed twenty-pair result

The repository randomized runner alternates roles across seeds 20601–20620,
one process per role/seed, count 1, 750 ms and `-benchmem`. Docker uses one CPU,
CPU 6, a 6 GiB limit, `GOMAXPROCS=1` and both repository locks. Source hashes,
all samples and execution order verify before `benchstat`.

| Case | Time before → prototype | Change | Bytes before → prototype | Objects before → prototype |
| --- | ---: | ---: | ---: | ---: |
| Ordinary YAML attempt/fallback | 195.3415 → 194.1375 µs | −0.62%, p=.052 | 16,760→16,760 | 37→37 |
| Diagnostic-owned YAML R4 | 350.4020 → 350.9835 µs | +0.17%, p=.820 | 12,872→13,352 (+3.73%) | 109→94 (−13.76%) |
| Diagnostic-owned YAML 8 KiB | 1.366831 → 1.363753 ms | −0.23%, p=.369 | 44,313→46,201 (+4.26%) | 336→277 (−17.56%) |

All three time directions are inconclusive. The measured byte increases are
480 and 1,888 per parse: 32 bytes for each of the 15/59 combined allocations.
This is consistent with allocator rounding of the larger object. The object
count improvement alone does not meet the throughput objective.

The screen stops after completing all twenty YAML pairs. No Go timing
comparison, full 206-language promotion gate, strict real-file C census or
1 MiB RSS qualification is claimed for this rejected source. The earlier
runtime's qualification and merge hold are unchanged.

## Evidence and next target

Profiles, line attribution and the diagnostic shape census are retained in
`20261016-runtime-hotpath-profile`. Frozen prototype source, scoped checks,
414-row comparisons, raw timing pairs, `benchstat`, medians and the rejection
decision are in `20261016-snapshot-flag-coallocation`. Both directories live
under `/home/draco/.local/state/gotreesitter-evidence/`.

The fresh Go allocation profile also highlights dependency histories and leaf
receipts. Those representations already have qualified changes in held
[incremental PR #1480](https://github.com/odvcencio/gotreesitter/pull/1480).
Do not duplicate that lane or treat its allocation wins as throughput evidence.
Further architectural work needs to account for those pending contributions.
