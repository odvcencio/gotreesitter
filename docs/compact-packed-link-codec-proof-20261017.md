# Packed graph-link codec proof — 2026-10-17

**The 16-byte codec round-trips 184,235 actual links exactly.** It is a private
component proof; the active parser still stores 32-byte logical link records.
There is no parser throughput, allocated-byte or RSS improvement claim.

A read-only census of qualified runtime
`5e52cf2a2644f3d38785513eebc39f8cf8a506b2` motivates the target. Every current
link carries three 32-bit graph references, a 64-bit score, a 64-bit branch order
and 32-bit flags. The sampled values fit two lossless inline formats:

- Zero score, a 30-bit order, and an optional order flag.
- A signed 16-bit score, a 13-bit order, and an optional order flag.

These widths choose an encoding, not a parser limit. The codec returns a
full-width-storage request for every other value, including extreme signed
scores, full unsigned orders, recovery metadata and unknown flags. Unflagged
nonzero orders remain intact. The three graph references keep all 32 bits.
Full-width storage itself has not been implemented or integrated.

## Actual-record proof

The tagged observer runs after scheduling, without disabling the corridor.
The probe checks the complete core work record before and after inspection.
Every inline result is decoded and compared against the entire original
logical link. Explicit round-trip counters must match the range census.
A second unobserved parse preserves the S-expression hash and native token/node
counts; all seven returned trees are clean and cover their inputs.

| Witness | Exact link round-trips | Zero-score format | Small-score format | Full-width requests |
| --- | ---: | ---: | ---: | ---: |
| go/real_sample | 1,381 | 1,223 | 158 | 0 |
| go/query_compile | 14,662 | 12,596 | 2,066 | 0 |
| go/rewrite | 2,995 | 2,460 | 535 | 0 |
| go/language | 15,073 | 11,376 | 3,697 | 0 |
| go/grammargen_lr | 148,367 | 118,762 | 29,605 | 0 |
| yaml/owned_r4 | 355 | 355 | 0 | 0 |
| yaml/owned_stream | 1,402 | 1,402 | 0 | 0 |

The large Go witness includes scores **-548 through 26**, and its largest
branch order is **8,155**. Larger files can exceed the inline ranges and must
retain their complete values. These seven samples do not prove a storage or
performance result for all grammars.

## Component validation

Normal, race and 386 runs pass the boundary tests and **10,000 generated
cases**. Tests cover both signed limits, both inline-order boundaries, unflagged
orders, maximum-width graph references, full 64-bit values and recovery/unknown
flags. The packed type is exactly 16 bytes on both tested architectures. Full
core unit tests also pass; the parser's actual storage remains unchanged.

The initial unit build failed because a tagged census hook was absent while
an instrumented root driver was used. Its [failure log](benchmarks/20261017-packed-link-codec/initial-build-failure.log.txt)
is retained. Normal component tests now use the uninstrumented root driver;
the separate corpus runs enable the hook's explicit build tag. This was a
harness correction, with no codec source change.

[Result and limits](benchmarks/20261017-packed-link-codec/result.json),
[all corpus counts and identities](benchmarks/20261017-packed-link-codec/corpus.json),
[source hashes](benchmarks/20261017-packed-link-codec/source-pins.json),
[codec source](benchmarks/20261017-packed-link-codec/link_record_packed.go.txt),
[boundary/generated tests](benchmarks/20261017-packed-link-codec/link_record_packed_test.go.txt),
and the [complete private patch](benchmarks/20261017-packed-link-codec/codec-and-census.patch)
retain the evidence. Logs and original Docker scripts are beside them.

For isolated reproduction, install only the codec and its test into an unchanged
baseline checkout and run the following in Docker, then repeat with race and
386 settings:

```sh
go test ./internal/parsercorephase0 -run '^TestPackedLinkRecord'
```

For the corpus proof, apply the full zero-context patch and run the tagged
`TestLinkStorageMetadataCensus` separately for Go and YAML using
`gts_parsercorephase0,gts_link_storage_probe`. The archived scripts use the
original frozen-overlay lab. No fixed-order timings are comparison evidence.

The next step is integrating packed links with full-width cold storage while
preserving logical IDs, append-only adjacency, transaction rollback, retention
and byte accounting. That integration still needs all parser correctness,
counter, timing and RSS gates. The earlier
[terminal-selection screens](compact-terminal-selection-screens-20261017.md)
remain rejected and separate from this proof.

A later [storage-integration screen](compact-packed-link-storage-screen-20261017.md)
preserves correctness and counters but slows compact Go and owned YAML. That
integration is rejected; the active parser still uses 32-byte logical records.
