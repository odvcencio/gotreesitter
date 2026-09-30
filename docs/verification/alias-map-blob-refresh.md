# Alias-map blob refresh verification

The refresh improves locked-C receipt parity from **217/292 to 222/292 files**, on both routes, with **zero regressions**. All **12,384 edit-session steps**, covering all 86 grammars on both routes, match fresh Go parsing before and after. The parser changes are ready for review; the pin refresh needs owner approval, and the existing registration, invariant, and RSS failures below remain visible.

Comparison: main snapshot `ab1a46f20199e47735895b431aafa250205df9e6` immediately before this lane, versus parser commit `14e4b7c09f1901153943c6c47986358464b993b5`. Main advanced during this work; comparing the original snapshot isolates the alias-map change. The corpus lock SHA-256 is `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`. Every sampled source hash was verified. The [machine-readable receipt](alias-map-blob-refresh.json) contains every file result, blob digest, changed counter row, benchmark sample, and the complete inventory additions.

## Root cause and changes

Alias-map support had landed without regenerating 86 of the 206 shipped blobs. Those maps tell the runtime to retain otherwise invisible wrapper nodes needed when applying nonterminal aliases; omitting them loses the string-content child in Teal and nodes present in C’s Org trees. Canonical re-encoding of all 86 old and refreshed blobs agrees after restoring the old alias maps, proving that the refresh changes only `NonTerminalAliasMap`. Raw old byte streams differ from today’s canonical encoder, so this is a serialized-field comparison.

The refresh also invalidated exact artifact identities. Regenerating 86 standalone package constants and updating 22 certified runtime profile hashes restores the same certified capabilities. Eight hash references in existing root, runtime, and C-parity tests and 86 hash-only fleet receipt entries now identify the refreshed artifacts. No engine code, capability grant, API, threshold, or table shape changed.

Added the Teal percent-content regression from the separate Teal lane and an integrity test covering all 56 runtime profiles. The Teal cases fail on the old main blob and pass on this branch. The integrity test detects 22 stale identities on the previously committed, unpinned refresh.

Teal’s alias-only blob remains `15ea5eb01e521bf93cb34fe18ca7d6ba9e23a13dc7d244fbed0cbb514c64dc17`. The separate full regeneration also changed recovery-cost and external-lexer fields; adopting it would expand this lane. Both percent-content cases pass with the narrower alias-only artifact.

Commits added during verification:

- `9fe1186b2` — profile identity and Teal regression tests.
- `14e4b7c09` — `update(pins): certify refreshed alias-map artifacts and wrapper counters`.

## Parity and counters

| Measure | Main snapshot | Refreshed branch |
| --- | ---: | ---: |
| Receipt files matching C on both routes | 217/292 | 222/292 |
| Org receipt files | 0/4 | 4/4 |
| Org minimized witnesses | 0/4 | 4/4 |
| Teal receipt files | 3/4 | 4/4 |
| Incremental/fresh mismatches | 0/11,088 | 0/11,088 |
| Fresh/C matches during receipt edit sessions | 2,704/11,088 | 2,792/11,088 |
| Supplemental incremental/fresh mismatches | 0/1,296 | 0/1,296 |
| Supplemental fresh/C matches | 392/1,296 | 536/1,296 |
| Fresh/C HasError agreements during all sessions | 11,660/12,384 | 11,660/12,384 |
| Counter ledger | 412 rows pass | 412 rows pass |

The five improved files are Org’s `testing/examples/ob-sed-test.org`, `ob-awk-test.org`, `normal.org`, and `links.org`, plus Teal’s `teal/metamethods.tl`. Nine refreshed grammars have no files in the sampled receipts: `capnp`, `cuda`, `dockerfile`, `doxygen`, `markdown_inline`, `norg`, `ssh_config`, `tmux`, and `yuck`. These receipts provide no C coverage for them. Separate locked-C checks and 72-step sessions on their pinned counter fixtures add 1,296 steps, all equal to fresh Go parses. Tmux’s fixture improves from mismatch to match on both routes; the others keep their prior result. Both Teal percent-content witnesses separately improve from C mismatch to match.

The Haskell compact decline changed only while its certified profile failed to attach to the new blob. Updating the exact hash restores the original decline: `shared (10115,577) live-link cap exceeded: 9 > 8`. Its decline string therefore stays pinned. Its default route materializes four more nodes, consistent with C retaining hidden aliasable wrappers.

