# Compact reuse frontier and profile correction

Baseline: 56bedd2a. Native compact repeated reuse remains open.
The Go incremental merge repair remains in place.

## First compact decline

A temporary trace records the frontier at the existing clean-version
decline. Each first edit has two headers, a shared lexer, no recovery
isolation, and already-borrowed subtrees.

| History | Token byte | Borrowed subtrees | Borrowed bytes |
| --- | ---: | ---: | ---: |
| token_class_change | 174 | 3 | 120 |
| same_line_length_change | 32,117 | 53 | 25,901 |
| early_newline | 1,038 | 3 | 124 |
| recovery_deletion | 2,068 | 10 | 1,168 |
| newline_prefix_call_conversion | 1,038 | 3 | 124 |

All four edits in each history still return C-matching trees through the
existing fallback path. Later results are legacy trees and do not show a
new native compact attempt in these traces.

Deferring multi-version handling only before the first borrow would not
resolve these witnesses. Native extension must account for ambiguity after
borrowing. The existing guard stays in place: raw ambiguity selection and
recovery can require derivations an opaque borrowed payload cannot supply.
No multi-version native certification is claimed.

## Returned reuse was overstated

ParseIncrementalProfiled added the discarded compact attempt's borrowed
subtrees and bytes to the successful legacy profile. Those are not reuse
in the returned tree. The two additions are removed. Failed-attempt time,
tokens, dependency checks and recorded allocation work are still added.

A permanent C-oracle regression starts each lane from a fresh compact old
tree. One lane attempts compact incremental parsing; the other disables
that attempt and enters legacy incremental parsing directly. Both results
match fresh C. The length-change fixture gives:

| Metric | Before correction | After correction | Direct legacy |
| --- | ---: | ---: | ---: |
| Reused subtrees | 146 | 93 | 93 |
| Reused bytes | 59,447 | 33,546 | 33,546 |
| Total reported tokens | 436 | 436 | 218 |

The regression fails before the change and passes afterward. It also
requires the compact decline to occur and its token work to remain in the
profile. The repeated-history tests, compact edit matrix and repeated
same-width test pass with the correction.

Prior benchmark raw rows retain their original reuse counters for provenance.
In particular, reuse totals in go-incremental-merge-fix/summary.json include
discarded compact borrowing on the first edit. Do not use those totals as
returned-tree reuse. The measured wall-time and allocation gains are not
recomputed or claimed to improve from this accounting correction.

## Remaining work and evidence

Full failed-attempt allocation and graph-work accounting still needs
validation. This correction does not establish that every existing work
counter is complete. Native multi-version borrowed-node execution, retained
memory, route coverage and retirement gates remain open.

Evidence: harness_out/compact-reuse-frontier contains the trace patch,
five-history trace, red/green regression output and Docker receipts.
The temporary trace is removed from runtime source. The accounting repair
and regression test remain.
