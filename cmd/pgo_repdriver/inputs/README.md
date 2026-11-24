# PGO input provenance

`production-v1.pgo` is the default profile that `pgo/default.pgo` held before 2026-07-18, collected with
`cmd/pgo_repdriver` over the lock-pinned Go, C#, Bash, Python, and CMake
corpora. Its SHA-256 is
`b55eb63bf103b1c53f7ff7defa3a7d77f40408924a641a5045a622e3b3dafcb3`
(171.57 seconds of CPU samples).

`selected-clean-error-v1.pgo` broadens the training surface. Its SHA-256 is
`0d1faade90a2fd4b307886c68f0cec98bce3ff859a28371d9c80ece123d02a9a`
(246.72 seconds of CPU samples). It is the deterministic merge of these
profiles:

| Class | Workload | Input identity | Profile SHA-256 |
| --- | --- | --- | --- |
| selected | `rewrite`, 10,000 iterations | 5,116 bytes; `74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097` | `4b49309ed66d32763f0530b4a80b29b9e23aa335c893bb8d8b151aa94ad37523` |
| selected | `query_compile`, 2,000 iterations | 20,168 bytes; `b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894` | `5c9580b3142c6d6c1f2f8cda961ef70c57b9261585f434c97fe4a7a1b11be7a0` |
| selected | `language`, 2,000 iterations | 41,387 bytes; `009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a` | `68f6fad4167c4006d47c8095a62382c87ecc262b9ff0cca230e1f31f1af9b3f6` |
| selected | `grammargen_lr`, 200 iterations | 235,626 bytes; `a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2` | `fb644fbd46c04d3f59743c0f7f4962bea88f8b5806b4b126c6b2ccf439cbf1e5` |
| clean | Go | `large__proc.go` (118,832 bytes; `b8abdfeecc9eb9a784d0931747b0f625b2831480a25b7a61d4974a3f62730c4a`), `medium__letter_test.go` (12,438; `38043063d5a3c67c5122a998382966b573197ac2fca0a1e8c5cd3aeaebb89c91`) | `647f54fc8f90266e7833e7ade48cdd4cee18ed805d03a421a44d357d400e2af4` |
| clean | Rust | `large__ast.rs` (66,281; `43fc2344174da29bb3c032b260a009828e4636965c1ab8cfff62b651caf91b92`), `medium__weird-exprs.rs` (6,436; `4968463f974c79afd641769a48d4e7e1c617b1acec03b5ccea3555b28def9dba`) | `0ae17fd7883fcbd38f2e68e1ab271a2652e04e14d8b5265ee440f5fba1313787` |
| clean | TypeScript | `large__parser.ts` (376,385; `e1429d9fa10832910568be5da7f38a6993db30c806a27e5375b02f95ccb51eb7`) | `c63a27f591c186ce688ee56f2ec225a6a27f91d72f00a211befdaf9ece76cd40` |
| clean | JavaScript | `large__jquery.js` (247,351; `140ff438eaaede046f1ceba27579d16dc980595709391873fa9bf74d7dbe53ac`), `large__text-editor-component.js` (151,531; `abe4f0e324382be35a6c11a659d3de1133017178f9a45113502f345cee00f76a`) | `2e1bc9c9e9d6bdba57ee2ff7e6634190f7826c6635d8528f0590c803110d2549` |
| error | Crystal | `src/lib_c/aarch64-linux-android/c/sys/sendfile.cr` (61; `162e7ba6347cfbf27054f4664bf486383d1a98a5e56e60f7a326f63672c48c61`), `src/lib_c/aarch64-darwin/c/sys/sendfile.cr` (69; `e741dedca6f917c856e0ab5eff493b9372606d063597c745c0161d78d1e0639e`) | `86f41d10164c2c0d47e7ae738b9aba41c93c97e58a45cab74b9fd210316e3bc4` |
| error | Matlab | `examples/code/@polynomial/numel.m` (79; `0f0b29e7a9f0c9f44ab7fb92437ec78516f3d2c68f79fd128604cf0356452a6e`), `test/nest/varg_nest2.m` (185; `98621e251bbf960e38e9ec762abe155675ba1764fce872937f07eea8d1cd6ee3`) | `c4ee0cd98d2fa5fcfbec284bf279133038367e2ab0ee54420838d6f334219cdf` |
| error | C# | `src/Bicep.Cli.IntegrationTests/Commands/DeployCommandTests.cs` (6,683; `a9adc0fde6724c8b9f73e4037a201d179729fa8db4cad4d4da7e4d88518eb218`), `src/Bicep.Cli.IntegrationTests/Commands/LocalDeployCommandTests.cs` (14,282; `9dfac34de5b374e0bb93c72b308860d621810dd27fb4c8b98942afd8a15b2654`) | `f8c0d58562d809a5b018df490b0b6265409838ad108341df8ab2fe1ff8d93194` |
| error | Scala | `project/PartestTestListener.scala` (4,653; `5bff3aea00f395040956b227d619194998324ff99fd10f50b96860eb36ed18a3`), `project/genprod.scala` (17,385; `080d72152961531da51546f62ab1c6233591c9aff9d70b84a88b2ca9711fc12b`) | `f2db12e304aeb2038a7b05a35ff2de74613231c23e20a9dde18c3649f677c48c` |

The selected profiles account for 126.9 seconds, clean profiles for 61.2
seconds, and accepted-error/retry profiles for 60.8 seconds. Every language
profile ran for approximately 15 seconds after its exact inputs were admitted
as complete and in the requested clean/error class. Regenerating either input
is a new training campaign: update this receipt, the hashes in
`compose_default.sh`, and rerun both performance gates before changing
`cmd/parity_report/parity_report.pgo`.
