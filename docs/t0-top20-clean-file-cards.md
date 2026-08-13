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

### Card 7 parity residual and route-confirmed compact path

Card 7, Perl `dist/Module-CoreList/lib/Module/CoreList.pm`, did not produce a
complete locked-C card receipt. A route-only diagnostic classified the parse as
`compact`, with one routed parse and zero fallbacks. The full tree reached the
source end, but compact materialization emitted no retry or classic
parser-runtime counters.

A parity-only probe still found structural differences. Keep the card open for
parity. Do not treat the route-only diagnostic as a performance or no-memo
control.

The source has 1,266,636 bytes and SHA-256
`51ed1b05b1d76cdd9a350e9f839a87e0aa14061f8a1042d50fe1a776d459802c`.
The first parity divergence is
`/source_file/subroutine_declaration_statement[10]/block[2]/expression_statement[2]/conditional_expression[0]/ambiguous_function_call_expression[1]`.
Go selects `ambiguous_function_call_expression`; C selects
`equality_expression`. The probe found additional type and field differences.

The route-only diagnostic reported Go peak resident set size `229720064` bytes.
It reported no out-of-memory stop and no wall timeout. The full locked-C card
remains blocked by the structural mismatch above.

Diagnostic log:
`harness_out/docker/20260813T144602Z-t0-perl-corelist-parity-diagnostic-v1/container.log`.

Route diagnostic:
`harness_out/docker/20260813T152741Z-t0-perl-route-v2/container.log`.

### Card 10 parity residual

Card 10, Solidity `test/utils/Packing.t.sol`, is not evidence-complete. The
locked-C gate found a structural mismatch before runtime facts could be written.

The source has 44,712 bytes and SHA-256
`f634a75c5d44a38de450d9e8314ba4ccf727ad6a6eaf316aafc3c3b139e34cdc`.
The first divergence is
`/source_file/contract_declaration[5]/contract_body[4]/function_definition[58]/function_body[11]/statement[1]/expression_statement[0]/expression[0]/assignment_expression[0]/expression[2]/call_expression[0]`.
Go selects `call_expression`; C selects `type_cast_expression`. The mismatch
also changes the `uint8` node from an unnamed C token to a named Go node and
continues across many later expressions.

The Go digest was
`13e074c354a6c4e8d6488824b040631ba92f0f43bdcc09732946ae39a32412d2`.
The C digest was
`298f236b4ca4bd8d9ae50c75bb019e717c1a29c0365f41d401f575207f459b11`.
The run produced no out-of-memory stop and no wall timeout.

Diagnostic log:
`harness_out/docker/20260813T144816Z-t0-solidity-packing-parity-diagnostic-v1/container.log`.

### Card 11 compact-route receipt

Card 11, Common Lisp `src/code/external-formats/enc-cn-tbl.lisp`, now has a
route-aware T0 receipt. The child classified the parse as `compact`, with one
routed parse and zero fallbacks. It emitted no retry or classic parser-runtime
counters because compact materialization publishes acceptance and span fields
only.

The source has 959,740 bytes and SHA-256
`17da8956acc6d8402cee1a21b5d486fe4cda91a5de041a9c736843e8e527ce19`.
The Go deep-tree digest is
`df5bcbb96087a0082241894df136bd3c5cada82a89ed485f93a3414a971bf907`.
The Go tree spans bytes `0..959740` without a root error. The route-aware T0
run reported peak resident set size `276848640` bytes, no out-of-memory stop,
and no wall timeout.

The v2 receipt records gotreesitter revision
`924a52c42285fdeda20e43a56ef325c328501533`, child binary SHA-256
`c40382031eff3e6e4fe2ed341ab8e8026f5e624c35557a34e0b4cad294720e96`, and
receipt SHA-256
`ee348ff0f41924da36071130bbf84e25223398303aceb41625dd6e98a45bad60`.
The Go peak resident set size was `276848640` bytes. The locked-C peak was
`74964992` bytes, and the Go and C deep digests matched.

Diagnostic logs:

- T0: `harness_out/docker/20260813T145328Z-t0-commonlisp-enc-cn-card-v1/container.log`.
- Parity: `harness_out/docker/20260813T145445Z-t0-commonlisp-enc-cn-parity-diagnostic-v1/container.log`.
- Route-aware T0: `harness_out/docker/20260813T152505Z-t0-commonlisp-route-v2/container.log`.