Fourteen rows across ten grammars change node counts. Tokens, maximum live versions, reused bytes, and block splices are identical to the freshly measured main baseline in every changed row. The stored ledger already differed slightly from that baseline in Haskell and Markdown; the receipt separates stored pins from measured before/after values.

| Grammar | Route | Full-parse new nodes | Edit new nodes |
| --- | --- | ---: | ---: |
| djot | default | 10937 → 10944 | 10950 → 10957 |
| haskell | default | 1275 → 1279 | 1335 → 1339 |
| kdl | default | 9762 → 9784 | 9629 → 9651 |
| liquid | candidate | 179 → 203 | 41 → 54 |
| liquid | default | 255 → 289 | 203 → 237 |
| markdown | default | 1859 → 1887 | 3468 → 3521 |
| markdown_inline | default | 6369 → 6384 | 6378 → 6393 |
| rst | candidate | 1183 → 1267 | 1183 → 1267 |
| rst | default | 1171 → 1255 | 1183 → 1267 |
| ruby | candidate | 1027 → 1039 | 1027 → 1039 |
| ruby | default | 960 → 972 | 1027 → 1039 |
| tmux | default | 2432 → 2530 | 1316 → 1319 |
| vimdoc | candidate | 948 → 1169 | 105 → 127 |
| vimdoc | default | 943 → 1164 | 98 → 120 |

An instrumented walk of the locked C runtime’s internal subtrees confirms the hidden aliasable unary nodes: Djot 83, Haskell 76, KDL 954, Liquid 24, Markdown 56, Markdown Inline 13, RST 84, Ruby 12, Tmux 115, and Vimdoc 222. RST’s 84 and Ruby’s 12 exactly equal the added Go nodes; Liquid’s compact route adds exactly its 24 C wrappers. Public Go/C trees improve from mismatch to match for Liquid and Tmux. Markdown, RST, Ruby, and Vimdoc remain matching; the added work preserves hidden intermediates. Existing mismatches in Djot, Haskell, KDL, and Markdown Inline have unchanged visible tree digests.

The ledger threshold remains 2%. The correctness-related increases above that threshold are **NEEDS-APPROVAL**, recorded in the separate pin commit rather than excused by weakening a gate.

## Performance and memory

All eleven benchmark comparisons completed the required 20 paired, alternating shuffle seeds through `scripts/run_randomized_benchmarks.sh`, with one CPU, one count per process, 750 ms, and memory metrics. The ten counter-sensitive grammars use pinned real fixtures; Go uses the primary full/edit/no-edit trio. Host load was not controlled; confidence intervals and all samples remain in the JSON. This is verification of this refresh, not the full v1 graduation performance matrix.

| Grammar | Full median µs, before → after | Edit median µs, before → after | No-edit median ns, before → after |
| --- | ---: | ---: | ---: |
| djot | 8452.169 → 8422.607 (-0.35%) | 8523.453 → 8491.380 (-0.38%) | 7.062 → 7.059 (-0.04%) |
| go | 8417.751 → 8419.191 (+0.02%) | 179.244 → 179.305 (+0.03%) | 7.878 → 7.877 (-0.03%) |
| haskell | 1873.133 → 1925.233 (+2.78%) | 1877.411 → 1922.582 (+2.41%) | 9.082 → 10.125 (+11.49%) |
| kdl | 7290.644 → 7679.851 (+5.34%) | 16382.148 → 17678.091 (+7.91%) | 7.550 → 7.514 (-0.46%) |
| liquid | 240.330 → 234.883 (-2.27%) | 235.737 → 239.130 (+1.44%) | 7.778 → 7.176 (-7.75%) |
| markdown | 1760.701 → 1754.128 (-0.37%) | 3658.181 → 3627.802 (-0.83%) | 7.088 → 7.133 (+0.63%) |
| markdown_inline | 3631.238 → 3644.666 (+0.37%) | 3812.748 → 3658.559 (-4.04%) | 7.798 → 7.511 (-3.68%) |
| rst | 1646.934 → 1613.159 (-2.05%) | 1593.049 → 1533.421 (-3.74%) | 9.247 → 8.911 (-3.64%) |
| ruby | 641.605 → 621.319 (-3.16%) | 623.213 → 634.851 (+1.87%) | 7.675 → 7.665 (-0.14%) |
| tmux | 2266.756 → 2299.173 (+1.43%) | 4381.911 → 4418.087 (+0.83%) | 7.155 → 7.084 (-1.00%) |
| vimdoc | 666.194 → 694.209 (+4.21%) | 846.403 → 897.146 (+6.00%) | 7.022 → 7.405 (+5.44%) |

