# Issue 1340: unfinished TOML pair recovery

The unfinished pair stays inside its TOML table and matches the locked C
runtime's fresh public tree, including byte/point spans and error flags.
The fix uses scanner and reduction properties; it adds no language-name
comparison to engine code.

The baseline is `1621f0014caa7f2422706fecc500388221a076d0`. The original
27-byte witness is `[session]\nvalue = 0\nhalf = `, with a trailing space.
The table witness shrinks to `[s]\na= ` (7 bytes); the scanner failure also
occurs with `a= ` (3 bytes). Removing the trailing padding changes C's tree,
so the tests retain the unpadded controls.

On the baseline, Go returned a table followed by an `ERROR` sibling. C
returned one table containing the unfinished pair and an integer with a
hidden missing terminal. Eight of the initial twelve route/witness checks
failed. The final regression covers eleven witnesses on both routes,
five edits on each route, and 32, 137, and 1024 KiB generated fixtures.
All match fresh C's public-tree digest. Every edit also matches fresh Go,
and every malformed witness reparses without edits with zero allocations.

## Cause and change

After the normal DFA failed, Go retried its internal error DFA without
first offering C's error-state symbol row to the external scanner. The
retry must start before skipped padding. Padding supplies byte progress
even when the resulting scanner token has an empty visible span. Without
padding or a scanner-state change, that empty token must be rejected.

Error mode offers the exact symbol row once. The normal scanner helper's
masked-symbol retries cannot be used here: they turned a rejected RON
float scan into string content. The RON regression now matches C, and the
first pinned VHDL edit improves from 262 to all 266 C nodes matching.

Reduction retains the padding span, the error flag of an empty public
node containing a hidden missing terminal, and that terminal's C error
cost. A single paused scanner version retains its original hidden layout
lookahead; re-lexing it as string content moved newline recovery outside
the table. Incremental roots with only hidden error evidence use the
existing fresh-result check.

## Deterministic counters

All 412 rows were collected before timing. Nine differ from the baseline.
TOML and requirements rows are unchanged. TOML full parsing consumes 167
tokens and constructs 278 nodes. Its candidate edit consumes 113 tokens,
constructs 185 nodes, and reuses 234 bytes. Its default edit consumes 23
tokens, constructs 45 nodes, reuses 1194 bytes, and performs 15 splices.

Only changed values appear below. `share` is multi-version token share in
parts per million. Both VHDL routes have the same edit-node change.

| Language/route | Phase | Before → after |
| --- | --- | --- |
| eds/default | full | nodes 6125 → 6127; share 0 → 265 |
| eds/default | edit | tokens 2593 → 2627; nodes 4806 → 4932; versions 1 → 2; share 0 → 380; reused bytes 9873 → 9664; splices 1028 → 1084 |
| kdl/default | full | nodes 9762 → 9773; share 605218 → 606352 |
| kdl/default | edit | nodes 9629 → 9640; share 605218 → 606352 |
| matlab/default | full | tokens 341 → 425; nodes 1699 → 2239; versions 10 → 9; share 454545 → 503529 |
| matlab/default | edit | tokens 341 → 425; nodes 1656 → 1979; versions 10 → 9; share 454545 → 503529 |
| prolog/candidate and default | edit | HasError false → true (fresh C: true) |
| promql/default | full | tokens 166 → 169; nodes 495 → 314; versions 6 → 3; share 469879 → 236686 |
| promql/default | edit | tokens 146 → 149; nodes 458 → 280; versions 6 → 3; share 534246 → 268456 |
| swift/default | full | tokens 124 → 126; nodes 280 → 291; share 24193 → 63492 |
| swift/default | edit | tokens 124 → 126; nodes 305 → 309; share 24193 → 63492 |
| vhdl/candidate and default | edit | nodes 454 → 455 |

The six rows requiring pin changes are EDS, MATLAB, both Prolog routes,
PromQL, and Swift. KDL and VHDL stay within the unchanged 2% gate. KDL's
public tree and pinned recovery digest are unchanged.

The changed-row audit compares public node signatures (type, spans,
fields, missing/error flags) with fresh C on the full fixture and first
pinned edit. All nine incremental results equal fresh Go.

| Language | Matching C nodes, full | Matching C nodes, edit | Explanation |
| --- | --- | --- | --- |
| eds | 4619 → 4622 / 4622 | 4619 → 4622 / 4622 | Both public trees become exact. |
| kdl | 270 → 270 / 272 | 270 → 270 / 272 | Extra recovery work stays below 2%; public tree unchanged. |
| matlab | 289 → 305 / 615 | 284 → 302 / 617 | Parses previously lost productions: 44 → 51 root children against C's 111; public nodes 478 → 542 against C's 615/617. Full parity remains incomplete. |
| prolog | 135 → 135 / 135 | 130 → 133 / 139 | Restores C's error flag. Edit parity remains incomplete. |
| promql | 127 → 134 / 155 | 127 → 134 / 155 | Removes surplus public nodes (195 → 181) and halves live versions. Full parity remains incomplete. |
| swift | 152 → 164 / 165 | 152 → 164 / 165 | Recovers more C productions. Full parity remains incomplete. |
| vhdl | 267 → 267 / 267 | 262 → 266 / 266 | Edit becomes exact. |

These correctness changes justify the separate pin refresh; they do not
waive the owner review required for a ledger exception. No gate threshold
or incremental-invariant allowance changes.

## Rejected candidates

- Retrying only the internal error DFA left eight of twelve witness checks
  failing. Fixing whitespace alone still failed both newline routes.
- Broad raw-error propagation raised requirements edit-node work from
  4411 to 8761 (98.6%) and Angular full-node work from 865 to 2188. The
  final change propagates a hidden missing terminal only into an empty
  public node; those rows return to baseline.
- Reusing normal masked-symbol retries reduced RON matching nodes from
  161 to 159. Restricting all retries to empty tokens instead reduced VHDL
  edit matches from 262 to 258. Offering the exact error row once preserves
  RON and makes the VHDL edit exact.

## Reference and verification

The C reference uses runtime 0.25.1 at
`f5afe475deb7c0bae6407fb776c76824f717bb61`, binding 0.25.0 at
`adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3`, and TOML grammar
`342d9be207c2dba869b9967124c679b5e6fd0ebe`. The cached grammar library's
SHA-256 is `fdac90bd6ff6fb0638fcd33e3562705157a6d905faabb8ab60de968bf2aa221d`.

The fetched corpus lock matches SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
It stays outside the repository. Its TOML corpus contains one 21-byte
file; the targeted malformed tests and generated fixtures supplement it.

Focused units, all new locked-C regressions, Python's 135 single-byte
deletions, token/node layout, reduction spans, hidden missing-token costs,
and the full root-package test passed in Docker with `GOWORK=off`.
The language-name guard reports the original 454 uses and 101 environment
reads; the layout guard reports 756 root files, 12 API tag sets, and no
test-file moves.
