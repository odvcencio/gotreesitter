# Go malformed-delete repair and authenticated-reuse qualification

The four reported malformed-delete C mismatches are repaired. Go's generated lexer now recognizes the colon and equals tokens that C retains inside the ERROR node after deleting `x` from `x := a + b`. All 12 exact Cognociente insert/replace/delete cases pass on both ordinary and candidate routes, through plain and profiled APIs. Eight reduced prefix cases also pass. This bucket changes the Go grammar artifact and its authenticated identities; it does not enable the held reuse patch, graduate Go, change routing defaults, or authorize a merge.

## Change and comparison

The report witness is 140,368 bytes, 1,760 functions, SHA-256 `5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`; the edit is byte 57. The 50-byte reduced witness deletes the declaration name at the start of a function body. Go's old error mode inherited an initial-document lexer without the required punctuation, and its missing-token follow allowlist omitted colon. Merely adding `:=` to normal modes is not C-equivalent: C recognizes colon first in the block context and equals after entering recovery.

Two explicit grammar options solve those distinct problems: a separate error mode includes fixed non-immediate terminals, while the existing follow/missing-token analysis can admit selected extra terminals. Go selects colon. Contextual pattern and immediate-only terminals are not indiscriminately added to recovery. Other grammars retain their old options. The generated parser action tables are unchanged; the DFA grows from 301 to 306 states.

The pre-repair base is `aa796ab8a435452903747f7b8da69773faf6aa2a`. Old blob SHA-256: `df63fc35604c4e4e7a484abde9eb2110b61640045601c23991723f323a48310d`. New blob SHA-256: `214322327c9ee7dda29febf32e415f7ed9e00a90b7690d3c32522e393211b8da` (51,685 bytes). Both exact identities remain recognized by the native publication contract. Runtime profiles, canonical fixtures and candidate authentication use the regenerated artifact. The immutable legacy trace file is unchanged: its test authenticates old/new blob hashes and compares every execution field after normalizing only identity metadata.

## Correctness and counters

- The final Go C suites pass, including the 12 report cases, eight reduced cases, compact R4 structural check, and four canonical human-source fixtures (5–235 KiB). These compare Go incremental/fresh and C incremental/fresh independently.
- Both Go invariant routes pass 72 edits each; no-edit reparses allocate zero. Focused lifetime and memory-budget tests pass.
- All four owned grammar reproducibility checks pass: Go, Regex, Swift and YAML. Focused generation/emission tests pass.
- Public compact-view tests, sealed work/trace gates, native contract checks and focused races pass. The old eager-publication test failed on the baseline too because lazy views bypassed its intended publisher. Its control now explicitly selects eager output; every original execution/metadata assertion remains. Its wide witness has 65,540 children.
- The entire standard Go R4 ledger is identical, including route and fallback reasons: candidate full/edit nodes 892/1,553; default 1,922/3,810. The clean report insert/replace complete-operation counters are also identical.
- Correct C error tokens add malformed-delete work. [Every changed complete counter](counter-changes.md) is retained: all increases are below 2%, with a maximum of +1.04% stack-version creations. Selected nodes rise 7,198→7,248 default and 7,163→7,213 candidate; reused bytes/subtrees/splices do not fall. [All selected profile changes](selected-profile-changes.json) include diagnostic stack and arena fields, including increases above 2% in diagnostic GSS fields. No counter baseline is reset.

The corrected public-work probe uses `Parse` for fresh work, and includes Tree.Edit, profiled incremental parsing and releases for edit work. Earlier `*-work.log` diagnostics used `ParseIncrementalProfiled(source,nil)` for their fresh phase; those are not substituted for public Parse. Instrumentation nanoseconds are not performance evidence. The final raw public-work logs and complete deltas are preserved.

## Paired performance check of the repair

Twenty paired processes use the unchanged primary Go DFA functions, seeds 1–20, identical seeded child order, alternating revisions, count=1, 750 ms and benchmem. This local Intel Core Ultra 9 285 / Linux amd64 / Go 1.26.6 run compares only the grammar repair against the pre-repair artifact; it is not the original hosted CI hardware and does not time the held V4 patch.

| Workload | Before median | After median | Result |
|---|---:|---:|---|
| Full parse | 6.257 ms | 6.673 ms | directional increase, inconclusive (p=.383) |
| Single-byte edit | 99.91 µs | 101.72 µs | directional increase, inconclusive (p=.242) |
| No edit | 4.913 ns | 4.919 ns | inconclusive (p=.678) |

