# Compact route real-corpus matrix

Current evidence date: 2026-08-22.
Current base commit: `f298328a` from `main`.
Current candidate base commit: `f298328a`.

## C26a Swift issue #576 token-production blocker

Receipt base: `8751e3684542134cec16f56771b6130cbde1ad8f`.

Status: **NO-GO / KEEP LIVE**. This receipt does not change compact route
admission, D6 status, or the bounded matrix counts. Keep issue #576 open.

The smallest existing witness is the 20-byte source `let x = unsafe bar()`.
Its source SHA-256 digest is
`b511d81ace2a89b05e8e5e0ca6730c10f2ac9295111dae013097c7c6be8861fe`.
The Go deep-tree digest is
`860b79483c37e217690deae43036bada15b259bed77713606124fa851702e62f`.
The locked C deep-tree digest is
`c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68`.

The first divergence is
`/source_file/property_declaration[0]/call_expression[3]/ERROR[1]`, bytes
15..18. Go emits a childless `ERROR` node. Locked C emits an `ERROR` node with
the `bar` child. The two existing Swift corpus witnesses share this unsafe
expression-prefix cause.

The locked C reference parser uses grammar version `0.7.2`, grammar commit
`41d6e5fe811ec94229ee71771174a8cce558dfee`, runtime `0.25.1`, and runtime
commit `f5afe475deb7c0bae6407fb776c76824f717bb61`. Its lexer skips `bar`,
emits an error token of size four, detects the error, and resumes through the
previous recovery point. Go emits identifier symbol `160`, reaches state 47
without a parser action, and enters generic recovery. Go therefore does not
receive the C error token before it materializes the childless `ERROR` node.

Canopy attributes the generic C error-mode acquisition to
`cRecoverAcquireToken` in `parser_recover_c.go`. The
`cRecoverResumeLookahead` path serves custom source fallback. The
`pushLexErrorRunLeaf` path owns the C-shaped error wrapper. Those paths cannot
restore a token that the deterministic finite automaton (DFA) token source did
not produce. The divergence is therefore in token production and grammar-table
behavior, before generic error materialization. The direct structural queries
were:

- `scripts/canopy_query.sh search symbols parser_recover_c.go --limit 120 --no-cache`
- `canopy graph calls 'pushLexErrorRunLeaf' parser_reduce.go --no-cache --depth 3`
- `canopy graph calls 'cRecoverAcquireToken' parser_recover_c.go --no-cache --depth 3`

The isolated experiment tried a grammar-agnostic predicate. When normal
lookahead had no parser action, it relexed from broad error mode before
recovery. The prototype touched only `parser.go` and
`parser_dfa_token_source.go`. The focused Swift run kept both deep-tree
digests and the first divergence unchanged. The prototype was removed. No
production or test change survives.

The focused Docker artifacts are:

- `/tmp/gts-c26a-artifacts/20260823T021728Z-swift576-minimal`
- `/tmp/gts-c26a-artifacts/20260823T021757Z-swift576-corpus`
- `/tmp/gts-c26a-artifacts/20260823T022322Z-swift576-predicate2`

An upstream regeneration also left the witness unchanged. It produced blob
digest `be5cd0bf8df7077804fe4b54ee47d76005c9a85c7c33b857ef6d2aff34461286`.
The shipped Go Swift blob is
`be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582`.
The upstream probe used revision
`172ada1cc4117d0260d9340680b4134adba2bc2c`, package version `0.7.3`, and
these artifacts:

- `/tmp/gts-swift-upstream-probe-artifacts/20260823T000638Z-swift-upstream-issue576-range`
- `/tmp/gts-swift-upstream-probe-artifacts/20260823T000433Z-swift-upstream-issue576-parity`

Reopen implementation work only after an authoritative grammar or lexer
revision changes token production for the `unsafe` expression prefix, or a
generic runtime change produces the locked C error token without a Swift
specific rule. Then run the 20-byte witness and one corpus witness in separate
Docker parity runs. Keep issue #576 open until both witnesses pass.

## Current bounded result

The bounded matrix completed with no silent divergence.

| Status | Files |
|---|---:|
| PASS | 70 |
| FALLBACK | 30 |
| SKIP | 10 |
| DIVERGE | 0 |
| ERROR | 0 |
| Total | 110 |

The corpus manifest contains 147 verified files across 50 languages.
This run selected files smaller than 16,384 bytes and excluded AWK.
The AWK medium file needs a separate slow-path budget.

The direct route served 64 percent of the selected files.
Production served every fallback and every ineligible file.

