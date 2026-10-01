These locals and injections queries are copied from the grammar commits in
`grammars/languages.lock`. Highlights use the registry's composed query, so
TypeScript includes its inherited JavaScript patterns.

| Grammar | Commit | Original query directory |
| --- | --- | --- |
| JavaScript | `58404d8cf191d69f2674a8fd507bd5776f46cb11` | `queries/` |
| TypeScript | `75b3874edb2dc714fb1fd77a32013d0f8699989f` | `queries/` |
| Rust | `77a3747266f4d621d0757825e6b11edcbf991ca5` | `queries/` |

`TestQueryPerfCaptureParity` compares every capture, including private captures
and duplicates, against the locked C binding and a native C query loop. It
checks tree equality before querying. Captures are ordered by byte range,
pattern index and capture ID; this does not assert cursor streaming order.
Directive metadata does not change the raw captured node spans being compared.
The native loop supports the text predicates used by these queries. Its POSIX
regular expressions translate `\d`, `\w` and `\s`; agreement with the binding's
Go regular expressions is checked on every measured workload.

`BenchmarkQueryPerf` measures precompiled query execution, predicate evaluation,
capture allocation and ordering. Parsing and the one-byte edit/reparse are
setup work. The edit range extends 128 bytes on either side of the edited byte.
Set `GTS_QUERY_PERF_LANGUAGE` to run one language per process. Use
`scripts/run_randomized_benchmarks.sh --tags treesitter_c_parity --package .`
from `cgo_harness/`, with a benchmark expression selecting `BenchmarkQueryPerf`.
The benchmark explicitly shuffles Go/C order from the process's shuffle seed.
C uses libc allocation directly; C allocations are not included in Go's
`B/op` or `allocs/op` counters.
