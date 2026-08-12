# T0 top-20 clean-file cards

Status: evidence specification

Use the accepted V10 epoch as the source.
This document adds no parser change and runs no new experiment.

## Locked source

- Epoch: `20260808T202958Z-v10-full-5003ffba`
- Go revision: `5003ffba01e2aee44043e71360c00f5aa93e6e8b`
- Scoreboard: `results/scoreboard/scoreboard.json`
- Corpus lock: `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`
- C runtime: `0.25.1@f5afe475deb7c0bae6407fb776c76824f717bb61`

All rows have a clean, full-span Go tree and an accepted full-parse result.
The table reports the measured file ratio and the existing memo tier.
The `Go ns/byte` value is a diagnostic scale indicator.

## Card matrix

| Card | Language | File | Bytes | Ratio | Go ns/byte | Memo tier | Current class |
| ---: | --- | --- | ---: | ---: | ---: | --- | --- |
| 1 | Python | `Lib/test/test_logging.py` | 276682 | 209 | 28674 | initial | memo candidate |
| 2 | Elixir | `lib/elixir/lib/enum.ex` | 154291 | 121 | 16274 | temporary | memo candidate |
| 3 | Python | `Lib/test/test_socket.py` | 288768 | 108 | 14544 | initial | memo candidate |
| 4 | Scala | `src/compiler/scala/tools/nsc/typechecker/Implicits.scala` | 103233 | 59 | 14336 | temporary | memo candidate |
| 5 | Scala | `src/compiler/scala/tools/nsc/typechecker/Namers.scala` | 102961 | 56 | 13342 | temporary | memo candidate |
| 6 | Elixir | `lib/elixir/lib/macro.ex` | 96618 | 43 | 6429 | temporary | memo candidate |
| 7 | Perl | `dist/Module-CoreList/lib/Module/CoreList.pm` | 1266636 | 41 | 6001 | none | large no-memo candidate |
| 8 | HTTP | `spec/examples/dotenv/with_dotenv.http` | 0 | 36 | 17542 | none | hygiene |
| 9 | HTTP | `spec/examples/dotenv/without_dotenv.http` | 0 | 33 | 16877 | none | hygiene |
| 10 | Solidity | `test/utils/Packing.t.sol` | 44712 | 31 | 4701 | initial | memo candidate |
| 11 | Common Lisp | `src/code/external-formats/enc-cn-tbl.lisp` | 959740 | 27 | 4891 | none | large no-memo candidate |
| 12 | Requirements | `test/integration/targets/ansible-test-integration-constraints/ansible_collections/ns/col/tests/integration/requirements.txt` | 9 | 24 | 3223 | none | tiny input |
| 13 | TOML | `.prettierrc.toml` | 21 | 23 | 2890 | none | tiny input |
| 14 | Solidity | `contracts/governance/Governor.sol` | 31997 | 23 | 1732 | initial | memo candidate |
| 15 | Enforce | `4_world/entities/dayzplayerimplement.c` | 104908 | 22 | 2551 | standard | memo candidate |
| 16 | Enforce | `4_world/entities/manbase/dayzplayer/dayzplayercfgbase.c` | 194135 | 22 | 8928 | standard | memo candidate |
| 17 | Liquid | `performance/tests/tribble/blog.liquid` | 1133 | 22 | 810 | initial | memo candidate |
| 18 | Requirements | `test/integration/targets/ansible-test-units-constraints/ansible_collections/ns/col/tests/unit/requirements.txt` | 9 | 22 | 3035 | none | tiny input |
| 19 | Enforce | `4_world/systems/inventory/dayzplayerinventory.c` | 108051 | 21 | 2947 | standard | memo candidate |
| 20 | PureScript | `src/Data/EuclideanRing.purs` | 4059 | 21 | 2790 | initial | memo candidate |

The memo cohort contains 13 rows.
The large no-memo cohort contains two rows.
The hygiene cohort contains two rows.
The tiny-input cohort contains three rows.

## Evidence gates

Do not infer generated status from a file path.
The V10 scoreboard does not record source-generation provenance.

Complete one card only when its receipt contains these facts:

- source provenance, with a manifest or repository proof;
- file size and source digest;
- locked-C and Go deep-tree identity;
- retry pass count and selected retry rung;
- peak memo tier, entries, bytes, and collisions;
- parser arena, scratch, and graph-stack allocation facts;
- materialization time and retained lifetime facts;
- maximum resident set size under the same process contract.

Keep hygiene and tiny-input rows out of mechanism credit.
Use the two large no-memo rows as negative controls.
Use the 13 memo rows as the first recovery-materialization cohort.

## Generated-file hypothesis

The current data supports a memo-bearing cohort.
It does not prove that generated files cause the scaling shape.

Test the hypothesis with a size series that preserves grammar and source class.
Compare at least three sizes for each selected source class.
Record whether memo tier, retry work, and materialization lifetime scale together.

Accept one generic mechanism only when the same predicate explains the cohort.
Reject the mechanism when large no-memo controls show the same shape.
Reject the mechanism when the result requires a language or file exception.

## Next bounded tranche

First collect the missing runtime facts for cards 1, 2, 4, 5, 6, 7, 10, 11, 14, 15, 16, 19, and 20.
Keep cards 8, 9, 12, 13, and 18 as hygiene or scale controls.
Use the B16 selected-rung telemetry for retry attribution.
Do not admit a performance change until the card matrix has a generic predicate.
