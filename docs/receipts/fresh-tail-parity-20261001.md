# Fresh-C parity beyond the top 20

The 2026-10-01 continuation replay confirms four additional passing grammar
sets between `9148a96db` and `7e4bc159f`. It uses the four largest locked
matching files at most 1024 bytes per grammar, including the opt-in Lean
set. The denominator is 187 grammars with selected files; 20 have no
eligible set. This measures bounded fresh parity, not graduation.

METRIC: fresh-C passing grammars | 131/187 -> 135/187 | 9148a96db -> 7e4bc159f | compact route, four largest locked files at most 1024 bytes plus Lean

METRIC: fresh-C matching files | 575/707 -> 582/707 | 9148a96db -> 7e4bc159f | same pinned fresh selection

| Grammar | Matching files before | Matching files after | Fix |
| --- | ---: | ---: | --- |
| Haskell | 3/4 | 4/4 | Preserve structural scanner boundaries against GLR extras. |
| Common Lisp | 3/4 | 4/4 | Carry direct hidden-slot fields to anonymous children. |
| Teal | 3/4 | 4/4 | Restore missing nonterminal alias provenance. |
| Org | 0/4 | 4/4 | Restore missing nonterminal alias provenance. |

Every previously passing compact-route set remains passing. No other set
changes. Default mode passes 134/187 after these changes; Djot is the one
additional set that passes only with the compact route. The supplied
130/187 baseline was rebuilt as 131/187 in the compact-route receipts;
this receipt does not claim a measured default-route baseline.

All 707 source hashes and locked-C tree hashes match the baseline receipts.
The oracle is tree-sitter 0.25.1 at
`f5afe475deb7c0bae6407fb776c76824f717bb61`, with grammar revisions from
`grammars/languages.lock`. The main corpus-lock SHA256 is
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
The SHA256 of compact JSON containing sorted
`[grammar,path,source_sha256,c_tree_sha256]` rows is
`c7dfb174078832ab715d85fd4ac18e29429d7b26fef1af7b23e7cf81685bd736`.
The corpus lock itself is not included.

Local Docker validation passed the lexer arbitration and alias-wrapper
unit tests, both blob metadata regressions, and all 23 locked-C witnesses
on both parser routes. The replay ran one grammar per process with
`GOWORK=off`, `GOMAXPROCS=1`, and a shared heavy-work lock in a 3 GiB
container. There was no OOM or wall timeout.

The [counter-ledger CI job](https://github.com/odvcencio/gotreesitter/actions/runs/36890581294/job/110465627702)
passes at `7e4bc159f`: all 412 rows remain within the existing gates.
The fix commits record unchanged before/after values for all 412 rows.
Pinned default full tokens/new nodes remain Haskell `476/1275`, Common
Lisp `426/1284`, Teal `598/1385`, and Org `517/1291`.

NEEDS-APPROVAL: separate commit `7e4bc159f` refreshes Teal and Org blob
identities after C-required wrappers are restored. Teal alias rows change
`0 -> 12`, with SHA256
`f96a610bbd4fdb7eb391baebaebaf5b44e1574d5779d60d5ffca45af5d5356b7 -> b65de455d8a177042af1ad47c2fbfcb8b85e8027ff4b62c8d2397d6c0c5052bb`.
Org alias rows change `0 -> 4`, with SHA256
`d267e28494f48f83dd64da4d6c0651b5648e4d86dce3a513872aeabc525cc4c2 -> 67e67265d2b5a5531e9f984dbb220ab5322fd77501eaf616d18571a334f77dad`.
The other exported grammar fields remain unchanged.

The prior Haskell timing receipt reports Go/C `1.95x -> 2.05x`, with Go
time rising 3.7% (below 5%; benchstat p=0.265), and unchanged `1568 B/op`
and `14 allocs/op`. Its raw timing files were not committed, so local
timing reproduction remains outstanding. The broader Haskell R4
`Tree.Edit` timeout also reproduces on the untouched baseline.
