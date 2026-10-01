# C fresh-session recovery receipt

The shared recovery changes restore locked-C parity throughout two malformed C
edit sessions. They correct physical version dispatch, closed ERROR merges,
competitor positions, paused lookahead ownership, pause progress, and the cost of
hidden missing terminals. `RecoveryStackVersionOrderEnabled` enables this path
for an exact grammar artifact. Other artifacts retain the conservative path.
C++ has the capability for diagnostics and retains its default recovery opt-out.

Baseline: `9148a96db0dcf95594d72671f48862d72bfee343`.
Engine change: `495713cc76881d6b7706b87701117a06ebaff84d`.
The following measurements include the exact-artifact profile grants.

| Workload | Baseline | Changed |
| --- | ---: | ---: |
| Git ctype.c, fresh Go equals C | 6/72 | 72/72 |
| Git ctype.c, incremental Go equals C | 5/72 | 72/72 |
| Git ctype.c, incremental Go equals fresh Go | 71/72 | 72/72 |
| Git sha256/gcrypt.h, fresh and incremental Go equal C | 5/72 | 72/72 |
| fmt assert-test.cc, default Go equals C | 0/72 | 0/72 |
| fmt assert-test.cc, forced recovery equals C | 12/72 | 29/72 |
| fmt assert-test.cc, incremental Go equals fresh Go | 72/72 | 72/72 |

The ctype.c session uses 1016 bytes with SHA-256
`ef2be0e95b607f509f1fc1cc6ed458bba0a2ae3f9cb94fcf799c3cd54b10a4d8`.
The gcrypt.h session uses 1020 bytes with SHA-256
`6ca73341b81ac7bb8291601b2bc4dc3b6decf93242f3aaedb9cf5ab32042fb73`.
Both come from Git revision `9ac3f193c05c2237e2b14ebaa1149e9fc8a1abe0`.
The C++ session uses 875 bytes with SHA-256
`b5c1ce1f68a2ae546be93288249a8e84bde6d88a876f9b4544c5b1d4f4fa8565`,
from fmt revision `e6a8751df7a68144952d5b21b0d4a76327b6c9f7`.
All sessions use the existing gts_mismatch 72 cumulative-edit recipe. Locked C
incremental parsing equals fresh C at every step.