## D6a producer evidence

D6a publishes an authenticated drop-cohort frontier with the feature disabled
by default. It does not enable route admission, frontier history, or frontier
verification. The bounded matrix counts above remain unchanged because D6a is
producer-only.

The isolated Docker targets accepted these published and member counts:

| Grammar | Target | Published/member | Route | Natural fallback |
|---|---|---:|---:|---|
| Go | `query_compile` | 2/6 | 0/1 | Unchanged |
| Go | `rewrite` | 2/6 | 0/1 | Unchanged |
| Go | `language` | 2/2 | 0/1 | Unchanged |
| Go | `grammargen_lr` | 2/2 | 0/1 | Unchanged |
| Erlang | `macro_function_clauses` | 8/26 | 0/1 | Unchanged |
| Erlang | `macro_expanded_top_level_function` | 3/6 | 0/1 | Unchanged |
| Haskell | `smoke` | 2/2 | 0/1 | Unchanged |
| JavaScript | `functions` | 2/8 | 0/1 | Unchanged |
| Bash | `converged_split` | 4/24 | 0/1 | Unchanged |

The gate fixed complete frontier byte accounting and stale-token transaction
poisoning.
It ran 10 internal tests. The root and no-core gates passed.
The final Docker artifacts span `20260822T065000Z` through
`20260822T065433Z`.

### Interleaved performance receipt

The accepted receipt uses 20 interleaved seeds and the primary benchmark trio.

| Benchmark | Candidate versus base | Probability |
|---|---:|---:|
| Full parse | -3.01% | `p=0.678` |
| Incremental single-byte edit | -1.09% | `p=0.758` |
| Incremental no-edit | +7.34% | `p=0.910` |
| Geomean | +0.97% | — |

Full-parse bytes per operation changed by -3.10 percent.
Allocation counts remained unchanged.
Discard the earlier sequential full-set comparison because unrelated host
tests overlapped the candidate run.

This receipt does not graduate the route and does not verify D6b.

## D6b internal-consumer evidence

D6b adds a default-off authenticated internal consumer for complete frontiers.
It does not activate the route driver, admission, or production drop wiring.

The driver verifier collects exact current heads and references. It requires
one common nonzero frontier sequence, rebuilds the frontier token, and consumes
the authenticated frontier immediately before each of the three drop sites.
Zero or mixed sequences fail closed. The verifier remains default-off, and the
route remains ungraduated.

The consumer enforces these contracts:

- Authenticate the owner, epoch, election, token, frontier seal, and ordered records.
- Reject producer-cap violations, blended references, malformed offsets, and mismatched reference sets.
- Require one survivor member to match every dropped participant within one common cohort.
- Match the exact survivor reference and compare action and full derivation identity, metadata, and bytes.
- Journal the consumed state only after proof succeeds, then restore it through checkpoint rollback.

The focused Docker gate results are:

