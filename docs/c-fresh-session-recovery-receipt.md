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
passes all 412 rows with the separately reviewed C row.

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
