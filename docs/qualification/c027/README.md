This is one sealed C 0.27 qualification baseline for all 207 artifacts: the 206 default language blobs and Lean. It records failures and unavailable inputs as well as passes. It does not graduate languages or change the parser default.

The measured Go revision is `cd785434b5608139d8e481455692f328b771a382`. The C runtime is Tree-sitter 0.27.0 at `6070dbfefd326bd735e5683eb128cc1b57dad0c0`, linked statically through the vendored binding. The C source manifest SHA-256 is `0912c8aa944b032e2bc37be9796a7d25a2f9623e788cc2fa275a6bb59adcbfe8`. Each receipt records its Go blob digest, C grammar commit and binary digest, compiler identity, source-file digests, route, and gate results.

[manifest.json](manifest.json) binds all 207 receipt hashes to the source revision. The [receipt archive](receipts-781c7b837cfa66683a8d302d4b7dd71a01c579671ba5a757940962b70bba9e1f.tar.gz) has SHA-256 `781c7b837cfa66683a8d302d4b7dd71a01c579671ba5a757940962b70bba9e1f`. Sealing the same receipt bytes twice produced identical archives and manifests. Extraction verified every receipt hash. The archive contains receipt JSON only; it contains neither corpus locks nor source bytes.

The workload selects up to four files per artifact, ordered largest first, with a 1024-byte file limit. Each available grammar runs 72 insert/delete/replace steps over 16 sites per edit class, then a no-edit allocation check. The corpus lock was fetched at run time and verified against `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`. Lean uses its existing separate lock with digest `be21bfe645441797853705719bbb5f7e76904af5f0e5f13c5fc32c1c558ee47c`.

The comparison revision is `086512373fa2e30649dc78ce2d8b7404c53b80f7`: the same C 0.27 runtime before the recovery-cost correction and receipt-axis split. Both runs used identical selected source files and edit inputs. The corpus manifest hashes also include run provenance, so those hashes differ between revisions.

| Metric | Before | Sealed baseline |
| --- | ---: | ---: |
| Sealed, authenticated C 0.27 baselines | 0 | 1 |
| Artifacts in that baseline | 0 | 207 |
| Fresh Go/C parity passes | 130/207 | 130/207 |
| Incremental parity passes | 36/207 | 36/207 |
| Both parity gates pass | 31/207 | 31/207 |
| Go invariant passes | 187/207 | 187/207 |
| Unavailable corpus samples | 20 | 20 |
| Timeouts / generator errors | 0 / 0 | 0 / 0 |
| Route probes: accepted / forest / declined | 191 / 11 / 5 | 191 / 11 / 5 |
| Pinned HTML structural witnesses | 9/10 | 10/10 |

[summary.md](summary.md) lists every artifact, route, decline details, parity axes, and unavailable reason. [run-metrics.json](run-metrics.json) contains aggregate results and generator accounting. There are 57 fresh-parity failures and 151 incremental-parity failures, in addition to the 20 unavailable samples. All 187 available grammars pass all 72 Go invariant steps, including incremental=fresh, root/error checks, and zero no-edit allocations: 13,464 edit steps in total.

No corpus files met the recorded selection for: arduino, beancount, capnp, cobol, cooklang, cuda, desktop, disassembly, dockerfile, dtd, eds, facility, hyprlang, json5, norg, promql, regex, ssh_config, tmux, and yuck. Their route probes and unavailable receipts remain in the archive.

Go's step gate now requires its invariants, Go incremental=fresh Go, and Go incremental=fresh C. C incremental=fresh C and Go incremental=C incremental are separate diagnostic axes. The baseline retains 165 C incremental/fresh disagreements. Thirteen steps pass the Go gate while preserving a C incremental disagreement. Missing fresh-C evidence still fails Go's C gate; missing C incremental evidence is diagnostic. Unit cases cover each condition.

C-oracle variability also matters. Three runs of each revision on identical CMake and RST inputs kept every Go fresh digest unchanged, but CMake's fresh C trees and RST's incremental C trees varied. CMake's matched-step counts were 49/15/15 before and 47/72/15 after. Those changes cannot establish a Go improvement or regression. The pinned CMake scanner allocates an uninitialized `token` field and clears only `level` on empty deserialization; this is consistent with the variability, but this run does not patch the locked grammar. See [validation.json](validation.json) for the per-run digest-change counts. The sealed baseline preserves the first full-fleet run, rather than selecting the rerun with more passes. CMake's edited-input oracle stability remains a qualification blocker.