Vimdoc has statistically significant increases: full +4.21%, edit +6.00%, and no-edit +5.44%. Preserving C’s hidden wrappers adds work while its public tree still matches C. Its edit memory rises 10,073 → 10,185 B/op (+1.11%) and 37 → 38 allocations (+2.70%). Other allocation increases are Haskell full 151 → 152 and edit 157 → 158, KDL edit 545 → 546, and Tmux edit 1,100 → 1,101. The JSON records every B/op and allocs/op sample; all no-edit reparses remain at zero bytes and zero allocations.

The initial Haskell no-edit median rises 11.49% (9.082 → 10.125 ns), with wide intervals and p=0.277. A second 20-seed paired campaign measures 7.075 → 7.109 ns (+0.47%), with p=0.952 and zero allocations. Both campaigns remain in the receipt.

All full and edit median increases stay below 10%; the largest edit increase is KDL +7.91%, with p=0.678. The primary Go trio changes by +0.02%, +0.03%, and −0.03%, with unchanged allocations. Correctness and deterministic counters ran before these comparisons.

Memory-budget contract tests pass before and after. The large-file C checks accept on both routes without a crash or OOM. Liquid, Ruby, and Vimdoc match C before and after; existing RST and Tmux recovery mismatches remain. All source hashes match the pinned corpus. Whole-check peak RSS includes the C oracle and tree comparison; parsing alone is also measured separately, one grammar/route per process with `GOMAXPROCS=1`.

| Grammar | Source bytes | Whole-check RSS KiB, before → after | Parser default RSS KiB, before → after | Parser compact RSS KiB, before → after |
| --- | ---: | ---: | ---: | ---: |
| tmux | 99,777 | 87,576 → 91,188 | 37,852 → 38,916 | 67,148 → 67,524 |
| rst | 249,557 | 252,568 → 259,276 | 41,820 → 43,940 | 138,076 → 144,596 |
| vimdoc | 1,686,443 | 1,559,924 → 1,591,688 | 2,069,572 → 2,090,900 | 2,067,164 → 2,081,520 |
| liquid | 3,903 | 24,412 → 25,992 | 20,556 → 20,036 | 20,576 → 20,404 |
| ruby | 449,176 | 178,320 → 183,100 | 42,940 → 45,412 | 117,200 → 119,488 |

RSS changes in these probes remain below the 10% ratchet. Vimdoc’s large-file memory use is already high on main; a narrower witness uses exactly the first 1,048,576 bytes of the same locked file. It **fails the 400 bytes/source-byte RSS safety threshold on both revisions**, whose limit is 409,600 KiB.

| Vimdoc 1 MiB route | Main RSS KiB | Refreshed RSS KiB | Change |
| --- | ---: | ---: | ---: |
| default | 1,131,332 | 1,133,764 | +0.21% |
| compact | 1,136,684 | 1,137,044 | +0.03% |

Both 1 MiB parses accept, cover every byte, and report errors consistently with the larger malformed fixture. This existing RSS failure remains a release-safety issue; no threshold or exemption was changed.

## Gates and remaining failures

| Gate | Result |
| --- | --- |
| receipt comparison (both routes) | pass |
| receipt edit sessions | pass |
| org shrunk cases | pass |
| counter ledger | pass |
| certified runtime profiles and exact blob identities | pass |
| ts2go tests | pass |
| grammar package generator tests and generated-file check | pass |
| standalone affected grammar packages | pass |
| Teal, Org, native alias, blob-export regression tests | pass |
| certified grammargen alias-map tests | pass |
| synthetic grammargen alias-map tests | pass |
| known uncertified grammargen tests | existing skips |
| pinned C-parity tests for templ, php, julia, solidity, sql | pass |
| TestMergeEventCensusBaseline | pass |
| TestAdmissionCandidateRecursiveInsertionHaskellSmall | pass |
| TestRecoveryMemoTelemetryPreservesAMD64HotLayouts | pass |
| incremental probe ledger and no-edit allocation tests | pass |
| root ERROR/HasError and no-edit checks across affected grammars | pass |
| root go vet ./... | pass |
| v1layout | pass |
| v1guard | pass |
| incremental invariant ledger ratchet | pass |
| Shipped alias-map inventory | FAIL, registration coverage described below |
| F# root coverage | FAIL before and after |
| Fleet table-shape receipt | FAIL before and after |
| Optional C-parity module vet | FAIL before and after |
| Supplemental nine-grammar edit sessions | pass, 1,296 steps per version |
| Parser memory-budget contracts | pass before and after |
| Randomized benchmark comparisons | complete, 11 languages × 20 paired seeds |
| RSS change ratchet on measured files | pass, every increase below 10% |
| Vimdoc 1 MiB RSS safety witness | FAIL before and after |

