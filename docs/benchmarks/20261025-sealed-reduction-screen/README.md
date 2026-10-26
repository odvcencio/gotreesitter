# Sealed deterministic reduction screen

**Hold the private runtime.** The kernel produces modest Go gains, but this
cohort establishes no useful YAML gain relative to its legacy control. It does
not complete the E-A gate or graduate a language. Main merges remain held.

The experiment pairs each immutable singleton reduction row with its already
validated production plan. A Core binds the exact table and plan providers
before graph construction. Only its owned fresh single-version continuation
uses the pair. Ordinary diagnostic providers, standalone reducers, ambiguity,
recovery, GOTO callbacks and their subsequent validation retain their existing
behavior. The measured runtime is independent of the held construction-output
and constructed-receipt experiments.

## Randomized timing

The unchanged randomized runner completed 60 matched pairs in 120 processes:
20 shuffle seeds per suite, alternating arm order, identical workload order,
750 ms, count 1, GOMAXPROCS 1, and benchmark memory metrics. Docker used one
pinned CPU, 4 GiB, no network, and Go 1.26.6 on Linux amd64. No activation
instrumentation was included in the measured runtime.

| Workload | Before median | After median | Change | p |
| --- | ---: | ---: | ---: | ---: |
| Generated Go full | 7.1159 ms | 6.9513 ms | −2.31% | <0.001 |
| Generated Go edit | 67.626 µs | 67.607 µs | inconclusive | 0.774 |
| Generated Go no edit | 3.1045 ns | 3.1180 ns | +0.43% | 0.016 |
| Ordinary Go compact | 764.050 µs | 755.953 µs | −1.06% | <0.001 |
| Ordinary Go legacy control | 640.182 µs | 637.882 µs | inconclusive | 0.091 |
| Ordinary YAML compact | 192.267 µs | 191.175 µs | −0.57% | 0.026 |
| Ordinary YAML legacy control | 136.475 µs | 135.662 µs | −0.60% | <0.001 |

Warm allocations remain unchanged: generated full/edit/no-edit use 22/5/0
allocations, ordinary Go compact uses 14, and YAML compact uses 39. No-edit
bytes and allocations remain zero. The no-edit directional regression is
recorded even though this experiment targets construction. YAML's legacy
control improves by roughly the same amount as compact, so its timing does
not establish an incremental gain from the kernel.

## Correctness, counters and memory

Untimed ordinary-entry instrumentation proves actual plan-provider calls fall
707→214 for Go and 196→14 for YAML. GOTO calls remain 723 and 187; Core
reductions remain 686 and 187. Complete output, cold receipts and logical work
match. Instrumentation is absent from the performance map.

All four actual default/candidate Go/YAML ledger files equal the frozen actual
baseline files exactly. These are collected rows, not a comparison against
looser tracked ratchet limits. Representative before→after values are:

| Route / language | Full tokens | Full nodes | Edit nodes | Edit reused bytes | Edit splices |
| --- | ---: | ---: | ---: | ---: | ---: |
| Default Go | 589→589 | 1922→1922 | 3810→3810 | 1166→1166 | 296→296 |
| Candidate Go | 589→589 | 892→892 | 1553→1553 | 1784→1784 | 266→266 |
| Default YAML | 107→107 | 288→288 | 480→480 | 0→0 | 0→0 |
| Candidate YAML | 107→107 | 232→232 | 232→232 | 0→0 | 0→0 |

The repaired private Core controls pass 23 families in release, work-count and
race builds. Further gates pass 55 existing Core families, 36 focused root
families, three Go public API families, ten Go C families, eight YAML C
families, and the original report witnesses: the 137 KiB top edit, malformed
declaration prefixes and all 96 assignment edit cases. Four R4 invariant
sessions cover 288 edit steps with no-edit allocations zero. Three public
tree race families pass. These are scoped gates against the existing locked
C reference, not a new C-version or platform qualification.

The new unique cold table storage is 29,368 bytes for Go and 48,160 bytes for
YAML. Complete observed table heap deltas are 133,904 and 243,800 bytes; both
pass the existing 4 MiB guard. Shared plan buffers are not charged again for
each sealed row. The bound Core separately charges its 48-byte binding.
Existing footprint and cache-sharing tests pass unchanged.

Two initial test fixtures failed: one primed a shifted boundary while testing
an unshifted reduction collision; another attributed an explicit callback's
frontier reset to operation publication. Identical-witness runs against both
paths establish those distinctions. Test-only repairs retain collision,
authority, poisoning, reset and no-publication assertions; the runtime map
never changed. Original failures and adjudication remain in the archive.

## Evidence and limits

[summary.json](summary.json), [verification.json](verification.json) and
[source-identities.json](source-identities.json) identify the inputs and
selected checks. [evidence.tar.gz](evidence.tar.gz) contains source overlays,
controls, complete raw samples, benchstat output, counter rows and historical
failures. [contents.json.gz](contents.json.gz) records each member's size and
SHA-256. Validate without Go or benchstat:

```sh
python3 docs/benchmarks/20261025-sealed-reduction-screen/validate-evidence.py
```

No production runtime or routing change is included. Full 206-language
qualification, API-consumer timing, large-file RSS, ARM and other platforms
were not run for this held kernel. The larger next target is eliminating
intermediate graph construction in deterministic runs, alongside direct
publication from owned compact bodies. Cognociente's small-edit verifier
remains in place; this experiment does not certify its removal.