The HTML correction excludes childless raw ERROR tokens from skipped-tree charges, as C does. On the 31-byte `html_log_7` witness, its recovered region costs 706 -> 606, below the competing missing-token lineage's 610. The Go digest becomes `ffca9ef006345898e5eb6cedcebf75a17908cffc62ff66d4926a0078bd01a7e0`, which passes the unchanged C structural assertion. Table lookup proxies rise 160 -> 168 (+5%); leaf constructions fall 13 -> 12; parent constructions stay 7. This lookup increase is a correctness tradeoff requiring owner review under D10. It is separate from the committed counter ledger, whose unchanged 2% gate passes.

The 206-language deterministic ledger covers 412 route rows. There are no work increases or reuse decreases. Only MATLAB's default-route node counts change: full parsing 1699 -> 1692, and its first edit 1656 -> 1649. The corrected C recovery-cost rule selects a different tree with less construction work. Its full structural-header distance to C remains 328 and its edited distance remains 337; no MATLAB parity gain is claimed. Its counter pins tighten in a separate update(pins) commit. The full [before ledger](counter-ledger-before.json.gz), [after ledger](counter-ledger-after.json.gz), hashes, and changed values are published in [validation.json](validation.json).

The required Go benchmark trio ran through `scripts/run_randomized_benchmarks.sh`: 20 explicit shuffle seeds, one process per seed, alternating revision order, GOMAXPROCS=1, one pinned CPU, count=1, benchtime=750ms, and benchmem. The input is the unchanged generated 500-function Go control (19,294 bytes). Raw samples are [go-before.txt](go-before.txt) and [go-after.txt](go-after.txt); [go-benchstat.txt](go-benchstat.txt) reports no significant change. Sample variability is substantial, so these results establish no speed win.

| Go control | Before ns/op | After ns/op | Before -> after B/op | Before -> after allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Full DFA | 40,131,252 | 39,064,218 | 1402.5 -> 1399 | 8 -> 8 |
| Single-byte edit | 828,936.5 | 795,948 | 403 -> 403 | 5 -> 5 |
| No edit | 35.365 | 34.855 | 0 -> 0 | 0 -> 0 |

A separate cold 27,000-function parse of 1,138,794 bytes completed with the default budget, without truncation or OOM. `/usr/bin/time -v` measured 162,044 KiB peak RSS, about 146 resident bytes per input byte. This single-operation run is a completion/RSS diagnostic, not timing comparison evidence. [validation.json](validation.json) records its source hash and allocations. Complete-operation Go/C performance and native-C allocation/RSS qualification across all 207 artifacts remain outside this receipt baseline; no performance ratio is claimed here.

Validation completed inside Docker, with each grammar processed separately: focused compact and legacy recovery-cost tests; all 20 pinned HTML/JavaScript/Swift witnesses; HTML fresh, incremental, error, highlight, and recovered-range parity; receipt-schema and seven axis cases; vendored C source authentication; cgo/standalone C deep parity on four frozen Go fixtures; C progress cancellation and the enabled static timeout canary; the C exact counter model; the 108-source/105-comparison derivation census; Go/Doxygen/Scala/Swift probe identities; HTTP/Robot EOF histories; all 207 receipt sessions; and the 412-row counter gate. Python sealer rejection tests and archive/hash reproducibility checks also pass. The recovery-reduction ordering candidate was rejected because HTML stayed at 9/10; the broader owned-recovery candidate declined before EOF and did not fix that witness. No gate threshold or allowlist changed.

To reproduce, use a clean checkout of the measured revision and the authenticated corpus checkouts. Fetch the corpus lock with `scripts/fetch_grammar_receipt_lock.sh "$GTS_CORPUS_LOCK_URL" <external-lock-path>`, then run `cgo_harness/grammar_receipts/run_all_in_docker.sh` with `GOWORK=off` and the documented corpus/report environment variables. Run `scripts/seal_grammar_receipts.py --report-dir <reports> --output-dir <sealed-output> --revision cd785434b5608139d8e481455692f328b771a382`. New runs have new timestamps and may expose oracle variability; resealing the preserved receipt bytes reproduces this archive exactly.

NEEDS-APPROVAL: the C runtime pin, C witness/census/counter expectation refreshes, MATLAB's tighter counter pins, this published baseline, and the HTML witness's +5% lookup tradeoff require owner review. The 2% ledger threshold is unchanged. The supplied overseer receipt index was unavailable on this VM, so the comparison was rebuilt from the clean pre-fix revision using the verified existing corpus and compiler-addressed grammar cache. No corpus lock is committed.
