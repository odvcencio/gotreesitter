# Mac ARM64 checkpoint-layout qualification

The owner approved the exact source overlays and benchmark controls. An isolated four-GiB Linux ARM64 Docker run on the Mac completes with Go 1.25.14, GOMAXPROCS=1, no OOM, unchanged source hashes and all 3,546 baseline Go files authenticated against runtime `dba13461f0df4dc8eaf626a80d143a7cf0e8e5ab`. Exactly seven candidate Go files differ. The shared benchmark overlay is identical. No user checkout changes.

Full core and shadow suites, focused races and ownership tests, four Go/YAML R4 sessions / 288 edits with zero no-edit allocations, twenty memory contracts, and byte-identical Go/YAML counter exports pass. C qualification is local on the identical source; this Mac receipt does not claim another C run or a Windows ARM64/x64 rerun.

Twenty alternating pairs per memory route use seeds 164001–164020. The exact 1,049,572-byte fixture and two-measured-iteration protocol match the preceding local receipt. Each measured parse checks work, completion and route; binaries are compiled outside the measured process and no manual GC is used.

| Route | Median RSS before→after | Maximum RSS before→after | Median bytes / allocations per operation |
| --- | ---: | ---: | ---: |
| Ordinary, existing fallback | 180,674,560→180,721,664 B | 181,506,048→181,673,984 B | 114,437,508 / 38, unchanged |
| Diagnostic-owned compact | 120,788,992→117,751,808 B | 127,213,568→119,627,776 B | 6,631,784 / 24,780, unchanged |

Owned median RSS falls **2.51%**; ordinary changes **+0.026%**. Both pass this protocol's relative and absolute screens. These samples do not replace the separately pinned cold-parse protocol for a future ordinary-ownership candidate.

The randomized runner completes twenty matched pairs for all six YAML cases (seeds 165001–165020) and the three primary Go cases (166001–166020), with count=1, 750 ms and benchmem. Every seed, shuffled case order and alternating revision position is independently checked. All byte/allocation medians stay unchanged except Go full bytes 1,167→1,166; no edit remains zero bytes and zero allocations.

Owned YAML R4 slows **493.7→506.7 µs (+2.63%, p=.005)**, and owned EOF8K slows **1.876→1.939 ms (+3.38%, p=.002)**. Ordinary R4 and EOF8K changes are inconclusive. The legacy R4 control improves 0.93% (p=.024), while legacy EOF8K is inconclusive. Go full measures **5.814→5.742 ms (−1.24%, p<.001)**; edit **82.60→82.51 µs (p=.698)** and no edit **6.063→6.067 ns (p=.909)** are inconclusive. The small Go full difference is observed on a route that does not exercise the scanner checkpoint storage; it does not establish a checkpoint-specific causal speedup. Complete [Go](benchstat-mac-go.txt) and [YAML](benchstat-mac-yaml.txt) tables retain every metric.

The checkpoint layout remains an **unpromoted memory tradeoff**. The owned YAML timing regressions remain visible, and this experiment neither activates ordinary YAML nor fixes the held Go RSS gate. The [raw receipts](mac-receipts.tar.gz) and [member hashes / analysis](mac-qualification.json) pin the completed run. Qualification for a combined ownership/link-layout source remains separate.
