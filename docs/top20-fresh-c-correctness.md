# Seven top-20 fresh-C fixes

All seven target grammars now pass their locked four-file fresh sets. The
selected files improve from 17/28 to 28/28. The complete fresh sweep loses
no previously matching file. Fresh-C edit sessions still have gaps, including
18 passing-step losses that need follow-up.

Baseline: `9148a96db0dcf95594d72671f48862d72bfee343`, after the C reuse-model
merge. Final engine: `556e9180ed154c94c33ecffabb37fa2ddc548c47`.
The final receipts identify `d43f4970d` with a dirty working tree because
they were built before the SQL changes were committed. Their engine source
is the source committed as `556e9180e`. No engine code changed after that build.

| Grammar | Before | After |
| --- | ---: | ---: |
| TypeScript | 3/4 | 4/4 |
| Bash | 3/4 | 4/4 |
| C++ | 3/4 | 4/4 |
| C# | 3/4 | 4/4 |
| Swift | 0/4 | 4/4 |
| Dart | 3/4 | 4/4 |
| SQL | 2/4 | 4/4 |

The same 707 selected fresh inputs and C tree digests were compared before
and after. Across 207 requested grammars, 187 had an available corpus and C
oracle; the same 20 were unavailable in both runs. Lean uses its repository
opt-in corpus and oracle locks. The default external corpus lock has SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea` and
is not committed here. Selection takes the four largest files at or below
1024 bytes, with the receipt's deterministic path order.

The C runtime is 0.25.1 at
`f5afe475deb7c0bae6407fb776c76824f717bb61`, with the 0.25.0 Go binding.
The original Swift grammar blob remains unchanged.

The shared causes and fixes are:

- C's physical GLR conflict order: exact-blob runtime grants fix TypeScript,
  C++, C# and Dart. Bash also needed the existing GSS convergence policy.
  Commit `cb733b155` adds the minimized conflicts and nearby C controls.
- External scanning during ERROR-mode lexing: `82906e564`, `88b860388` and
  `4730dfb93` retry the scanner from its saved checkpoint, consume the private
  retry flag, and preserve skipped newlines for zero-width tokens.
- Swift scanner state: `d43f4970d` restores C comment and directive behavior,
  implicit semicolons before directives, the exact-blob ERROR lex row and
  physical version ordering. Ordinary scanner rows are preserved.
- Missing-token recovery scheduling: `556e9180e` retains the first real
  lookahead boundary and lets a provisional missing version take its
  physical dispatch turn before competing in recovery. An exact SQL blob
  certification confines this change to the proven tables.

No engine language-name branch or gate threshold was added. Triage and
minimization use `cgo_harness/cmd/gts_mismatch` as documented in the harness
README. The resulting witnesses are represented in committed parity tests;
local triage receipts remain ignored artifacts.

The full sweep passes all 13,464 invariant steps for all 187 available
grammars: incremental Go equals fresh Go, ERROR roots report errors, roots
cover their inputs or explain the stop, and no-edit reparses allocate zero.
Focused Docker checks cover lexer retry state, zero-width token replay,
recovery and GLR units, exact/stale runtime certifications, 22 first-group C
witnesses and controls, 14 Swift C witnesses and controls, and six SQL C
witnesses and controls. Both production and candidate routes are exercised
by the new C regression tests. Heavy runs use one shared lock and one
container grammar at a time.

The final locked Docker run passes all 412 counter-ledger rows across 206
languages, Swift root and C regression tests, runtime certification tests,
and counter-ledger unit tests. No randomized timing comparison was run, so this
receipt claims correctness gains and deterministic work changes only.

Separate pin refreshes require owner review:

- `f96597518`: runtime profile census 56 to 57; C# pinned full/edit new nodes
  1936/2078 to 1268/1271 and Dart 3559/8259 to 2107/6008. Their fresh sets
  improve from 3/4 to 4/4. The 412-row ledger passed at that revision.
- `bae5f2556`: final pin refresh, SQL profile census 57 to 58; Swift known-error digests
  move with lower locked-C leaf distance (1007 to 764 on the 104681-byte
  witness, 28 to 17 and 124 to 95 on its 8192/16384-byte prefixes). The
  known structural gaps remain asserted. Swift's real-sample benchmark
  becomes C-exact, and Matlab processes previously dropped tokens.
  Matlab full/edit tokens rise 341/341 to 384/384, full/edit new nodes
  1699/1656 to 1945/1652, and peak versions fall 10 to 9. Its locked-C leaf
  distance falls 253/256 to 248/251; preorder node distance rises 332/341
  to 341/347, so the structural tradeoff remains explicit. Swift new nodes
  fall 280/305 to 276/295 and peak versions 3 to 1. Fixture identities,
  error/coverage checks and the 2% threshold are retained.

Rejected candidates include a global SQL scheduling grant: eight languages
moved outside the ledger tolerance and AWK edit peak versions rose from 5
to 6 without a tree improvement. The exact SQL grant keeps AWK at 5.
Clearing the recovery mark before the first real shift reduced SQL fresh
matches from 4/4 to 2/4. A replacement Swift blob passed the selected set but
introduced errors in four previously C-clean corpus controls, so the
original blob was restored. Wider GLR stacks fixed 0/11 original failures.

Follow-up triage shrank RON's first lost edit step from 360 to four bytes,
`d 1"`, and Dart's from 1025 to 20 bytes, `"x//\nimport system;\n'"`.
Each reduction preserves both the baseline's exact C match and the current
engine's C mismatch; the minimizers took 45 and 274 checks respectively.
RON's ERROR external row already enables all four tokens, and Dart's all
seven. Replacing either with another complete row fixed 0/1 tested failures.
Physical ordering, missing-version scheduling and convergence each fixed
0/1 of the tested RON failure. Token identity and external-lookahead
provenance candidates passed their synthetic controls but also fixed 0/1
of that C failure, so neither was adopted. The minimized RON trace instead
shows a masked scanner retry enabling string content after a rejected FLOAT
result. The native scanner's rejection remains the next mechanism to fix.

