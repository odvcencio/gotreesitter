# Incremental parity recovery-root receipt

Baseline: `5ff20d9`. The comparison uses the same locked C reference cache and the first 20 alphabetically ordered rcpt6 grammars whose original fresh parity passes and incremental receipt fails. Each session replays 72 edits on the file containing its first recorded failure.

The first-step classification is **19 fresh Go/C differences, zero Go incremental-only differences, and one C incremental/fresh difference (Elm)**. All 19 cold fresh mismatches are error-bearing recovery trees. CMake needs a separate oracle qualification below. Go incremental equals fresh Go on all 1,440 session steps both before and after.

## Classification and replay

Counts below are Go incremental versus fresh C matches out of 72, not whole-session receipt passes. `A` means a cold fresh Go parse already differs from C; `C` means fresh Go matches C but C incremental differs from C fresh. Every first failing step also has Go incremental equal to fresh Go.

| Grammar | First step | Corpus path and site | Class / first divergence | Main | Branch |
|---|---:|---|---|---:|---:|
| ada | 1 | `boards/native/config_src/adl_config.ads:site-00` | A / shape | 0 | 0 |
| astro | 32 | `packages/astro/test/fixtures/content-collection-references/src/pages/welcome.astro:site-07` | A / shape | 31 | 31 |
| awk | 7 | `bugs-fixed/subsep-overflow.awk:site-06` | A / shape | 6 | 6 |
| bass | 57 | `pkg/runtimes/testdata/addrs.bass:site-08` | A / type | 56 | 56 |
| bibtex | 38 | `tests-manual/extract-references/paper1/references.bib:site-13` | A / field | 37 | 37 |
| bicep | 40 | `src/Bicep.Core.Samples/Files/baselines/ResourcesSubscription_CRLF/main.bicep:site-15` | A / type | 39 | 39 |
| bitbake | 4 | `meta/recipes-extended/msmtp/msmtp_1.8.32.bb:site-03` | A / type | 3 | 3 |
| brightscript | 3 | `getting started/SceneGraphDeveloperExtensions-master/extensions/SGDEX/Views/OtherNodes/ItemComponents/StandardCategoryListItemComponent/StandardCategoryListItemComponent.brs:site-02` | A / shape | 47 | 47 |
| c | 6 | `sha256/gcrypt.h:site-05` | A / shape | 5 | 5 |
| circom | 14 | `circuits/sha256/main.circom:site-13` | A / shape | 13 | 13 |
| cmake | 16 | `Modules/Platform/Linux-NAG-Fortran.cmake:site-15` | A / shape; oracle state varies | 67 | 49 |
| corn | 16 | `assets/inputs/input.corn:site-15` | A / type | 23 | 65 |
| cpon | 2 | `examples/cli/minimalshvbroker/config/access.cpon:site-01` | A / shape | 1 | 1 |
| cue | 1 | `internal/ci/base/helpers.cue:site-00` | A / field | 0 | 0 |
| cylc | 2 | `tests/functional/cylc-kill/04-handlers/flow.cylc:site-01` | A / shape | 1 | 1 |
| devicetree | 9 | `dts/upstream/src/arm/nxp/lpc/lpc4350.dtsi:site-08` | A / type | 8 | 8 |
| dhall | 1 | `Prelude/JSON/tagNested.dhall:site-00` | A / range | 0 | 0 |
| dot | 1 | `tests/graphs/style.gv:site-00` | A / type | 8 | 72 |
| editorconfig | 65 | `.editorconfig:site-00` | A / shape | 64 | 64 |
| elm | 1 | `reactor/src/NotFound.elm:site-00` | C / C incremental oracle | 69 | 69 |

Raw Go/C matches: **478 → 566 / 1,440**. Strict steps requiring Go incremental = Go fresh = C fresh = C incremental: **466 → 562 / 1,440**. CMake changes from 67 to 49 Go/C matches and from 56 to 46 strict steps, but its C scanner varies with parser state; these changes are not credited to this fix. Excluding CMake, the reproducible gain is **411 → 517 / 1,368**, or **106 additional matching steps**. Corn gains 42 and Dot gains 64. Strict whole-session passes increase from **0 to 1 of 20** (Dot); CMake still has three C incremental/fresh differences. All **74 original fresh receipt files remain passing** on both Go routes. These results do not establish graduation of other grammars or a new global pass count.

## Confirmed cause and reductions

The classic recovery path authenticates an EOF `ERROR` root, but publication previously required zero children and a span exactly covering the source. Failing those restrictions adds a grammar-root wrapper, even when C accepts the bare recovery root. Retaining the existing receipt gate, after grammar-root reconstruction declines, fixes both child-bearing roots and roots with trivia outside their span. A short published root now reports `accepted_prefix` so its uncovered tail is explained.

`-mode min` reduces Corn and Dot to a single newline. A child-preserving reduction also reduces both to `}`. The four C-backed regression cases fail on main with the extra grammar-root wrapper and pass after the change on default and compact routes, including an insertion session and zero-allocation no-edit reparse. The EOF marker, existing grammar receipt set, and route-divergence exclusions are preserved.

Bass and Bicep also first diverge at an `ERROR` versus grammar root, but shrinking exposes an earlier recovery-election problem: an empty resynchronized grammar root wins before EOF-root publication. They are not fixed by this change. Minimal remaining examples are Bass `(w["""\n` (7 bytes) and Bicep `resources''=[for in{d:r` (23 bytes). Corn still differs at steps 58–64. Other first differences include recovery shape, missing-node handling, field propagation, and scanner token selection; shared root cause is not proved across those groups.

