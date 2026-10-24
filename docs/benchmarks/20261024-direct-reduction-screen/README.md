# Direct reduction kernel: not promoted

The direct reduction experiment preserves the tested semantics but does not deliver the requested architectural speedup. Twenty paired runs show compact Go full DFA improving 6.03%, the 1 MiB case 3.41%, two real Go files under 2%, and ordinary YAML 2.46–2.98%. The runtime patch is archived and removed from the checkout. No merge or graduation follows.

Baseline: `6fe1f7c1a05af777223024d96d960007f8bd7fd3`, on the stacked #1512 branch. The last installed runtime change remains `6d6ead01873d54d8cda839947e04b387474190ca`. These are branch-relative measurements, not main-based qualification. All results below use Linux amd64 Docker on CPU 6, Go 1.26.6, GOMAXPROCS=1, one process per explicit seed, count=1, 750 ms, and benchmem. Correctness and deterministic counters finish before randomized timing.

## Experiment and result

A new internal kernel walks a bounded deterministic predecessor chain, constructs its reduction, and publishes the resulting head and lineage in one owned operation. It bypasses the general reducer's scratch/output machinery. Extras, ambiguity, recovery, historical boundaries, and wider reductions retain the general reducer. A declined probe carries its authenticated GOTO into that reducer so it does not perform a second table lookup. The scheduler attempts the kernel before entering the general reducer's larger call frame.

| Compact workload | Baseline | Experiment | Time change | p |
| --- | ---: | ---: | ---: | ---: |
| Go full DFA | 8.744 ms | 8.216 ms | −6.03% | <.001 |
| Go fresh 1 MiB | 630.6 ms | 609.1 ms | −3.41% | <.001 |
| Go LR source | 102.2 ms | 100.5 ms | −1.70% | .035 |
| Go query source | 9.714 ms | 9.537 ms | −1.82% | <.001 |
| Ordinary YAML R4 | 211.4 µs | 206.2 µs | −2.46% | <.001 |
| Ordinary YAML EOF 8 KiB | 795.3 µs | 771.6 µs | −2.98% | .001 |

Allocation counts remain identical on all 15 workloads. No-edit stays at zero bytes and zero allocations. Every timed work and routing metric is exact, and all 10% median ratchets pass. The [complete Go](benchstat-compact-go.txt), [primary control](benchstat-go.txt), and [YAML](benchstat-yaml.txt) tables preserve every result.

Diagnostic-owned YAML timing remains inconclusive, with R4's median rising 0.21%. Primary Go full/edit timing is inconclusive. No-edit and legacy-YAML controls also move despite not executing the new kernel; those movements are not attributed to reduction work. Independent bucket percentages must not be added.

## Correctness and counters

- All 824 whole-operation work rows match: 206 grammars, two routes, full and edited operations, including fallback and hidden verification work. No event increase or reuse decrease is hidden behind the selected result.
- All 206 selected-result ledger comparisons preserve work, reuse, completion, error, coverage and route metadata exactly. No exception or reset is used.
- All 412 invariant sessions pass 72 edits each: 29,664 steps, with zero no-edit allocations. All 1,236 standard C checks pass without skips, one language per process.
- Core normal and diagnostic builds, core/incremental races and actual 386 execution, 44 focused root tests, root race checks, 20 memory contracts, larger YAML ownership/EOF probes, and 12 large-Go C comparisons pass.
- The new kernel's differential tests cover empty through 32-child reductions, source-order precedence, fields/aliases, converged lineage, and the general reducer's zero-end span behavior. Refusal tests retain graph/work state; fallback performs one GOTO; ignored capacity errors still poison and discard the owned session. Metadata callbacks cannot hide recovery cost.

The latest pinned Cognociente witness retains 12 independent incremental/fresh agreements and eight clean C agreements. Its four malformed-delete C shape failures remain unresolved: `/source_file/function_declaration[2]/block[4]/ERROR[1]` has one child in Go and two in C. Passing standard fixtures does not turn those failures into exemptions.

## Native C comparison and remaining target

Twenty shuffled cycles per language run each fixture/mode in Go-C-C-Go order, for 640 timing rows. Exact deep-tree admission precedes timing. The C side calls `ts_parser_parse_string`, validates coverage/error state, optionally walks a native tree cursor, and releases each tree entirely inside C. There is one Go-to-C call per sample, not per parse or node. Both sides reuse a warm parser; source loading and parser creation are outside timing.