Keep this card as compact-route evidence. Do not use it as a no-memo control or
grant performance credit.

### Card 12 runtime-facts control

Card 12, Requirements
`test/integration/targets/ansible-test-integration-constraints/ansible_collections/ns/col/tests/integration/requirements.txt`, has a complete evidence-only receipt. The source has nine bytes and SHA-256
`21d94dafd767c22de542deba74a9d2844e47054848a8116a2665619b5d5a8a39`.
The locked source is clean at commit
`fc5931c90beabea7b62747d2bc3038572c732f11`.

The Go and locked-C deep digest is
`7275ade5df87f05df55c8706333fc217f821fdf04ee9e35674301949cafdacf1`.
The Go tree spans `0..9` without an error or early stop. The compact route
served the parse with one routed event and zero fallbacks. No retry ran, and
the recovery memo tier stayed none.

The authenticated compact scheduler used 162,869 nanoseconds and the
materializer used 846,292 nanoseconds. Its retained footprint was 1,480 bytes.
The selected tree had three nodes, two parents, and one leaf. The compact
scratch receipt recorded 64 postorder frames. Go allocated 3,152,672 bytes and
reached 11,227,136 bytes of resident memory. The locked C oracle reached
188,416 bytes of resident memory.

The receipt is `harness_out/t0-requirements-integration-card-v1.json` with
SHA-256
`a9cca3758c7956eb3fb0f32225dd45d95d1bdc61c75224227b430bb975f97374`.
The Docker log SHA-256 is
`f4128e59b3625d28032dae324d1fc90067e222bbdabececadf65ad29273ddc05`.
This control grants no performance credit.

### Card 18 runtime-facts control

Card 18, Requirements
`test/integration/targets/ansible-test-units-constraints/ansible_collections/ns/col/tests/unit/requirements.txt`, has a complete evidence-only receipt. The source has nine bytes and SHA-256
`21d94dafd767c22de542deba74a9d2844e47054848a8116a2665619b5d5a8a39`.
The locked source is clean at commit
`fc5931c90beabea7b62747d2bc3038572c732f11`.

The Go and locked-C deep digest is
`7275ade5df87f05df55c8706333fc217f821fdf04ee9e35674301949cafdacf1`.
The Go tree spans `0..9` without an error or early stop. The compact route
served the parse with one routed event and zero fallbacks. No retry ran, and
the recovery memo tier stayed none.

The authenticated compact scheduler used 154,295 nanoseconds and the
materializer used 853,093 nanoseconds. Its retained footprint was 1,480 bytes.
The selected tree had three nodes, two parents, and one leaf. The compact
scratch receipt recorded 64 postorder frames. Go allocated 3,152,672 bytes and
reached 11,223,040 bytes of resident memory. The locked C oracle reached
77,824 bytes of resident memory.

The receipt is `harness_out/t0-requirements-units-card-v1.json` with SHA-256
`fe9d94dccaf94b32e0f1cbe5a11ed86ffe37dabbeb29737c71236774cc7d6a7c`.
The Docker log SHA-256 is
`0ec585dde9a1f3928dfe3a82c629c3595ac7dfec856a8668671156f141295422`.
This control grants no performance credit.

### Card 17 compact-admission residual

Card 17, Liquid `performance/tests/tribble/blog.liquid`, reached the
production fallback and matched the locked-C tree. The compact candidate did
not authenticate. The source has 1,133 bytes and SHA-256
`52be8daea35fc95da630dd2d5a2efe7bb9b539a7ddd0e89889ff57508193ed44`.
The locked source is clean at commit
`7b368dffb844c44a9466226d1a243b05aafc5be5`.

The route classified as `production_fallback`, with zero compact routes and
one fallback. The candidate declined because its accepted root spanned
`0..1133`, while the admission gate expected `2..1133`:
`accepted compact root is incomplete or erroneous: span=0..1133 expected=2..1133 error=false allowErrorRoot=false`.

The fallback Go tree and locked-C tree share deep digest
`ce7eff8f36209b0a954b34083dd38167a90121ddf059eaf4b9f2aca8e60f3937`.
The production tree spans `0..1133` without an error. The fallback run
allocated 21,492,016 bytes and reached 19,615,744 bytes of resident memory.
The locked C oracle reached 307,200 bytes of resident memory. These figures
are fallback measurements, not compact performance evidence.

