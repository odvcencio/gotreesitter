# Native replay interface screen, 2026-10-23

A narrower replay interface reduces time and one allocation on the private ordinary-YAML prototype. **It remains unpromoted and is not installed in this PR's runtime.** Apply [source.patch.gz](source.patch.gz) to the 77-file prototype preserved in [the earlier receipt](../20261023-ordinary-yaml-link16/README.md), not directly to main. The patch checks cleanly against that exact frozen source.

Native publication already rejects reused subtrees before replay. Its new value interface calls the same parser-table transition with the real symbol and terminal flag, avoiding construction of a general borrowed view and its callback closure. The full-view interface remains available, and exactly one replay interface must be supplied. Borrowed-subtree rejection, raw validation, checkpoint authentication, polling and byte coverage checks remain in force.

Twenty shuffled alternating pairs per suite establish these YAML time changes:

| Workload | Before → after | Change |
| --- | ---: | ---: |
| R4 ordinary | 232.4 → 229.8 µs | −1.12%, p=.006 |
| EOF 8 KiB ordinary | 880.2 → 865.7 µs | −1.65%, p<.001 |
| R4 diagnostic-owned | 368.9 → 364.3 µs | −1.23%, p=.012 |
| EOF 8 KiB diagnostic-owned | 1.408 → 1.399 ms | nonsignificant, p=.174 |

Legacy YAML controls are nonsignificant. Each native YAML case drops one allocation and 16 B/op: ordinary R4 **40→39**, ordinary EOF **71→70**, owned R4 **74→73**, owned EOF **197→196**. All directions remain in the [YAML table](benchstat-yaml.txt).

**No Go speedup is attributed to this interface.** Current Go routing does not select native publication; its materializer is a different path. The primary edit control moves −0.46% (p=.037) while bypassing the change. The other primary timings and all six compact-Go timings are nonsignificant. Go allocation counts are unchanged. See [primary Go](benchstat-go.txt) and [compact Go](benchstat-compact-go.txt).

Both routes complete **412 invariant sessions / 29,664 edits** and **1,236 locked-C fixture checks** across all 206 languages. All no-edit allocations are zero. **Every one of the 206 selected ledger exports is byte-identical**, and all timed work, route and reuse metrics remain exact. Focused Core/shadow/race, actual 386, large YAML, root checkpoint and twenty memory-contract checks pass. Source hashes remain unchanged. The source delta is three existing Go files, with no new root file or language-name comparison.

Timing uses Go 1.26.6 linux/amd64, one CPU, GOMAXPROCS=1, count=1, 750 ms and benchmem with the repository randomized runner. YAML seeds 181001–181020, primary Go 182001–182020 and compact Go 183001–183020 yield **300 rows per role**. Seed identity, paired workload order, alternating role order and all median ratchets are independently checked. No additional cold-RSS or native Windows/Mac qualification is claimed for this three-file delta.

The equal-value version-state reuse experiment is rejected separately. Its own 412 invariant sessions, 1,236 C checks, 206 exact ledger exports and 300 randomized rows per role pass, but allocations stay unchanged and timing moves do not establish causal improvement: legacy controls move more than the affected YAML cases. Its complete source patch and failed idea remain in the archive.

An untimed follow-up checks Go publication capability. Both the diagnostic scanner and production Go admission reproduce all four canonical inspection digests, including the 235,626-byte grammar source. The production runner has no native workspace and issues no owned lexer requests; its S3 recovery capability also keeps the current selection gate closed. This establishes a target for further qualification, not permission to change routing. The first probe build's import-cycle failure and its import-safe repair are retained. The logged production `recovery_executed` value is the constructor's initial marker, not evidence about the later parse.

The earlier ordinary-YAML prototype still fails its main-based throughput and allocation qualification. This incremental improvement does not clear those holds, the existing warmed legacy-Go ceiling, or Cognociente's malformed-delete C mismatch. No graduation or default change is made.

[qualification.json](qualification.json) and [receipts.tar.gz](receipts.tar.gz) retain every source hash, successful and failed run, raw sample, order check and counter export. Every regular archive member was extracted and verified against the SHA-256 manifest.

Archive: 3430070 bytes; 3472 regular members; SHA-256 `6987f0e901e6c442885db430404953067bb71ad040c4866e578f4819d243c7ed`.
