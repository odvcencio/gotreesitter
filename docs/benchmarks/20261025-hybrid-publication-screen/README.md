# Held hybrid and publication screen

**HOLD.** The combined dense-hybrid and promotion-budget-region candidate improves generated Go full parsing, leaves real Go R4 inconclusive, and significantly regresses YAML R4. Nothing here promotes runtime code, graduates a language, validates Mac behavior or authorizes a main merge. The full-parse gain cannot be attributed to the budget cache alone.

The latest comparison used corrected production as baseline: 20 shuffled seeds per series, **60 pairs / 120 processes**, `GOMAXPROCS=1`, count 1, 750 ms, benchmem. It ran on Linux amd64 / Intel Core Ultra 9 285 / Go 1.26.6, in bounded network-disabled Docker pinned to CPU 6. Every R4 compact sample reported one compact route and zero fallbacks.

| Workload | Before → combined candidate | Change | p | allocs/op |
|---|---:|---:|---:|---:|
| Generated Go full | 8.779478 → 7.854259 ms | −10.54% | .006 | 22 → 22 |
| Go edit | 84.2985 → 81.7715 µs | −3.00%, inconclusive | .495 | 5 → 5 |
| Go no-edit | 3.7935 → 3.8215 ns | +0.74%, inconclusive | .529 | 0 → 0 |
| Real Go R4 compact | 924.7665 → 956.950 µs | +3.48%, inconclusive | .429 | 14 → 14 |
| YAML R4 compact | 211.721 → 230.601 µs | **+8.92%, regression** | <.001 | 39 → 39 |

Go/YAML R4 bytes remain 1,112/5,456 B per operation. Generated full bytes are 261,547→261,546 B; edit/no-edit are 412/0 B. The raw archive also retains inconclusive legacy controls and all unfavorable results. Earlier hybrid-only timing separately showed generated full −6.26% (p=.008), but real Go R4 **+6.29%** (p<.001); these separate runs do not isolate the cache's contribution.

Correctness and deterministic work preceded timing. The current gate passed 18 core controls, 10 region/snapshot families, four lifetime/ordinary-entry controls, 15 memory controls, 10 Go C families, eight YAML C families and four 72-step Go/YAML edit sessions with zero no-edit allocations. Diagnostic mode compared **561 live budget checks across 56 regions**, with no fallback or cached/full disagreement. Every original pre/post reservation and cancellation check remains.

Eight ordinary-entry pairs preserve full public output and work/board receipts. Candidate scanner, dependency and scheduler receipts match exactly; the intended graph-link proxy falls 15,505→2 on generated Go and 1,381→609 on real Go. Four actual Go/YAML default/candidate counter rows exactly match corrected production. Raw rows and validators are retained.

Supplemental **untimed** YAML off/on proof also passes output/scanner/dependency/semantic-work equality. Hybrid executes 12 batches / 193 actions / 65 elections / 20 authentic graph-anchor pops, with 12 entries, 11 reentries and 12 promotions. It records nine row-boundary refusals and three extra-payload crossings; graph links fall 304→168. This demonstrates real execution, while the timing regression remains held.

Prior hybrid and ordinary YAML CPU/allocation profiles are included for attribution. Hybrid R4 shows election 19.23%, native publication 15.18% and exact footprint accounting 9.31% cumulative. Unmodified YAML shows single-version execution 34.07%, election 27.14% and native projection 14.31%. These inclusive percentages overlap; profiler runs include calibration/warm calls and establish no speed comparison. Output continues through the existing native projection/capture path.

[summary.json](summary.json) and [source-identities.json](source-identities.json) identify the decisions and frozen inputs. [evidence.tar.gz](evidence.tar.gz) retains exact overlay backings/maps, source/control manifests, commands/workers/validators, raw statistics/logs, earlier unfavorable controls and profile data. The compressed [member index](contents.json.gz) authenticates each path; native executables, caches, credentials, private CSV and unrelated private files are excluded. All archived files have mode 0644. Frozen originals were not edited.

Run `sha256sum -c SHA256SUMS` and `python3 validate-evidence.py` here to validate the retained package without parser execution. Runtime replay requires the recorded pinned sources/dependencies and serialized Docker commands. Historical prepared manifests remain unchanged alongside their later terminal results. No new runtime tests or benchmarks were run while packaging.