CMake cold and warm checks differ on the same edited text (`4ba1cc7029c772c3c96f77f933ccc76b78946377dfce3184d99274e462e45f97`), at a zero-width external bracket-content token. The pinned scanner allocates its state without initializing `token`, and zero-length deserialization initializes only `level`; stale token state is a plausible explanation, not a change made here. An instrumented 72-step replay confirms identical Go fresh and incremental digests on main and branch at every step, while C fresh digests differ at 46 steps. CMake also scored 72/72 Go/C in an earlier branch replay. Its raw count is reported, but changes are excluded from the fix's credited gain. The locked reference remains untouched.

## Dependency-walk fix

Running the affected-language invariant gates exposed a pre-existing PowerShell timeout: main and the initial branch both exceed 180 seconds in the edit dependency walk. The entry predicate recursively walks descendants that the node predicate already checked, doubling visits at every error-bearing ancestor. Clean non-missing nodes now compare their span directly, and ordinary error-bearing nodes use the completed descendant check; the special missing-node path retains its descendant check. A 40-ancestor regression times out after 5 seconds before the fix and passes in 0.004 seconds afterward. The PowerShell R4 gate now completes in 4.403 seconds in the final complete sweep.

## Deterministic counters and gates

`GOMAXPROCS=1 go run ./cmd/perfcounterledger` passes. Independently collected actual main and branch ledgers have **412 identical rows across 206 grammars**, with zero changed work or reuse counters. The pinned ledger is unmodified; pre-existing main-versus-pin differences are within its ratchets. Selected default-route edit counters:

| Grammar | Tokens main → branch | New nodes | Reused bytes | Block splices |
|---|---:|---:|---:|---:|
| Corn | 7 → 7 | 16 → 16 | 4 → 4 | 4 → 4 |
| Dot | 291 → 291 | 909 → 909 | 508 → 508 | 174 → 174 |
| PowerShell | 588 → 588 | 4120 → 4120 | 399 → 399 | 76 → 76 |
| Go | 574 → 574 | 3804 → 3804 | 1162 → 1162 | 294 → 294 |

Passing checks: focused root-building, root-span, missing-node dependency, tree-edit, both new regressions, recovery publication sweeps and route exclusions, pinned incremental probe/session/no-edit allocation suites, `TestMergeEventCensusBaseline`, `TestRecoveryMemoTelemetryPreservesAMD64HotLayouts`, `TestBuiltinRuntimeProfilesStayNarrow`, root-module `go vet ./...`, incremental-ledger ratchet script, and whitespace validation. C-backed checks run in isolation, one grammar at a time. Root Go file count stays 756; no production exported identifiers or layout fields are added.

The all-grammar R4 invariant sweep passes **202 of 206**. Awk (step 1), JavaScript (step 2), Meson (step 1), and Twig (step 4) fail with identical incremental/fresh digests on main and branch. These existing failures remain unresolved. The extra nested C-harness-module vet check fails on both main and branch on an invalid corpus example and existing lock-copy warnings; root-module vet passes. No pinned expectation is changed.

## Reproduction

With the locked receipt and corpus mounts available, build `go build -tags treesitter_c_parity -o /tmp/gm ./cmd/gts_mismatch` from `cgo_harness`. For each table row, strip `:site-NN` from the corpus path and run:

```sh
/tmp/gm -mode session -grammar "$grammar" -in "$corpus/$grammar/$file" -dump "$output/$grammar"
/tmp/gm -mode receipt-check -receipt "$receipts/$grammar.json" -corpus "$corpus"
/tmp/gm -mode check -grammar "$grammar" -in "$edited_text"
/tmp/gm -mode min -grammar "$grammar" -route compact -in "$edited_text"
```

The first edited text is reconstructed by applying session replacements through the table step. It is checked independently, rather than assuming original-file fresh parity also holds on edited text.

The final memory probe parses 26,000 generated Go functions (1,095,794 bytes) in an isolated process. Main peaks at **155,248 KiB RSS** and the branch at **155,368 KiB**, a **0.08% increase** and about **145 bytes per source byte**. Both complete without truncation or a memory-budget failure. This is a memory/completion probe, not timing comparison evidence.

## Randomized timing

The paired comparison uses `scripts/run_randomized_benchmarks.sh`, alternating main/branch order for seeds 1–20, `GOMAXPROCS=1`, one process per seed, `-count=1`, `-benchtime=750ms`, and `-benchmem`, without build tags. Timing uses Go 1.26.8 on linux/amd64; C and correctness checks use Go 1.25 in isolation. `benchstat` reports:

| Benchmark | Main | Branch | Difference | B/op main → branch | Allocs/op |
|---|---:|---:|---:|---:|---:|
| Go full DFA | 8.414 ms | 8.711 ms | +3.54% | 1275 → 1277 | 8 → 8 |
| Go single-byte edit DFA | 161.1 µs | 160.9 µs | no significant difference | 387 → 387 | 5 → 5 |
| Go no-edit DFA | 8.330 ns | 7.908 ns | −5.07% | 0 → 0 | 0 → 0 |

The full-parse timing and 0.16% bytes/op increases are directional regressions. They remain below the 10% ratchet, while work/reuse counters and allocation counts stay unchanged. The tradeoff is 106 corrected receipt steps and removal of the exponential error-tree edit walk. The source of the full-parse timing increase was not isolated by a CPU profile. This comparison does not replace the complete per-language E-A graduation gate; no language or default is graduated here.