The oracle is the languages.lock runtime, tree-sitter 0.25.1 revision
`f5afe475deb7c0bae6407fb776c76824f717bb61`, with go-tree-sitter 0.25.0 revision
`adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3`. Grammar revisions are
`b780e47fc780ddc8da13afa35a3f4ed5c157823d` for C and
`c009222808634c1014f82438d4883753516a2c24` for C++.
The external corpus lock was verified against SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`;
it is not stored in this repository.

The pin refresh changes only the C default-route first-edit row. A fresh full
parse is unchanged at 430 tokens, 1064 nodes, and three live versions. Actual
before/after collections contain 412 rows for 206 languages; the other 411 rows
are identical. Several older ledger bounds were already above the measured
baseline and remain unchanged.

| C first edit, 2050 input bytes | Before | After | Change |
| --- | ---: | ---: | ---: |
| Tokens processed | 255 | 320 | +25.5% |
| New nodes | 2903 | 1274 | -56.1% |
| Maximum live versions | 18 | 8 | -55.6% |
| Multiple-version token share, ppm | 745098 | 165625 | -77.8% |
| Reused bytes | 1093 | 1340 | +22.6% |
| Block splices | 32 | 148 | +362.5% |

Both runs accept, cover all 2050 bytes, and report errors. The higher token count
reflects the changed recovery dispatch and requires owner review. The two C
sessions above prove that the shared recovery path produces the locked-C tree,
including fields, ranges, missing flags, and ERROR extras. The ledger fixture
still has a fresh-C mismatch after its first edit; the pin refresh does not claim
that fixture has complete C parity. Gate thresholds stay unchanged. The 2% gate
passes all 412 rows with the separately refreshed C row.

Reduced witnesses are tested on both requested routes. The C witnesses cover an
enumerator, an initializer action transaction, closed ERROR links, competitor
positions, and a paused keyword. The C++ witnesses cover a literal suffix, a
missing-token EOF trial, pause progress, and a hidden missing newline. The
hidden newline contributes 600 error-cost units even when public children omit
it. A unit regression checks that captured cost directly.

Focused recovery, GSS, raw-cost, and memory-budget tests pass in Docker. Existing
C declaration, preprocessor, enum, extern-wrapper, and compact-certification
parity tests pass. C/C++ invariant gates pass for all 72 edits, including
incremental/fresh equality, ERROR-root flags, input coverage or stop reason, and
zero allocation on an unchanged reparse. Exact-blob profile tests pass.

Rejected candidates included physical ordering before actual recovery
(Cylc's 30-second collector deadline), restricting competitor positions by the
shared token start (C parity fell from 72 to 6 steps), and applying ANY-terminal
EOF trials to every recovery grammar (jq's 30-second collector deadline).
Keeping the old exact-EOF trial after adding raw missing costs left C at 15/72.
The final capability scopes physical ordering and ANY-terminal EOF trials to
measured artifacts; the other 205 languages keep their measured counters.

C++ still diverges after step 29. Two remaining witnesses are
`r C+-/x o,e""` and `class D{d e({}}\nd A::b(`, where recovery placement and EOF
selection differ. The six-byte C witness `}if>n;`, reduced from the ledger edit,
still differs in keyword/error placement. These remain open; no existing test
or gate was deleted or disabled.

METRIC: c_ctype_fresh_steps | 6/72 -> 72/72 | 9148a96 -> 495713cc plus profiles | locked Git ctype.c, 1016 bytes, 72 cumulative edits
METRIC: c_gcrypt_full_session | 0/1 -> 1/1 | 9148a96 -> 495713cc plus profiles | locked Git gcrypt.h, 1020 bytes, 72 cumulative edits
METRIC: c_edit_tokens | 255 -> 320 | 9148a96 -> 495713cc plus profiles | pinned C first edit, 2050 bytes
METRIC: c_edit_new_nodes | 2903 -> 1274 | 9148a96 -> 495713cc plus profiles | pinned C first edit, 2050 bytes
METRIC: c_edit_reused_bytes | 1093 -> 1340 | 9148a96 -> 495713cc plus profiles | pinned C first edit, 2050 bytes
METRIC: cpp_forced_recovery_steps | 12/72 -> 29/72 | 9148a96 -> 495713cc plus profiles | locked fmt assert-test.cc, 875 bytes, 72 cumulative edits

## Complete census and remaining gates

The reconstructed locked-corpus census contains 187 languages. For each language
it selects the largest nonempty matching source of at most 1024 bytes, breaking
ties by the relative path. It uses the lock's extensions, basenames, and exact
paths. Both revisions use the same file and SHA-256 for every language. Complete
sessions improve from **38/187 to 39/187**, with C the only new complete session.
The historical 35/187 count in the task is not reproduced at baseline 9148a96;
this receipt reports the measured baseline rather than substituting that count.
Dhall and F# reach the existing 120-second census deadline on both revisions.
CMake changes from 15 to 43 fresh-C matching steps without a full-session change;
this is not attributed to this patch because its measured counters are unchanged
and this recovery capability is absent from its artifact.

The broad invariant audit runs one language per Docker process. It finishes all
206 entries: baseline 200 pass and six time out; changed 199 pass and seven time
out. Elsa, Godot resource, Haskell, Kotlin, Nickel, and PowerShell time out on both.
F# initially passes baseline in 60.806 seconds, then times out at 90 seconds on
both the changed revision and a baseline repeat under the later VM load. There
are no assertion failures among completed runs. The broad gate is **not green**;
the baseline timeouts still need resolution. C and C++ pass their targeted gates.
The final audit reuses successful runs from the immediately preceding candidate;
the only subsequent engine change adds a nil-Language guard to a helper, without
changing valid-parser behavior.

## Randomized timing receipt

All three comparisons use scripts/run_randomized_benchmarks.sh with 20 explicit
shuffle seeds, alternating baseline and changed processes per seed, GOWORK=off,
GOMAXPROCS=1, count=1, benchtime=750ms, and benchmem. Correctness and counter gates
precede timing. Each cgo process requires all six Go/C benchmark names and passes
the harness's fresh and edited C-parity checks; no mismatch override is set.
C and C++ run in separate Docker containers. The VM is shared and busy.

The root trio uses no build tag. The real-corpus suites use treesitter_c_parity
and only the locked ctype.c or assert-test.cc source. They measure the complete
operation, including Tree.Edit for edits. C++ timings use its existing default
opt-out; they do not measure forced C++ recovery. The raw runner cannot resolve
Git metadata inside these containers; source trees are held constant at the
baseline and 00278bafc while benchmarking. The machine-readable
[c-fresh-session-measurements.json](c-fresh-session-measurements.json) records
all per-seed samples, medians, minimum/maximum spread, and both census receipts.

| Go primary benchmark | Before median [min, max] | After median [min, max] |
| --- | ---: | ---: |
| Full | 53.417 [38.283, 64.386] ms | 52.303 [36.704, 103.516] ms |
| One-byte edit | 999.909 [801.342, 1383.651] us | 1068.040 [537.238, 1819.367] us |
| No edit | 41.675 [33.160, 61.390] ns | 45.425 [36.500, 55.140] ns |

Benchstat finds no significant timing change for the Go trio (p=0.738, 0.149,
and 0.398). The full-parse time delta is below 5% and is inconclusive. Full-parse
bytes rise from 1434 to 1510.5 B/op (+5.33%, p<0.001); allocs remain 8. Edit
bytes are 406 to 408, allocs remain 5. No-edit stays at zero bytes and allocations.
The extra recovery-version state is a recorded memory tradeoff.

| Complete operation, Go/C ratio | Before median [min, max] | After median [min, max] |
| --- | ---: | ---: |
| c Full | 21.625 [17.341, 32.209] | 22.750 [15.360, 32.514] |
| c One-byte edit | 4441.416 [3450.135, 6852.360] | 3406.970 [1998.984, 5292.791] |
| c No edit | 0.393 [0.227, 0.596] | 0.399 [0.288, 0.633] |
| cpp Full | 1.526 [0.809, 3.054] | 1.786 [0.941, 3.458] |
| cpp One-byte edit | 4.482 [3.649, 8.516] | 4.257 [2.978, 7.977] |
| cpp No edit | 0.033 [0.027, 0.047] | 0.035 [0.024, 0.066] |

Ratios are medians of the per-process Go/C ratios, not ratios of medians. C full
Go time is 29.57 to 32.45 ms (p=0.096); C edit Go time is 193.4 to 197.8 ms
(p=1.000). The edit delta is below 5% and is inconclusive. The native C edit
median also moves, 44.08 to 56.79 us, which drives much of the apparent ratio
improvement. These runs do not establish a C performance win. C still misses
the complete-operation performance floor by a wide margin on this workload.
C++ default edit time changes 1.342 to 1.122 ms (p=0.033), with wide spread; its
ratio improvement is about 5%, with wide overlapping ranges on this busy VM.

The C edit attribution remains dominated by reparse/rebuild rather than editing
or selecting reuse. Per-seed Tree.Edit, reuse-selection, and reparse values are
included in the JSON receipt for assessing the next optimization.

METRIC: full_edit_sessions | 38/187 -> 39/187 | 9148a96 -> 00278bafc | locked corpus, largest matching input <=1024 bytes, 72 edits
METRIC: go_full_bytes | 1434 -> 1510.5 B/op | 9148a96 -> 00278bafc | BenchmarkGoParseFullDFA, 20 shuffled processes
METRIC: c_complete_edit_ratio | 4441.416 -> 3406.970 | 9148a96 -> 00278bafc | locked ctype.c, median per-process Go/C, 20 seeds
METRIC: cpp_default_complete_edit_ratio | 4.482 -> 4.257 | 9148a96 -> 00278bafc | locked assert-test.cc, median per-process Go/C, 20 seeds

## Large C input

Three alternating before/after check processes parse the locked Git diff.c
(222531 bytes, SHA-256
`dcebb3b4c67fc61d449f649a7da2a4ee7527cc42b0d9e69e096421ca11d89931`)
under /usr/bin/time -v. The default and requested compact routes change from
both mismatching C to both matching C, including the full tree digest. Every
run accepts; the compact request falls back because this input is ineligible.
Max RSS median [min, max] is 150672 [147628, 160608]
KiB before and 62172 [61796, 62420] KiB after.
This is a three-process diagnostic, not a timing comparison. No OOM or memory
budget failure occurs.

METRIC: c_large_max_rss | 150672 -> 62172 KiB | 9148a96 -> 00278bafc | locked diff.c, 222531 bytes, median of three processes