| Target | Result | Artifact |
|---|---|---|
| `TestG18D6b` | PASS, exit 0 | `harness_out/docker/20260822T101332Z-d6b-proof-tests-v4` |
| `TestG18DropCohortFrontier` plus `TestG18D6b` | PASS, exit 0 | `harness_out/docker/20260822T101347Z-d6b-frontier-regression-v4` |
| `TestG18D6bDriver` | PASS, exit 0, no out-of-memory (OOM), no timeout | `harness_out/docker/20260822T123532Z-d6b-driver-refresh-v19` |
| `TestG18D6aProducerTelemetry/go/language` | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123627Z-d6a-go-language-refresh-v21` |
| `TestG18D6aProducerTelemetry/erlang/macro_expanded_top_level_function` | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123634Z-d6a-erlang-refresh-v22` |
| Default no-tag compile | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123642Z-d6b-no-tag-refresh-v23` |
| Combined D6a/D6b gate | FAIL, exit 1, no OOM, no timeout | `harness_out/docker/20260822T123547Z-d6b-d6a-combined-refresh-v20` |
| Combined D6a/D6b gate | FAIL, exit 1, no OOM, no timeout | `harness_out/docker/20260822T123655Z-d6b-d6a-combined-refresh-v24` |

Both combined gates failed only in the documented process-global pool telemetry comparison.
The affected subtests changed between runs. The isolated targets passed. No OOM or timeout occurred.

### Safe D6b decline

D6a authenticates each frontier member independently. D6b also requires one common action and exact derivation.

An authenticated frontier can place every dropped participant in one cohort with the survivor candidate.
If it has no common proof, the consumer returns a typed decline.
The scheduler then runs the existing alternative-set proof.

A mixed-cohort frontier is not a safe decline. The consumer returns a fatal error and poisons the transaction.

Keep every malformed, stale, foreign, resealed, blended, different-cohort, or otherwise unauthenticated frontier as a fatal error.
Do not weaken exact derivation comparison.

The `grammargen_lr` witness demonstrates this boundary:

- D6a publishes two complete frontiers.
- The target frontier has two members with equal actions and different derivation digests.
- D6b declines the frontier without changing its state or journal.
- The existing alternative-set proof declines, so production fallback serves the tree.
- Candidate and production trees have equal locked-C deep digests.

The focused Docker receipts are:

- `harness_out/docker/20260822T184936Z-d6b-core-typed-decline-refresh-review7`
- `harness_out/docker/20260822T184953Z-d6b-grammargen-lr-safe-decline-refresh-review8`
- `harness_out/docker/20260822T185008Z-d6b-driver-positive-fatal-refresh-review9`
- `harness_out/docker/20260822T185021Z-d6b-layout-budgets-refresh-review10`

These receipts do not graduate direct D6b admission.

The matrix counts and performance counts remain unchanged.
This evidence does not graduate the route or wire production drops.

### D6c blocker receipt: `grammargen_lr`

The receipt branch uses base commit `ed0568d9`. The focused Docker reproduction
ran at original probe commit `0c34a681` and records the authenticated frontier
used by the D6b decline. The frontier drops participant `1`.

| Member | Derivation digest | Length | Continuation state | Head byte |
|---|---|---:|---:|---:|
| Survivor | `9b1c3a249bec15d4b74a7462f701c491e022be80f7a51a5590f1520a76fd2c06` | 5,254 | 1,141 | 1,046 |
| Dropped | `d72a6fe90ca3aec9883bd00494eb8ca7110ede90d5f09fb5000fdc6441a79e8f` | 5,324 | 680 | 1,046 |

Both members use the same authenticated scanner checkpoint at source span
`1046..1047`. The first continuation-state difference is `1141` versus `680`.

The first public projection difference is path `[0,4]`, span `1030..1037`,
source text `prodIdx`:

- The survivor materializes terminal `identifier` symbol `86`, production `0`.
- The dropped member materializes `parameter_declaration` symbol `113`,
  production `36`, with one `_simple_type` child under field `type`.

The focused regression test records a typed `no_action` D6b decline. The
existing `grammargen_lr` fallback gate also passes. Candidate and production
trees retain locked-C deep digest
`1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2`.

The labeled Docker artifacts are:

- `/tmp/gts-d6c-nogo-receipt-20260822/harness_out/docker/20260822T202359Z-d6c-frontier-state-public-shape`
- `/tmp/gts-d6c-nogo-receipt-20260822/harness_out/docker/20260822T202427Z-d6c-grammargen-lr-fallback`

The D6c decision is NO-GO. A drop may proceed only when the authenticated
continuation state and the canonical public projection match. Any state or
public-shape mismatch must decline and preserve the production fallback.

## Current fallback taxonomy

The 30 fallbacks divide into 15 clean production trees and 15 production error trees.

| Class | Clean | Error tree | Total | Exact trigger |
|---|---:|---:|---:|---|
| Recovery handoff | 0 | 13 | 13 | The elected token has no table action at end-of-file. |
| Selected-lineage ownership | 11 | 1 | 12 | A converged split drop lacks one selected-lineage proof. |
| Certified repetition conflicts | 3 | 0 | 3 | The generic scheduler declines a repetition shift. |
| Acceptance-frontier ownership | 1 | 1 | 2 | The end-of-file frontier has more than one active head. |
| Total | 15 | 15 | 30 | |

The recovery handoff class contains these witnesses:

- Dart
- Go module
- INI, two files
- Make
- Objective-C
- PHP, two files
- PowerShell
- SQL, two files
- Swift
- TypeScript

The selected-lineage class contains these witnesses:

- C#
- D
- Dart
- Elixir
- Julia
- Kotlin, two files
- Perl, two files
- Scala, two files
- TSX

The repetition class contains one C# file and two Haskell files.
The acceptance class contains one Bash file and one Markdown file.

The scheduler now scopes each condense action to live headers.
It no longer counts removed versions against the shared link cap.
This retires the prior Go module, OCaml, and Rust shared-cap witnesses.
The live scope retains discarded boundary history as split provenance.
Both Perl witnesses remain on the production route without an exact lineage proof.

## Class prerequisites

The recovery handoff must preserve production error-tree ownership.
Do not convert a failed compact acceptance into a clean result.

Selected-lineage ownership must identify the exact surviving reduction path.
An artifact certificate can authorize only a pinned grammar with C-oracle evidence.

Certified repetition conflicts need one reusable conflict rule or exact artifact evidence.
Do not add a grammar-name branch to the scheduler.

Post-accept continuation must preserve an accepted result while live end-of-file reductions finish.
The Markdown continuation reaches score 160 and branch 8.
It materializes the exact production digest `a411b8648c76`.
The path still needs one selected-lineage proof for an intervening converged split drop.

The Bash frontier has one accept head and one table-dead head.
Production returns an error tree for that file.
Keep this file on the production route.

## Historical bounded result at `382080a3`

Commit `963fae08` recorded this receipt on 2026-07-29.
The run used `382080a3` as its base.
It is not the current matrix result.
It enabled ratchet mode, excluded AWK, and set the maximum file size to 16,383 bytes.

| Status | Files |
|---|---:|
| PASS | 67 |
| FALLBACK | 33 |
| SKIP | 10 |
| DIVERGE | 0 |
| ERROR | 0 |
| Total | 110 |

That corpus manifest contained 147 verified files across 50 languages.
The run selected files smaller than 16,384 bytes and excluded AWK.
Its manifest included one additional Elm highlight file.
The current manifest includes that generated file.

That receipt also recorded this 206-language smoke scorecard:

| Status | Languages |
|---|---:|
| PASS | 200 |
| FALLBACK | 1 |
| SKIP | 5 |
| DIVERGE | 0 |
| ERROR | 0 |

## Earlier campaign receipts

The following sections preserve earlier certification, fixture, corpus, rejection, and performance evidence.
They do not replace the current bounded result.

### Certified acceptance frontiers

Exact artifact profiles now enable three generic selection mechanisms.

| Language | Mechanism | Pinned C commit |
|---|---|---|
| HTTP | Accept one EOF head and drop no-action siblings | `db8b4398de90b6d0b6c780aba96aaa2cd8e9202c` |
| Robot | Accept one EOF head and drop no-action siblings | `278958ff2fc44732833f717ee864c9fe4dae6e11` |
| Meson | Select the sole primary accepted derivation | `c84f3540624b81fc44067030afce2ff78d6ede05` |
| Bash | Allow converged-path reduction split drops | `a06c2e4415e9bc0346c6b86d401879ffb44058f7` |
| Erlang | Allow converged-path reduction split drops | `1d78195c4fbb1fc027eb3e4220427f1eb8bfc89e` |
| Haskell | Allow converged-path reduction split drops | `0975ef72fc3c47b530309ca93937d7d143523628` |
| JavaScript | Allow converged-path reduction split drops | `58404d8cf191d69f2674a8fd507bd5776f46cb11` |
| Python | Allow converged-path reduction split drops | `bffb65a8cfe4e46290331dfef0dbf0ef3679de11` |

Separate Docker runs compared each selected Go tree with its pinned C parser.
All eight passed field-aware exhaustive comparison.

The evidence is in these artifact directories:

- `harness_out/docker/20260728T050445Z-compact-frontier-http-c-oracle-fields`
- `harness_out/docker/20260728T050509Z-compact-frontier-robot-c-oracle-fields`
- `harness_out/docker/20260728T050512Z-compact-frontier-meson-c-oracle-fields`
- `harness_out/docker/20260728T054141Z-compact-split-bash-c-oracle-fields`
- `harness_out/docker/20260728T054151Z-compact-split-erlang-c-oracle-fields`
- `harness_out/docker/20260728T054158Z-compact-split-haskell-c-oracle-fields`
- `harness_out/docker/20260728T054206Z-compact-split-javascript-c-oracle-fields`
- `harness_out/docker/20260728T104725Z-pr491-python-cert-final`

Custom, adapted, stale, and same-name grammars retain conservative defaults.

### Bounded no-lookahead reductions

The generic scheduler now supports one authenticated synthetic-EOF shape.
One runnable head can apply one reduction and re-elect at the same byte.
A transparent goto marks the reduced node as an extra.
A root reduction must meet authenticated EOF on the next election.
The scheduler declines wider frontiers, scanner changes, and runaway re-election.

This mechanism routes the Doxygen, JSDoc, and VHDL smoke fixtures.
Field-aware C-oracle runs passed for all three grammars:

- `harness_out/docker/20260728T061644Z-compact-nolookahead-doxygen-c-oracle-fields`
- `harness_out/docker/20260728T061726Z-compact-nolookahead-jsdoc-c-oracle-fields`
- `harness_out/docker/20260728T061735Z-compact-nolookahead-vhdl-c-oracle-fields`

### Zero-width extras with byte progress

The generic scheduler now recognizes progress from a parser boundary to the
end of a zero-width extra token.
It still declines when the byte, parser state, and scanner state do not change.

This mechanism routes the COBOL smoke fixture.
Its field-aware C-oracle run passed:

- `harness_out/docker/20260728T064111Z-compact-zero-width-cobol-c-oracle-fields`

### Cooklang smoke fixture

The prior Cooklang smoke source ended an ingredient instruction with a period.
Production recovery discarded that period.
The fixture now uses a valid ingredient instruction without the period.
The corrected source routes directly.
The old dotted source remains a required compact fallback.

The Cooklang field-aware C-oracle run passed:

- `harness_out/docker/20260728T064513Z-compact-clean-smoke-cooklang-c-oracle-fields`

### Lock-pinned corpus refresh

The previous 109-file matrix used a stale generated corpus directory.
The builder at `382080a3` selected one additional Elm highlight source.
Two independent rebuilds produced the same normalized manifest hash.
After volatile fields are removed, the hash is
`1e9998f1e4282c3c3397f518638a8779e016a0038064903f8de90f48b781661e`.

| Field | Value |
|---|---|
| Repository | `elm-tooling/tree-sitter-elm` |
| Commit | `6d9511c28181db66daee4e883f811f6251220943` |
| Source path | `test/highlight/basic.elm` |
| Bytes | 1,231 |
| SHA-256 | `8fca87bd8cc2735e83704acd8d06ffbc6cf04e386505de45596218d7fb72642c` |
| Compact result | PASS |

The tracked [Elm fixture](../testdata/admission_direct/elm_highlight_basic.elm)
protects this source when the generated corpus directory is absent.

The canonical ratchet enforces these bounds:

- At least 110 selected rows.
- At least 67 direct PASS rows.
- At most 33 FALLBACK rows.
- Exactly 10 SKIP rows.
- No DIVERGE or ERROR rows.

Ratchet mode rejects noncanonical language, bucket, AWK, and byte filters.
Manifest order does not affect the aggregate bounds.

### Rejected C# convergence candidate

The cap-one GSS convergence candidate did not preserve C# parity.
Commit `3204480b` lost field attributes and a modifier in this 65-byte witness:

```csharp
[System.Serializable]
struct S
{
    [System.Obsolete]
    int x;
}
```

Setting `GOT_GLR_MAX_MERGE_PER_KEY=16` restored the expected tree.
The 25-case Docker suite also found a net correctness and memory loss.

| Metric | `main` | Candidate |
|---|---:|---:|
| No-error outcomes | 24/25 | 24/25 |
| Deep tree matches | 20/25 | 19/25 |
| Maximum RSS | 1,382,676 KiB | 1,601,596 KiB |

The campaign rejected this candidate.

### Performance gate

The stable benchmark trio used:

- `GOMAXPROCS=1`
- `-count=10`
- `-benchtime=750ms`
- `-benchmem`

The comparison uses `f639fbaa` as the base.

| Benchmark | Time | Bytes | Allocations |
|---|---:|---:|---:|
| Full DFA parse | -8.01% | -12.06% | unchanged |
| Incremental single-byte edit | -5.71% | unchanged | unchanged |
| Incremental no-edit | +1.48% | unchanged | unchanged |

The large 5,000-function probe completed without an out-of-memory failure.
A balanced rerun measured 83,240 KiB for the base and 83,360 KiB for the head.

Compact reduction outputs now carry the multi-pop fact directly.
This avoids copying the complete compact work record twice per reduction.

The compact scheduler now stores its seed frontier in its own allocation.
The warm full-parse benchmark moves from 20,352 to 20,328 bytes per operation.
Allocation count moves from 66 to 65 per operation.
Time remains statistically unchanged (`p=0.853`).

## Reproduce the run

Run this command from the repository root:

```sh
GTS_ADMISSION_REAL_CORPUS=1 \
GTS_ADMISSION_REAL_CORPUS_RATCHET=1 \
GTS_ADMISSION_REAL_CORPUS_EXCLUDE_LANGS=awk \
GTS_ADMISSION_REAL_CORPUS_MAX_BYTES=16383 \
GTS_ADMISSION_CENSUS=1 \
GOMAXPROCS=1 go test . \
  -tags gts_parsercorephase0 \
  -run '^TestAdmissionCandidateRealCorpusMatrix$' \
  -count=1 \
  -v