The receipt is `harness_out/t0-liquid-tribble-card-v4.json` with SHA-256
`f534ac4c42ad65265897a67e6bb5ee80bdeffcc16031fc282e3d99e8149e74c1`.
The Docker log SHA-256 is
`337181da339c188d25f86f376cfdc9509f5ef915706114c7a58820203ed71c74`.

Keep this card open as a compact root-ownership residual. Resolve it with a
generic proof that retained children cover any leading source padding. Do not
relax the root gate without that proof. Grant no performance credit.

### Card 13 runtime-facts control

Card 13, TOML `.prettierrc.toml`, has a complete evidence-only receipt. The
source has 21 bytes and SHA-256
`75d82f31b7ec7968ad7534e0c6aaa73ed984f9cbd45c8448ea1a7a9372a7320f`.
The locked source is clean at commit
`b69253c93d3dec7ee7627c96159c2d3c753ed794`.

The Go and locked-C deep digest is
`b0261cee0d0c14524f861fb928fd82cb60b25dd28fe3a43f091622f4f6c67eb4`.
The Go tree spans `0..21` without an error or early stop. The compact route
served the parse with one routed event and zero fallbacks. No retry ran, and
the recovery memo tier stayed none.

The authenticated compact scheduler used 127,508 nanoseconds and the
materializer used 436,682 nanoseconds. Its retained footprint was 2,752 bytes.
The selected tree had seven nodes, three parents, and four leaves. The compact
scratch receipt recorded 4,096 scanner bytes and 64 postorder frames. Go
allocated 3,088,296 bytes and reached 10,911,744 bytes of resident memory.
The locked C oracle reached 188,416 bytes of resident memory.

The receipt is `harness_out/t0-toml-prettier-card-v1.json` with SHA-256
`5adb0e2d81336eccab2b7fe346b1cf9704b175de1bf3b08d547c76d094aba615`.
The Docker log SHA-256 is
`6cbb037a2be7cf1701d2d35e0c0ea002a1f191b9eadeb1c0a6d16b2a079f6a99`.
This control grants no performance credit.

### Card 14 parity residual

Card 14, Solidity `contracts/governance/Governor.sol`, reached the runtime
facts gate but failed the locked-C deep-tree identity. The run did not produce
an accepted machine receipt.

The source has 31,997 bytes and SHA-256
`658db74558ecc9e57dfde43216ce019867f45db775f016a15ebd8975ffa33e8d`.
The first divergence is
`/source_file/contract_declaration[16]/contract_body[17]/state_variable_declaration[5]/expression[5]/call_expression[0]`.
Go selects `call_expression`; C selects `type_cast_expression`. The mismatch
also changes `bytes32` and `uint8` from unnamed C tokens to named Go nodes and
continues across later expressions.

The Go digest was
`4db304a56ecf0e37c3908e3b984b7af3b5f05b2a12dc1527b6ee988283bf0a24`.
The C digest was
`351c94a2e8466d6bfb1e696aaf445cc95130c7cfa2f5517c7cc255b892b40b82`.
The run produced no out-of-memory stop and no wall timeout.

Diagnostic log:
`harness_out/docker/20260813T145657Z-t0-solidity-governor-parity-diagnostic-v1/container.log`.

Keep this card open as a correctness residual. Do not use it for mechanism
evidence or performance credit.

### Card 19 parity residual

Card 19, Enforce
`4_world/systems/inventory/dayzplayerinventory.c`, reached the runtime facts
gate but failed the locked-C deep-tree identity. The run did not produce an
accepted machine receipt.

The source has 108,051 bytes and SHA-256
`ee8ea290277a11312ce7c22289240fd0befbdf0cd1708930d59032b5a7b19cca`.
The first divergence is
`/compilation_unit/decl_class[2]/class_body[4]/decl_method[5]/block[6]`.
Both nodes span bytes `835..904`, but Go has four children and C has three.
Later blocks repeat the shape, including one block with 82 Go children and 52
C children.

The Go digest was
`de1caed4f2f8d82438321c47674cbe70d92ac506bbe30aff7d9ccd83b171844d`.
The C digest was
`0d3d6df4477c81508a9ed461e77cb941344f2ecea2c4d1f03d8bdfb0a1f802eb`.
The run produced no out-of-memory stop and no wall timeout.

Diagnostic log:
`harness_out/docker/20260813T150245Z-t0-enforce-dayzplayerinventory-parity-diagnostic-v1/container.log`.

Keep this card open as a correctness residual. Do not use it for mechanism
evidence or performance credit.