Allocations remain 6/5/0; edit/no-edit bytes remain 394/0. Full allocated bytes are inconclusive (p=.156). These noisy results establish no speed win or statistically demonstrated equivalence. The change is a correctness fix with the stated additional malformed-recovery work. [Raw before](repair-before.txt), [raw after](repair-after.txt), and [benchstat](repair-benchstat.txt) retain every sample. The baseline mounts the same canonical checkout with old Go blob/profile/fixture/driver overlays; this is a source-controlled artifact comparison, not a claim that both mount paths contain separate Git revisions.

## Held authenticated-reuse experiment

The existing V4 patch was applied only through per-file overlays to the repaired Go source. The patch's uncompressed SHA-256 is `5e4b5e0c77031777d68ed78cd6a1bc6a44cd02468c4a84c37ccd8c517f8e9284`; its source is [the earlier held receipt](../20261021-trusted-small-source-results/README.md). No V4 runtime code is activated by this commit.

On this composition, all repaired Go C checks, canonical C checks, both 72-edit invariant sessions, internal reuse tests, producer/source/context controls, memory/lifetime tests and focused races pass. Custom/wrapped streams and unsupported provenance retain fresh verification. Qualification covers actual default DFA producer identity, source identity, complete reads, dispatch schedule, old clean acceptance and clean resulting acceptance.

For each ordinary insert and replacement in the report witness, complete default-route edit work falls as follows:

| Work | Repaired baseline | Held V4 |
|---|---:|---:|
| Parse attempts | 2 | 1 |
| Shifts | 72,200 | 1,792 |
| Reductions | 75,729 | 1,803 |
| Lexer front-door calls | 59,875 | 1,786 |
| Parent constructions | 45,804 | 1,798 |

This is removal of a redundant verification parse, not a measured speedup on this composition. Candidate-route ordinary edits already use one attempt and are unchanged. Delete still uses five attempts; V4 increases its default-route shifts 214,855→214,859 and table lookups 1,024,644→1,028,175. Every complete change is in [the work delta](public-work-changes.json).

The standard default Go R4 first edit has these changed fields (full and candidate rows stay identical):

| Counter | Repaired baseline | Held V4 | Change |
|---|---:|---:|---:|
| Tokens | 572 | 614 | +7.34% |
| New nodes | 3,810 | 3,219 | −15.51% |
| Max live versions | 12 | 8 | −33.33% |
| Reused bytes | 1,166 | 1,044 | −10.46% |
| Block splices | 296 | 260 | −12.16% |
| Multi-version share, ppm | 531,468 | 495,114 | informational |

Those token/reuse regressions fail the 2% ledger tolerance. The global V4 patch also retains the earlier all-language holds; Go-only qualification does not erase them. No R3 approval has been given for these changes. The next reuse step is to isolate why this malformed-edit fixture loses reuse and remove that regression before considering promotion. If accepting a measured trade becomes justified, [R3](../../v1-design.md#r3-counter-ledger-changes) requires an explicit owner exception: “Agents do not approve ledger exceptions.” Earlier Liquid/YAML approvals do not apply. The older M1 Max timing belongs to its old source composition and is not new timing evidence here.

## Limits and reproduction

An adjacent reduced bare `= a + b` prefix still differs from C's recovery selection. The old Go artifact already fails that case; the candidate improves tokenization but can elect a missing-identifier assignment where C skips equals. Its failed diagnostic is retained. This is an unresolved Go correctness gap, not an accepted exception or a graduation pass.

C reference grammar stays pinned to `tree-sitter-go` commit `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`; the local C runtime is 0.25.1. Graduation still needs the prescribed C 0.27.x qualification, E-A corpus exit gate, hardware/platform coverage and large-file RSS. No new native Windows/ARM64 or 1 MiB RSS qualification is claimed. All merges to main remain held.

The evidence archive contains raw final logs, earlier failed controls, all per-file overlays, exact Docker command arrays and run scripts. No checkout copy, build binary, credential, or unrelated file is included. `manifest.json` authenticates each retained member. Scripts mount the canonical checkout read-only at `/candidate` and `/baseline`; overlays define the actual comparison. They use pinned Go 1.26.6, one CPU, GOMAXPROCS=1, a 4 GiB Docker memory cap, offline dependency caches and the shared workload lock. Adapt only the host mount paths for reproduction. The tracked C regression is `TestGoCognociente137KiBTopEditLockedC`; the prefix regression is `TestGoMalformedDeclarationPrefixLockedC`.