Fresh-C edit-session completeness improves from 38/187 to 39/187 grammars.
Passing steps gained: BitBake 69 and Swift 11. Passing steps lost: Dart 2
and RON 16, on unchanged inputs and C fresh digests. D8 still passes at every
step, so these are fresh-C recovery differences, not incremental/fresh Go
inequality. These losses remain open; this branch is a measured fresh-parity
increment, not a claim of complete edit-session parity. There are still
49 grammars with failing selected fresh sets and 148 with incomplete C edit
sessions across the available grammar set.

METRIC: seven target fresh matches | 17/28 -> 28/28 | 9148a96db..556e9180e | locked selected fresh sets
METRIC: complete fresh grammar passes | 131/187 -> 138/187 | 9148a96db..556e9180e | 207 requested, 20 unchanged unavailable
METRIC: complete fresh file matches | 575/707 -> 586/707 | 9148a96db..556e9180e | identical input and C tree digests
METRIC: invariant steps | 13464/13464 -> 13464/13464 | 9148a96db..556e9180e | 187 edit sessions
METRIC: no-edit allocations | 0 -> 0 | 9148a96db..556e9180e | all available grammars
METRIC: complete C edit-session passes | 38/187 -> 39/187 | 9148a96db..556e9180e | 80 step gains, 18 step losses
METRIC: counter ledger | 412/412 -> 412/412 | 556e9180e plus separate pins | 206 languages, both routes
METRIC: RON lost-step witness bytes | 360 -> 4 | 9148a96db..71839e2ad | baseline C match and current C mismatch retained
METRIC: Dart lost-step witness bytes | 1025 -> 20 | 9148a96db..71839e2ad | baseline C match and current C mismatch retained