| Fixture | Go parse median | C parse median | Paired Go/C parse ratio | Paired Go/C with full traversal |
| --- | ---: | ---: | ---: | ---: |
| Go query source | 9.436 ms | 3.225 ms | 2.929× | 2.603× |
| Go LR source | 100.249 ms | 34.051 ms | 2.937× | 2.681× |
| YAML R4 | 196.901 µs | 94.920 µs | 2.076× | 1.882× |
| YAML EOF 8 KiB | 757.511 µs | 380.869 µs | 1.984× | 1.820× |

Ratios are the median of 20 within-cycle ratios; backend times are medians of the corresponding two samples per cycle. Go/C allocation numbers are not comparable because Go's allocation metric omits C malloc. The [complete ratio analysis](C-analysis.json) retains all paired ranges and raw-output hashes. This uses the repository's locked C **0.25.1**, commit `f5afe475deb7c0bae6407fb776c76824f717bb61`: the runtime is statically linked into the cgo test binary and the pinned grammar is a shared object compiled at `-O2`. It is a native-loop diagnostic, not the separate standalone-static publication protocol or a C 0.27.x graduation receipt. These ratios describe the archived experiment, not the restored runtime.

Reaching 2× C from 2.937× requires about **32% less total Go parse time** on that witness. Matching C would require about 66%. This experiment establishes neither result.

Separate profiles of Go full DFA/LR and YAML EOF8k locate the remaining cost. Disjoint stack classification excludes setup and avoids adding overlapping cumulative pprof rows:

| Sampled parse work | Go | YAML |
| --- | ---: | ---: |
| Reduction construction and publication | 26.08% | 23.91% |
| Shift construction and publication | 15.71% | 8.56% |
| Native output projection and reads | 19.06% | 14.54% |
| Election outside token production | 6.78% | 12.92% |
| Token production inside election | 11.08% | 17.61% |
| Other parse work | 21.29% | 22.46% |

The [attribution](profile-attribution.json) and raw stacks are diagnostic samples, not estimates of removable work. They reject a reduction-wrapper-only strategy for the requested larger result. The next architectural candidate should combine a tight, owned single-version transition loop with a directly readable compact output representation: avoid intermediate reduction outputs, repeated header publication, and per-node replay into a second pointer tree. Ambiguity, extras, scanner ownership, historical boundaries and recovery must retain exact handoff semantics. The earlier private suffix stack regressed despite reducing graph nodes; repeating its simultaneous graph/vector storage and import/flush bookkeeping is not evidence of a viable shortcut. D4's structure-of-arrays output and lazy stable views also need complete parse-plus-consumption measurements, so deferred work cannot masquerade as a speedup. These are implementation targets, not completed changes or promised gains.

The known delete failure now also has a **31-byte witness**: `package p\nfunc f(){ := a + b\n}\n`. It fails against C on both routes. The trace isolates a lexical difference: Go returns one `errorSymbol` run over bytes 20–22 (`:=`); C returns separate `:` and `=` tokens. The 114-byte report prefix reproduces the original ERROR-child mismatch too. All four reduced parity checks remain failed in the archive; reducing the witness does not fix it.

## Evidence and limits

The [qualification](qualification.json), [unapplied source patch](source.patch.gz), and [complete receipts](receipts.tar.gz) retain source manifests, all overlay backings, commands, raw samples, counter ledgers, logs and profiles. The runtime patch reverse-applies cleanly to the measured candidate. Measurement baseline overlays use empty package stubs for the two newly added files; the published patch adds those files normally. The repository's randomized runner is retained byte-for-byte; a wrapper ensures the complete-operation counter gate has passed first.

The first final-source run stops because Git does not yet trust the read-only baseline mount. Real repository metadata is verified; the resumed run configures that exact mount and retains VCS stamping. Completed source-identical checks are reused, and the remaining ledgers and all timing then finish. Two native-C harness builds fail before producing samples: first the cgo backing file has a `.txt` suffix, then the test references the wrong exported reset helper. Both failures and their fixes remain recorded; no timing samples are discarded.

No new large-file RSS or ARM64/native-platform qualification is claimed. This runtime experiment is not promoted, so those additional gates are not spent on it. The existing main-based, allocation, warmed-RSS, malformed-input correctness and graduation holds remain. PR #1511's separate large incremental improvement still requires its exact counter exception and independent composition/memory qualification.
