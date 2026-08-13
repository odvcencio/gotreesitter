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

## Completed runtime-facts cards

Three evidence-only cards now have complete runtime-facts receipts:

- Card 1, Python `Lib/test/test_logging.py`:
  `hypha-receipt:2026-08-13:t0-python-runtime-facts-receipt-v1`.
- Card 2, Elixir `lib/elixir/lib/enum.ex`:
  `hypha-receipt:2026-08-13:t0-elixir-runtime-facts-v1`.
- Card 3, Python `Lib/test/test_socket.py`:
  `hypha-receipt:2026-08-13:t0-python-socket-runtime-facts-v1`.

All three cards match the locked C deep-tree identity. No card grants
performance credit or proves a shared runtime mechanism.

### Card 3 runtime facts

The card ran at gotreesitter revision `90a872833706fcdf83685e66b44ecf2a243ac411`.
The source has 288,768 bytes and SHA-256
`5691e8a4e5186f74bd2e49babbab447b95a876806c3b7aebf3d4ff62a74ca896`.
The Python source checkout is clean at commit
`e5ced1f7788e77e318165b331d967156f81d6709`.

The initial full parse was accepted on one attempt. The Go and locked-C deep
digest is `69af657e9d86bd96f78c461c7c3d9a12345a9b61b268aa661ac93722b89c1745`.
The peak memo tier was initial, with 128 entries, 3,072 bytes, and zero
collisions. The arena used 41,275,152 bytes, scratch used 19,552,480 bytes,
and the graph stack used 14,700,576 bytes across three slabs.

The parser used 52,783 tokens, 55,454 iterations, and 201,094 allocated nodes.
Materialization timing was 11,795,575 nanoseconds. The tree observation lasted
12,105,167 nanoseconds. Go peak resident set size was 79,966,208 bytes, and C
peak resident set size was 26,701,824 bytes.

The machine receipt is
`harness_out/t0-runtime-facts/python-socket-v1.json` with SHA-256
`02f6017ac8e22fd5aad779b44c84830e52686470c72e91a056bdfa7bdd16977b`.
The Docker log SHA-256 is
`d416a5523c0088d5330b3e73070d2574c763f4942c59bcff1b491b3d9ec6a883`.

### Card 4 parity residual

Card 4, Scala `src/compiler/scala/tools/nsc/typechecker/Implicits.scala`, is
not evidence-complete. The locked-C gate found a structural mismatch before
runtime facts could be written.

The source has 103,233 bytes and SHA-256
`4c59df9daeb021cd4fcc61a8ac7a542087dd1a329b34c7e8e8483f0a56de684d`.
The first divergence is
`/compilation_unit/trait_definition[17]/template_body[3]`.
Both trees use `template_body` over bytes `1117..101110`, but Go has 12
children and C has 70 children. The run produced no out-of-memory stop and no
wall timeout.

The Go digest was
`526ee3399a6a519fe211372dc10afac05c81d53155bc238e83f4d49c1397ed18`.
The C digest was
`97515a48635aa1bc9c1fe9548b1a5a6b3d46e1d7fd74c3e2462feff484734786`.
Keep this card open as a correctness residual. Do not use it for performance
credit or generic mechanism inference.

Diagnostic log:
`harness_out/docker/20260813T143924Z-t0-scala-implicits-parity-diagnostic-v2/container.log`.

### Card 6 parity residual

Card 6, Elixir `lib/elixir/lib/macro.ex`, is also not evidence-complete. The
locked-C gate found a structural mismatch before runtime facts could be written.

The source has 96,618 bytes and SHA-256
`c1be1fd45a151e14a1602bcb6a1d23416113e7d2b8d629e9a8f5264a4999ef34`.
The first divergence is
`/source/call[4]/do_block[2]/call[296]/do_block[2]/call[1]/do_block[2]/stab_clause[1]`.
The Go end byte is 61,513, while C ends at 61,538. The Go tree has one body
child where C has two, and it selects `access_call` where C selects `list`.
The run also found a later span divergence near byte 90,698.

The Go digest was
`4cd3dbd2b895e93f70f080f8d5368d5ee7550489697d67419d441a59e33ad516`.
The C digest was
`b73a755643300948f1c128f99d89bd9df20196c9a1359e1704b95662854b3fd8`.
The run produced no out-of-memory stop and no wall timeout.

Diagnostic log:
`harness_out/docker/20260813T144133Z-t0-elixir-macro-parity-diagnostic-v1/container.log`.

### Card 5 parity residual

Card 5, Scala `src/compiler/scala/tools/nsc/typechecker/Namers.scala`, is not
evidence-complete. The locked-C gate found a structural mismatch before
runtime facts could be written.

The source has 102,961 bytes and SHA-256
`d5ca0021418ad1bc6ed342ae56e6f771538afec293a1c535743676cb509fa83d`.
The first divergence is at `/compilation_unit`: Go reports no root error,
while C reports an error. The template body at `root[10][3]` has 34 Go
children and 37 C children over the same bytes `700..102960`.

The Go digest was
`6578d87e6672aa3ae2b84b27e8ed5a77a5fd7193b67155dde710330d171ec376`.
The C digest was
`4650e21b2ad3b776eaad5b846b3c4de49cf122ac23428094625c17f8083eb1f3`.
The run produced no out-of-memory stop and no wall timeout.

Diagnostic log:
`harness_out/docker/20260813T144335Z-t0-scala-namers-parity-diagnostic-v1/container.log`.

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

First collect the missing runtime facts for cards 7, 10, 11, 14, 15, 16, 19, and 20.
Keep cards 4, 5, and 6 open as parity residuals until their exact tree differences
are resolved or explicitly accepted as campaign residuals.
Keep cards 8, 9, 12, 13, and 18 as hygiene or scale controls.
Use the B16 selected-rung telemetry for retry attribution.
Do not admit a performance change until the card matrix has a generic predicate.
