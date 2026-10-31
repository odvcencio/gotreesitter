# R7 cliff fixtures

These files pin the seven inputs from the gotreesitter v1 design, Workstream R7.
The detector decompresses each archive and verifies the source SHA-256 before parsing.
The archives use gzip with no timestamp. The listed digests name the decompressed bytes.

| File | Source and revision | License | SHA-256 |
| --- | --- | --- | --- |
| `c_sharp_generated_32k.cs.gz` | `cmd/issue454bench` generator at `c3ae5dc09bb5990120f473c66e29b06f7125ea3a`; run `c_sharp 32 emit` on this branch | MIT, gotreesitter | `b712fa3408c0d0c7d8ebd1c7919b47fe4a68281c0e61f5a7619a43004d6cdca9` |
| `elixir_generated_32k.ex.gz` | The same generator; run `elixir 32 emit` | MIT, gotreesitter | `a8178ccc477384b9d71b6ab9df31fb12e306282abde68b7db1b5b32b494f76d8` |
| `python_generated_32k.py.gz` | The same generator; run `python 32 emit` | MIT, gotreesitter | `685ae2d152c1585e22fb875780b4831a5df6e162a0e5fdf55a3cd2dc7608d4f9` |
| `go_parser.go.gz` | [golang/go `src/go/parser/parser.go`](https://github.com/golang/go/blob/go1.25.1/src/go/parser/parser.go), tag `go1.25.1` | BSD-3-Clause | `ad0a0a8fce883ab86ec3d2bf1d8d8dcc1072534bff46ab26c0796795f7b3087a` |
| `php_run-tests.php.gz` | [php/php-src `run-tests.php`](https://github.com/php/php-src/blob/d26068059e83fe40de3430a512471d194119bee0/run-tests.php), tag `php-8.3.0` | PHP-3.01 | `f19d07587177c12822d604717f2474b91b57a7d7d3a4ca97227e824bc32a6084` |
| `markdown_volumes.md.gz` | [kubernetes/website `volumes.md`](https://github.com/kubernetes/website/blob/a746bbd76e9504ecdedafbda7f7abfdbdee5642e/content/en/docs/concepts/storage/volumes.md), commit `a746bbd76e9504ecdedafbda7f7abfdbdee5642e` | CC-BY-4.0, The Kubernetes Authors | `12f2cd6d66512b683ecd00d0b32cff41b70709068f691c063043e342c88ce193` |
| `html_go_mem.html.gz` | [golang/go `doc/go_mem.html`](https://github.com/golang/go/blob/go1.25.1/doc/go_mem.html), tag `go1.25.1` | BSD-3-Clause | `561b50bca58e07455e993d68cf49281ed80df2150d09a38c77c906eabee36b7a` |

The design names `golang/website` as the source for `go_mem.html`.
That file is absent from `golang/website` at commit `f2661d967b28530da480f0a1da9a4279026d34ca`.
The file is present in `golang/go` at tag `go1.25.1`, so this fixture uses that source.

The generated fixtures reach or exceed 32 KiB because the generator writes complete records.
The copied files have no changes. Read the upstream [Go license](https://github.com/golang/go/blob/go1.25.1/LICENSE), [PHP license](https://github.com/php/php-src/blob/d26068059e83fe40de3430a512471d194119bee0/LICENSE), and [Kubernetes license](https://github.com/kubernetes/website/blob/a746bbd76e9504ecdedafbda7f7abfdbdee5642e/LICENSE).

## Top-20 coverage fixtures

The cliff report also covers every top-20 language, the first 20 names in `grammars/update_tier1_top50.txt`.
The nightly run on main then gives each engine pull request a "before" column.
`cliffCoverageFixtures` in `cgo_harness/cliff_fixture_table_test.go` lists each fixture with its grammar, source, size, and digest.

A language with no committed whole file uses two other inputs, which add no files here:

- Its counter-ledger sample, a small real file under `internal/benchfixtures/testdata/real` that `real_corpus.json` pins.
- A generated 32 KiB file from `benchfixtures.GeneratedSource`, which `generated.json` pins.

Eleven languages have no whole-file archive and use only these two inputs: bash, cpp, css, sql, c, rust, json, ruby, swift, kotlin, and dart.
Each sample is about 2 KB.
Those rows can show a regression, but they do not show whether a cliff exists on a large real file.
The editor-latency manifest has a whole file for only one of them, Bash, and its license rules it out (see below).

The nine archives below hold whole upstream files.
`cgo_harness/editor_latency/fixtures.json` pins the same bytes by repository, commit, path, size, and digest.
`TestCliffFixtureTable` checks each archive against that manifest.
PowerShell is a top-50 language, not a top-20 language. Its archive comes from the same manifest.

| File | Source and revision | License | Bytes | SHA-256 |
| --- | --- | --- | --- | --- |
| `typescript_extractType.ts.gz` | [microsoft/TypeScript `src/services/refactors/extractType.ts`](https://github.com/microsoft/TypeScript/blob/7539c04d94b5adc924efb3f8fef90e4de18d39d3/src/services/refactors/extractType.ts), commit `7539c04d94b5adc924efb3f8fef90e4de18d39d3` | Apache-2.0 | 19950 | `1c9771844ea117e1414365311fb181ca3a8fc58dcea06e761abd6d47c190a93f` |
| `tsx_openSourceAppList.tsx.gz` | [t3-oss/create-t3-app `www/src/components/docs/openSourceAppList.tsx`](https://github.com/t3-oss/create-t3-app/blob/4709861f7e67a15564c0460c13e7b4b6cfcae40d/www/src/components/docs/openSourceAppList.tsx), commit `4709861f7e67a15564c0460c13e7b4b6cfcae40d` | MIT | 11305 | `db25d3e10b36b102ce31dbd3eeb8de972c743a931ed9f1c5830a87a6ad318ec8` |
| `javascript_async_hooks.js.gz` | [nodejs/node `lib/internal/async_hooks.js`](https://github.com/nodejs/node/blob/45f6b8337e67d071a01336360f4905a0c6318b33/lib/internal/async_hooks.js), commit `45f6b8337e67d071a01336360f4905a0c6318b33` | MIT | 20575 | `c3ba587240b9a3bf529bf9f5b084dd8d210bb90e9627eaddc75d09bb36957ce9` |
| `python_locks.py.gz` | [python/cpython `Lib/asyncio/locks.py`](https://github.com/python/cpython/blob/e5ced1f7788e77e318165b331d967156f81d6709/Lib/asyncio/locks.py), commit `e5ced1f7788e77e318165b331d967156f81d6709` | PSF-2.0 | 20508 | `dd622639fdeeee94bd0164bb5dcd0d679a47a0a7f3bdb6094893834bc305f6b7` |
| `java_JdbcUtils.java.gz` | [spring-projects/spring-framework `spring-jdbc/src/main/java/org/springframework/jdbc/support/JdbcUtils.java`](https://github.com/spring-projects/spring-framework/blob/14f4deef3a5290f38ad46f53058a26730c3c1023/spring-jdbc/src/main/java/org/springframework/jdbc/support/JdbcUtils.java), commit `14f4deef3a5290f38ad46f53058a26730c3c1023` | Apache-2.0 | 20455 | `9b9efbdd7e6c1dd5c505bf3dd9e93d4247948a95833eb6f7edf5bbe897f25140` |
| `c_sharp_AzResourceTypeProviderTests.cs.gz` | [Azure/bicep `src/Bicep.Core.UnitTests/TypeSystem/Az/AzResourceTypeProviderTests.cs`](https://github.com/Azure/bicep/blob/1ec01291f20c9a813dfc2b112dd1d6df6c97ee16/src/Bicep.Core.UnitTests/TypeSystem/Az/AzResourceTypeProviderTests.cs), commit `1ec01291f20c9a813dfc2b112dd1d6df6c97ee16` | MIT | 20247 | `dbbae94b9dd80852bd58939f29492017d1b727c87b91f64bc3b362caaa6c9518` |
| `php_EditorDocument.php.gz` | [symfony/symfony `src/Symfony/Component/Tui/Widget/Editor/EditorDocument.php`](https://github.com/symfony/symfony/blob/121cf3b402e85e26cf1f57163186fc9193c95d21/src/Symfony/Component/Tui/Widget/Editor/EditorDocument.php), commit `121cf3b402e85e26cf1f57163186fc9193c95d21` | MIT | 20597 | `cd3f81810093827c3c5287c39e184250d45b55dce53801b3d4a0df000bfcc9c2` |
| `go_elf.go.gz` | [golang/go `src/debug/elf/elf.go`](https://github.com/golang/go/blob/d00c67f297ef6f2cb2cd0e9aae59fa3936bb7eca/src/debug/elf/elf.go), commit `d00c67f297ef6f2cb2cd0e9aae59fa3936bb7eca` | BSD-3-Clause | 140026 | `ff2f811d29a56303bad96c3c7cff1a83814a974503a51cee918a314a96eb476e` |
| `powershell_Get-Content.Tests.ps1.gz` | [PowerShell/PowerShell `test/powershell/Modules/Microsoft.PowerShell.Management/Get-Content.Tests.ps1`](https://github.com/PowerShell/PowerShell/blob/c23c421068dd4967765d8d658d4ba68eb649ef4f/test/powershell/Modules/Microsoft.PowerShell.Management/Get-Content.Tests.ps1), commit `c23c421068dd4967765d8d658d4ba68eb649ef4f` | MIT | 19441 | `7210a67d7bc3039211067e0a5a9a3731cb340477673fed80b0531d2c0a94c3e2` |

The copied files have no changes. Each archive is gzip level 9 with no file name and a zero timestamp:

```sh
python3 -I -c 'import gzip, sys; sys.stdout.buffer.write(gzip.compress(sys.stdin.buffer.read(), 9, mtime=0))' < upstream-file > archive.gz
```

Read the upstream licenses: [TypeScript](https://github.com/microsoft/TypeScript/blob/7539c04d94b5adc924efb3f8fef90e4de18d39d3/LICENSE.txt), [create-t3-app](https://github.com/t3-oss/create-t3-app/blob/4709861f7e67a15564c0460c13e7b4b6cfcae40d/LICENSE), [Node.js](https://github.com/nodejs/node/blob/45f6b8337e67d071a01336360f4905a0c6318b33/LICENSE), [CPython](https://github.com/python/cpython/blob/e5ced1f7788e77e318165b331d967156f81d6709/LICENSE), [Spring Framework](https://github.com/spring-projects/spring-framework/blob/14f4deef3a5290f38ad46f53058a26730c3c1023/LICENSE.txt), [Bicep](https://github.com/Azure/bicep/blob/1ec01291f20c9a813dfc2b112dd1d6df6c97ee16/LICENSE), [Symfony](https://github.com/symfony/symfony/blob/121cf3b402e85e26cf1f57163186fc9193c95d21/LICENSE), [Go](https://github.com/golang/go/blob/d00c67f297ef6f2cb2cd0e9aae59fa3936bb7eca/LICENSE), and [PowerShell](https://github.com/PowerShell/PowerShell/blob/c23c421068dd4967765d8d658d4ba68eb649ef4f/LICENSE.txt).

This directory does not hold the editor-latency Bash file, `t/t0003-attributes.sh` from git/git at commit `9ac3f193c05c2237e2b14ebaa1149e9fc8a1abe0`.
Its license is GPL-2.0-only, so the cliff report uses the Bash sample and a generated file instead.
To use that file, get the owner's approval for the license, add the archive here, and add a `cliffWholeFile` row.

## Reading a report

Each fixture writes one JSON report. It holds a `c` block for the C oracle and two Go routes, `legacy` and `compact`.
Read each field by its kind:

- Deterministic fields, which repeat exactly between runs: `tokens`, `new_nodes`, `frontier` (`max_live` and `multi_share`), `stop`, `has_error`, `root_end`, the tree hashes (`tree_sha256`), the tree-equals-C flags (`matches_locked_c_tree`), and the `decline` text.
- Approximate fields: `allocs_per_op`, `b_per_op`, and `max_rss_kib`. They change between identical runs. In local runs at 1 and 20 seeds, `allocs_per_op` differed in up to 31 of 114 measurements (the two Go routes and C, for each of 38 fixtures), by up to 11 allocations, and `b_per_op` by up to 4.5%. One 1-seed run reported a `php` legacy `b_per_op` 20 times the usual value. Do not use them as exact counters.
- Directional fields: `ns_per_op` and `go_c`. Timing evidence comes only from `scripts/run_randomized_benchmarks.sh` (AGENTS.md, section 3).
- Unmeasured rows: a route that took the forest fast path (`forest_fast_path` true) reports `frontier.measured` false, with zero tokens and nodes. A declined compact row has no frontier of its own and repeats the legacy tree.
- Full parses only: the report has no reuse counters, so an incremental pull request still needs the counter ledger (`internal/benchfixtures/counter_ledger.json`).
- Error-recovery rows: check `has_error` before you read a row's counters as a clean parse. Some rows have an `ERROR` on C itself, such as `cpp_sample` (the `FMT_TRY` and `FMT_CATCH` macros) and `html`. Others have an `ERROR` on a Go route while C is clean, such as `kotlin_sample`. Those rows measure error recovery, not clean parsing.
- The latest nightly: run `gh run list --workflow cliff-report.yml --event schedule --branch main --limit 1`, then `gh run download <id> -p 'cliff-report-nightly-*'`. A nightly turns red when a report is missing or the fixture table check fails, so do not read a red run as a baseline. Add `--status success` to the list command to skip red runs.
