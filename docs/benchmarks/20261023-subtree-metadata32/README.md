# 32-byte subtree experiment: not adopted

Moving field and alias ranges into a sparse sidecar shrinks each hot compact subtree record from 44 to 32 bytes. Each record with metadata then needs a 16-byte descriptor. The full qualification passed, but the 20-pair comparison did not establish a throughput improvement and large-Go allocated bytes increased 2.98% (`p=0.007`). The experiment is archived here and removed from the runtime branch.

The baseline is the frozen `native-record-receipts-v2` implementation, source-equivalent to the runtime installed at `154bac17b`. The candidate is `6eb1ffdbd` plus [the complete source patch](source.patch.gz). It preserves full-width child counts, fields, aliases, transaction rollback, shadow cloning, memory accounting, and logical metadata caps. This is not a comparison against main.

| Workload | Baseline | Candidate | Timing result |
| --- | ---: | ---: | --- |
| Compact Go, 1 MiB | 1.016 s | 1.051 s | Inconclusive, p=0.127 |
| Compact Go, full DFA | 12.42 ms | 12.15 ms | Inconclusive, p=0.142 |
| Compact Go, grammar-generation LR source | 140.7 ms | 146.5 ms | Inconclusive, p=0.265 |
| Ordinary YAML, R4 | 331.8 µs | 310.6 µs | Inconclusive, p=0.355 |
| Ordinary YAML, 8 KiB EOF witness | 1.266 ms | 1.226 ms | Inconclusive, p=0.289 |

The full [Go](benchstat-compact-go.txt), [primary Go control](benchstat-go.txt), and [YAML](benchstat-yaml.txt) tables retain all directional changes. Large-Go allocated bytes rose from 258.9 to 266.7 MiB/op; these cold samples vary with pooling and GC, so the observed allocation increase is recorded without attributing all of it to descriptor storage. All 10% median ratchets passed. Twenty matched cold RSS pairs per fixture also passed: Go −4.25%, ordinary YAML +4.99%, owned YAML +2.01%; no probe exceeded the 400 bytes/source-byte ceiling.

Correctness and deterministic counters ran before timing: 412 invariant sessions, 29,664 edit steps, 1,236 standard C checks, 12 large-Go C comparisons, YAML ownership/EOF probes, 20 memory checks, focused race checks, and actual 386 tests passed. Every counter and routing row across 206 languages stayed identical. No-edit reparses allocated zero. The exact Cognociente witness retained all 12 independent D8 agreements and eight clean C agreements; the same four malformed-delete C shape differences remain failures.

Timing used the repository randomized driver, 20 explicit shuffle seeds per suite, alternating baseline/candidate order, matching workload order, one process per seed, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`. Runs were serialized in a pinned Go 1.26.6 container with one CPU, 4 GiB memory, no network, and one grammar per correctness process. All 3,561 candidate Go files were hash-identical before and after the run. The container exited zero without OOM and was removed after audit.

The [machine-readable qualification](qualification.json) and [receipts archive](receipts.tar.gz) contain source hashes, the patch, controls, raw samples, counter comparisons, test logs, RSS probes, and CPU profiles. The archive contains 2,424 individually verified members. Profiling is separate attribution evidence: Go reduction construction accounts for 20.06% of samples and publication for 27.97%; ordinary YAML reduction construction accounts for 18.22% and the projection stream for 16.71%. Cumulative shares overlap and must not be added. These costs motivate removing repeated construction and publication work rather than further record-width changes alone.

The combined runtime remains held on its existing main-based performance and correctness gates. This experiment does not graduate compact or qualify another platform.