```

The test reads `cgo_harness/corpus_real/manifest.json` by default.
Use `GTS_ADMISSION_REAL_CORPUS_MANIFEST` to select another manifest.

Add ratchet mode when you reproduce the current 110-row receipt.
The current result satisfies the canonical 110-row bounds.

Use these optional filters:

- `GTS_ADMISSION_REAL_CORPUS_LANGS`
- `GTS_ADMISSION_REAL_CORPUS_EXCLUDE_LANGS`
- `GTS_ADMISSION_REAL_CORPUS_BUCKETS`
- `GTS_ADMISSION_REAL_CORPUS_MAX_BYTES`

## Per-language matrix

| Language | PASS | FALLBACK | SKIP |
|---|---:|---:|---:|
| bash | 2 | 1 | 0 |
| c | 0 | 0 | 2 |
| c_sharp | 0 | 2 | 0 |
| clojure | 2 | 0 | 0 |
| cmake | 2 | 0 | 0 |
| cpp | 0 | 0 | 3 |
| css | 1 | 0 | 0 |
| d | 1 | 1 | 0 |
| dart | 0 | 2 | 0 |
| elixir | 2 | 1 | 0 |
| elm | 3 | 0 | 0 |
| erlang | 2 | 0 | 0 |
| go | 2 | 0 | 0 |
| gomod | 2 | 1 | 0 |
| graphql | 3 | 0 | 0 |
| haskell | 0 | 2 | 0 |
| hcl | 2 | 0 | 0 |
| html | 3 | 0 | 0 |
| ini | 0 | 2 | 0 |
| java | 0 | 0 | 2 |
| javascript | 1 | 0 | 0 |
| json | 0 | 0 | 3 |
| json5 | 2 | 0 | 0 |
| julia | 1 | 1 | 0 |
| kotlin | 0 | 2 | 0 |
| lua | 2 | 0 | 0 |
| make | 1 | 1 | 0 |
| markdown | 1 | 1 | 0 |
| nix | 3 | 0 | 0 |
| objc | 1 | 1 | 0 |
| ocaml | 2 | 0 | 0 |
| perl | 1 | 2 | 0 |
| php | 1 | 2 | 0 |
| powershell | 1 | 1 | 0 |
| python | 2 | 0 | 0 |
| r | 3 | 0 | 0 |
| ruby | 2 | 0 | 0 |
| rust | 1 | 0 | 0 |
| scala | 0 | 2 | 0 |
| scss | 2 | 0 | 0 |
| sql | 1 | 2 | 0 |
| svelte | 2 | 0 | 0 |
| swift | 1 | 1 | 0 |
| toml | 3 | 0 | 0 |
| tsx | 1 | 1 | 0 |
| typescript | 1 | 1 | 0 |
| xml | 2 | 0 | 0 |
| yaml | 3 | 0 | 0 |
| zig | 2 | 0 | 0 |

## Historical safety boundary receipt

The refreshed Kotlin fixture exposed one silent compact divergence.
The 38-byte reduction is:

```kotlin
internal actual fun f(): String = "x"
```

Production Go and tree-sitter C return the same error-bearing tree.
The compact parser previously returned a clean function declaration.

The compact scheduler merged two conflict paths.
A later reduction split those paths into separate heads.
The scheduler then dropped one head and accepted the other.

Admission fails closed after this unproved frontier shape.
Exact built-in profiles can certify the behavior against the C oracle.
Custom, adapted, stale, and unproved built-in grammars stay conservative.

The boundary originally moved four files to FALLBACK:

- Bash medium
- Erlang medium
- JavaScript small
- Kotlin small

Focused field-aware C-oracle receipts now certify Bash, Erlang, and JavaScript.
Those files route directly again. Kotlin remains fail-closed.

## Corpus state

The current manifest has these properties:

- 50 declared languages
- 50 languages with selected files
- 147 files
- No missing files
- No size mismatches
- No SHA-256 mismatches
- No stale source commits
- No absolute output paths

The focused quality gate passes:

```sh
cd cgo_harness
GTS_REAL_CORPUS_MANIFEST=corpus_real/manifest.json \
  go test . -run '^TestRealCorpusManifestQuality$' -count=1
```

The corpus directory is ignored by Git.
Rebuild it from the exact profile before a new campaign.
