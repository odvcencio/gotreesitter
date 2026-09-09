# Tiny JSON, large Go, tiny JSON retention

Status: incomplete memory graduation evidence. No runtime change was made.
Source commit: d67594aa6160d3135744babaeb2d0412533e3233.

## Workload and controls

Five fresh processes each parse 12-byte JSON, 407,794-byte Go, then the
same JSON. The Go input contains 10,000 functions. Both grammars load
before measurement. JSON receives one warm-up parse. Both parsers stay
alive in the primary sequence. Each tree is released, then two garbage
collections run before retained heap is read. The Go source stays alive
throughout, so its lifetime does not explain the difference.

Runs use the Go 1.25.14 Docker image, one CPU pinned to CPU 18,
GOMAXPROCS=1, an 8 GiB container limit, and the compact admission route.
All 15 primary parses and 15 ownership-control parses routed to compact
without fallback. Trees were checked for errors, source coverage, and
truncation receipts. This is not a C tree-parity certification.

## Primary sequence

Values are medians of five fresh processes. Heap and allocation units
are decimal bytes. Timing is diagnostic, not a randomized performance
comparison.

| Metric | First JSON | Large Go | Second JSON |
| --- | ---: | ---: | ---: |
| Allocated bytes, parse plus release | 6,088 | 370,266,552 | 6,088 |
| Allocated objects, parse plus release | 18 | 77,818 | 18 |
| Retained heap after two GCs | 5,609,936 | 136,792,440 | 136,792,440 |
| GC-scannable heap bytes | 3,805,216 | 48,978,656 | 48,978,656 |
| Parse time, ns | 56,906 | 378,707,654 | 105,349 |
| Release time, ns | 1,070 | 2,009,117 | 1,751 |

Process RSS is recorded separately in the raw time reports. No
statistical runtime regression claim is made from these five sequences.

A separate capacity probe reads struct fields through reflection,
without reading slice elements or mutating parser state. It skips
language tables, stops at depth 12, and tracks visited pointers.
All 316 observed JSON parser array/slice capacities are identical before
and after Go. This proves equality for the observed fields; it is not
a complete inventory of backing allocations or global pools. Zero
scratch fields in ParseRuntime are not evidence of zero compact storage.

## Ownership control

Five more fresh processes drop the Go parser reference after its parse.
The second JSON then retains a median 52,312,440 bytes, compared with
5,609,344 bytes before Go. Its allocation count and bytes remain 18 and
6,088. This removes about 84.5 MB from the primary retained heap, but
does not restore the initial process footprint.

Separate heap profiles corroborate the ownership split. With the Go
parser alive, compact node, subtree, and graph-link records account for
about 77 MiB of sampled live storage. After dropping it, those allocations
no longer dominate. Node-arena overflow still accounts for about 38 MiB,
with further node slices, primary arenas, and metadata.

Profiles are sampled and instrumented. Use the unprofiled MemStats rows
for exact measured heap totals. Incidental VHDL registry initialization
comes from importing grammars; this probe does not parse VHDL.

## Gate interpretation and next work

The observed JSON parser capacities and per-operation allocations are
isolated from the large Go parse. The full process heap does not return
within five percent. The observed capacity subset alone cannot certify
the complete scratch/storage gate.

Retain both findings. Do not describe all retained memory as JSON
scratch contamination, and do not describe unchanged JSON allocations
as proof that memory graduation is complete.

The next retention change must address ownership and reuse economics:
parser-owned compact storage and node-arena retention have separate
lifetimes. Re-test a later large parse as well as this sequence, because
earlier slab trimming reduced retained heap but increased subsequent
large-parse allocation and time.

## Reproduction

Evidence lives under harness_out/cross-language-retention.
run.sh builds probe.go and runs the five primary sequences. The
capacity-probe.go and drop-owner-probe.go controls have separate outputs.
All source, JSONL receipts, process reports, profiles, and a hash
manifest are in the evidence archive. Profile binaries remain remote
for symbolization and are excluded from the archive.