The alias-map inventory expands from 21 to 107 nonempty maps. Its certified eight and known-uncertified three registrations do not change. Unregistered maps increase from 10 to 96: exactly the 86 refreshed grammars are added to `[arduino blade c_sharp d dart kotlin ocaml perl scala yaml]`. The machine-readable receipt lists all 86 additions. This registration gate was run and preserved, without adding certifications or skipping tests.

F#’s counter fixture begins with a UTF-8 BOM. Both versions return an accepted root covering `[3,2045)` on 2,045 bytes, so the strict coverage check fails identically; ERROR/HasError and zero-allocation no-edit checks still pass on both routes. The fleet receipt also fails identically on unrelated table drift in Angular, Blade, Erlang, OCaml, Perl, PowerShell, and R, plus pre-existing artifact drift outside the refreshed set. The 86 affected table shapes remain unchanged.

The required root-module `go vet ./...` passes. Optional C-parity-module vet retains five copy-lock diagnostics in untouched tests; its broad vet command also treats an intentionally incomplete structural parsing fixture as a Go package and reports undefined `Language`. Baseline checks confirm both failures.

Swift’s certified generator test first timed out at its existing 150-second limit with the default CPU setting. Retrying with `GOMAXPROCS=1` passed on both snapshots (130.21 seconds before, 122.91 after), without altering the timeout. Dhall, TSX, and TypeScript retain their existing explicit skips; this lane introduces none.

## Reproduction

Use the pinned receipt files and corpus identified by the source and lock digests in the JSON receipt. Build the C-parity tool with `GOWORK=off go build -tags treesitter_c_parity -o "$TOOL" ./cmd/gts_mismatch` inside the isolated C harness. For each refreshed grammar, run:

```sh
GOWORK=off "$TOOL" -mode receipt-check -grammar "$GRAMMAR" \
  -receipt "$RECEIPTS/$GRAMMAR.json" -corpus "$CORPUS"
```

Run the counter ledger with `GOWORK=off GOMAXPROCS=1 go run ./cmd/perfcounterledger`. For the benchmark helper, materialize `benchmarks.helper_source` from the JSON as `harness_out/alias-map-bench/bench_test.go` in both checkouts. For each of the ten moved grammars, run the repository wrapper with the same settings:

```sh
GOWORK=off GTS_ADMISSION_CANDIDATE=0 ALIAS_BENCH_LANGUAGE="$GRAMMAR" \
  bash scripts/run_randomized_benchmarks.sh \
  --output "$AFTER" --baseline-root "$BASELINE" --baseline-output "$BEFORE" \
  --package ./harness_out/alias-map-bench --tags gts_parsercorephase0 \
  --bench-regex '^Benchmark(Full|Edit|NoEdit)$' \
  --require-benchmarks BenchmarkFull,BenchmarkEdit,BenchmarkNoEdit
```

The wrapper defaults provide 20 alternating shuffled seeds, one CPU, one count per process, 750 ms, and memory metrics. Run the primary Go full/edit/no-edit trio through the same wrapper and compare with `benchstat`. Correctness checks ran separately from timing. Large-file resource probes use `/usr/bin/time -v` on the locked files listed in the JSON. The parser-only source is `parser_only_resources.helper_source`: materialize it as `harness_out/alias-map-memory/main.go`, build it with `GOWORK=off go build -o "$MEMORY_TOOL" ./harness_out/alias-map-memory`, then run `GOWORK=off GOMAXPROCS=1 /usr/bin/time -v "$MEMORY_TOOL" "$GRAMMAR" "$SOURCE" "$ROUTE"`. The 1 MiB witness takes `head -c 1048576` of the listed Vimdoc file; its source hash is in the receipt.