### Card 16 parity residual

Card 16, Enforce
`4_world/entities/manbase/dayzplayer/dayzplayercfgbase.c`, reached the
runtime facts gate but failed the locked-C deep-tree identity. The run did not
produce an accepted machine receipt.

The source has 194,135 bytes and SHA-256
`d6b3387228f21eb39f41e644ba7c209c851b057867462eafe1ed62abb14e51e3`.
The first divergence is `/compilation_unit/decl_method[4]/block[5]`. Both
nodes span bytes `402..498`, but Go has six children and C has four. Later
blocks show the same extra-child shape, with larger child-count gaps.

The Go digest was
`52646a14a7f81bbac9ea3c48036ea1c4f6bcb7953da3b32a276bfb2bc3b45cb9`.
The C digest was
`3d82e83cfd3ec32c6d7e92be3249602d8e0448820db10d375b35945dfb6ac9a1`.
The run produced no out-of-memory stop and no wall timeout.

Diagnostic log:
`harness_out/docker/20260813T150055Z-t0-enforce-dayzplayercfgbase-parity-diagnostic-v1/container.log`.

Keep this card open as a correctness residual. Do not use it for mechanism
evidence or performance credit.

### Card 15 parity residual

Card 15, Enforce `4_world/entities/dayzplayerimplement.c`, reached the runtime
facts gate but failed the locked-C deep-tree identity. The run did not produce
an accepted machine receipt.

The source has 104,908 bytes and SHA-256
`56f81ccc9289a1865d7b15f66ba4f15b4f97aa779bb69de74a3da4b813453665`.
The first divergence is
`/compilation_unit/decl_class[1]/class_body[4]/decl_method[3]/block[5]`.
Both nodes span bytes `280..319`, but Go has four children and C has three.
The same extra-child shape repeats across later blocks.

The Go digest was
`3df4cfbfc776577ba7aa39638436051ce7ce144a27fe79db5ca4f8f3d886e8f0`.
The C digest was
`0f104f635b7d2382441eae34fd39af4b199ade53015076814bfa10a8c0b1e98e`.
The run produced no out-of-memory stop and no wall timeout.

Diagnostic log:
`harness_out/docker/20260813T145903Z-t0-enforce-dayzplayerimplement-parity-diagnostic-v1/container.log`.

Keep this card open as a correctness residual. Do not use it for mechanism
evidence or performance credit.

### Card 20 parity residual

Card 20, PureScript `src/Data/EuclideanRing.purs`, reached the runtime facts
gate but failed the locked-C deep-tree identity. The run did not produce an
accepted machine receipt.

The source has 4,059 bytes and SHA-256
`15d83e118a173ae91fff5b12152afdacc549c12da0a541602d0cf5d702fae863`.
The first divergence is `/purescript/class_declaration[56]`. Go ends the node
at byte 3,311; C ends it at byte 3,309. Later expressions select `exp_apply`
in Go and `exp_name` in C, with named child nodes where C has unnamed tokens.

The Go digest was
`c65dd00ae3efe178a59cdca150986269662871eb65f2725370693dd7b9045c77`.
The C digest was
`80e5ee05c1a5fd2067390eaea5c2f45ff468171967c48d2aeb4ddcd736e94f75`.
The run produced no out-of-memory stop and no wall timeout.

Diagnostic log:
`harness_out/docker/20260813T150433Z-t0-purescript-euclideanring-parity-diagnostic-v1/container.log`.

Keep this card open as a correctness residual. Do not use it for mechanism
evidence or performance credit.

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

All seven previously pending T0 cards now have receipts. Cards 11, 12, 13, and
18 are parity-clean and route-confirmed. Card 7 still needs a route-aware
receipt.
Cards 7, 14, 15, 16, 19, and 20 are parity residuals.
Card 17 remains a compact-admission residual even though its production
fallback matches the locked-C tree.
Keep cards 4, 5, 6, 7, and 10 open as parity or route-facts residuals until
their exact tree differences are resolved or explicitly accepted as campaign
residuals.
Keep cards 8, 9, 12, 13, and 18 as hygiene or scale controls.
Use the B16 selected-rung telemetry for retry attribution.
Do not admit a performance change until the card matrix has a generic predicate.

The T0 collector records admission routed and fallback counts.
Treat a zero classic runtime record as compact-route evidence only after that
route marker is present. Do not infer route selection from zero counters alone.
