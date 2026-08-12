# T0 tail cards

This receipt closes the six R1 tail cards from the accepted V10 epoch.
It uses existing evidence only. It changes no parser code and runs no new experiment.

Source epoch: `20260808T202958Z-v10-full-5003ffba`
Source revision: `5003ffba01e2aee44043e71360c00f5aa93e6e8b`
Source scoreboard: `harness_out/gcp/20260808T202958Z-v10-full-5003ffba/gts-v10-full/results/scoreboard/scoreboard.json`

R1 counts the Scala 56–58x pair as one card. The table therefore has six cards.

| Card | Witnesses | Ratios | Disposition | First attributable mechanism |
|---:|---|---:|---|---|
| 1 | Python `Lib/test/test_logging.py` | 209.170723x | grouped with card 2 | Initial recovery-node memo; 128 entries and 3,072 bytes |
| 2 | Python `Lib/test/test_socket.py` | 108.118260x | grouped with card 1 | Initial recovery-node memo; 128 entries and 3,072 bytes |
| 3 | Elixir `lib/elixir/lib/enum.ex` | 121.410147x | grouped with card 4 | Temporary recovery-node memo; 262,144 entries and 6,291,456 bytes |
| 4 | Elixir `lib/elixir/lib/macro.ex` | 42.503260x | grouped with card 3 | Temporary recovery-node memo; 262,144 entries and 6,291,456 bytes |
| 5 | Scala `Implicits.scala`, `Namers.scala` | 58.538671x, 55.995092x | grouped | Temporary recovery-node memo; 262,144 entries and 6,291,456 bytes each |
| 6 | Perl `Module/CoreList.pm` | 40.899346x | not-actionable | No mechanism facts; R1 requires a size series |

## Interpretation

The Python, Elixir, and Scala cards form one recovery-memo mechanism cohort.
The evidence supports a grouped recovery candidate, not a parser change.

The Perl card remains not-actionable. Its single 1.2 MB witness cannot separate scale cost from parser, tree, or materialization cost.

No card qualifies as hygiene or fixed-candidate.
Named files remain witnesses. They are not code selectors.

## Top-20 clean-file census

This census ranks clean, full-span, measured rows by the existing full-parse ratio.
It uses the same V10 scoreboard and performs no new run.

| Rank | Language | File | Ratio | Bytes | Memo tier | Disposition |
|---:|---|---|---:|---:|---|---|
| 1 | Python | `Lib/test/test_logging.py` | 209.170723x | 276,682 | initial | recovery candidate |
| 2 | Elixir | `lib/elixir/lib/enum.ex` | 121.410147x | 154,291 | temporary | recovery candidate |
| 3 | Python | `Lib/test/test_socket.py` | 108.118260x | 288,768 | initial | recovery candidate |
| 4 | Scala | `src/compiler/scala/tools/nsc/typechecker/Implicits.scala` | 58.538671x | 103,233 | temporary | recovery candidate |
| 5 | Scala | `src/compiler/scala/tools/nsc/typechecker/Namers.scala` | 55.995092x | 102,961 | temporary | recovery candidate |
| 6 | Elixir | `lib/elixir/lib/macro.ex` | 42.503260x | 96,618 | temporary | recovery candidate |
| 7 | Perl | `dist/Module-CoreList/lib/Module/CoreList.pm` | 40.899346x | 1,266,636 | none | no mechanism fact |
| 8 | HTTP | `spec/examples/dotenv/with_dotenv.http` | 35.727088x | 0 | none | hygiene; exclude from ratio credit |
| 9 | HTTP | `spec/examples/dotenv/without_dotenv.http` | 32.834630x | 0 | none | hygiene; exclude from ratio credit |
| 10 | Solidity | `test/utils/Packing.t.sol` | 30.709653x | 44,712 | initial | recovery candidate |
| 11 | Common Lisp | `src/code/external-formats/enc-cn-tbl.lisp` | 27.175048x | 959,740 | none | no mechanism fact |
| 12 | Requirements | `test/integration/targets/ansible-test-integration-constraints/ansible_collections/ns/col/tests/integration/requirements.txt` | 24.254181x | 9 | none | tiny-input candidate |
| 13 | TOML | `.prettierrc.toml` | 23.040623x | 21 | none | tiny-input candidate |
| 14 | Solidity | `contracts/governance/Governor.sol` | 22.572329x | 31,997 | initial | recovery candidate |
| 15 | Enforce | `4_world/entities/dayzplayerimplement.c` | 22.170520x | 104,908 | standard | recovery candidate |
| 16 | Enforce | `4_world/entities/manbase/dayzplayer/dayzplayercfgbase.c` | 21.968405x | 194,135 | standard | recovery candidate |
| 17 | Liquid | `performance/tests/tribble/blog.liquid` | 21.734392x | 1,133 | initial | recovery candidate |
| 18 | Requirements | `test/integration/targets/ansible-test-units-constraints/ansible_collections/ns/col/tests/unit/requirements.txt` | 21.626286x | 9 | none | tiny-input candidate |
| 19 | Enforce | `4_world/systems/inventory/dayzplayerinventory.c` | 21.493946x | 108,051 | standard | recovery candidate |
| 20 | PureScript | `src/Data/EuclideanRing.purs` | 21.303608x | 4,059 | initial | recovery candidate |

The top 20 contains four evidence groups:

- Initial, temporary, or standard memo activity appears in 13 rows.
- Two large rows have no recovery memo fact.
- Two zero-byte HTTP rows fall below the 1,000 nanosecond hygiene threshold.
- Three small-input rows need a size series before mechanism attribution.

The memo-bearing rows support one recovery-materialization hypothesis.
They do not prove one implementation mechanism or justify a parser change.
The card matrix in `docs/t0-top20-clean-file-cards.md` defines the next evidence gate.

## Evidence boundary

All selected rows have `axes.full.status=ok`, clean full-span trees, and no root error.
The source rows and per-language receipts remain in the accepted V10 artifact.
The signed Hyphae receipt is `hypha-receipt:2026-08-12:codex-t0-tail-cards`.

The next step is T1 mechanism work after B8f and C0f gates close.
