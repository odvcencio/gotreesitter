# End-of-file recovery receipt for issue 1056

This receipt records end-of-file (EOF) recovery behavior.
This receipt compares the candidate with main at `d25623f69`.
The 57-row production baseline was sealed at `6643615cf` and survived the rebase unchanged.
The locked C runtime is tree-sitter 0.25.1 at `f5afe475deb7c0bae6407fb776c76824f717bb61`.

## Native witness

The Scala source is `((y)->`.
Its SHA-256 digest is `66568f2d2cf310e447ed512da7a3fad0be45158327905c4ffd0dd857bd90934c`.

Locked C processes eight versions after recovery starts.
The later physical states appear in this order:

1. `5364`
2. `5381`
3. `4304`
4. `5952`
5. `12937`
6. `14397`
7. `1`

The C oracle publishes one missing `)` at byte 6.
The root covers bytes 0 through 6 and has an error.
The production and compact routes match its deep digest, `03a458abd5832f6326e03b833a3890ea8d48ca43eb91ada1951bb020871f49a6`.
The incremental edit at byte 1 matches a fresh C parse with digest `1cdf3d91810eba806f4b152eb30fc386f3811053e289006b1ab012008dccb400`.

## Scala suffix differential

The 57 cases combine three prefixes with 19 suffixes.
The fixture `cgo_harness/testdata/scala_eof_recovery_production_baseline.tsv` records each production tree's deep digest, root type, and error flag.

| Prefix | Exact against C | Existing C gaps | Candidate tree changes |
| --- | ---: | ---: | ---: |
| `((y)->` | 18 | 1 | 0 |
| `(y;` | 19 | 0 | 0 |
| `((y)->; ` | 13 | 6 | 0 |
| Total | 50 | 7 | 0 |

Every gap has a Go `compilation_unit` root and a C `ERROR` root.
The seven gaps exist on main at `6643615cf`.
The complete 57-row production digest table has SHA-256 `b4582e302531c84e6ef703879cb29c2231a9b184ccbc87b4e2fcdb97da804f60` before and after the change.

The gap sources are:

- `((y)->\n;`
- `((y)->; ;`
- `((y)->; }`
- `((y)->; \n}`
- `((y)->; ; `
- `((y)->; \n;`
- `((y)->; ;\n`

## Bounds and counters

The direct bound test reserves shared stack slots before it constructs a reduction parent.
It admits one parent when one slot remains and constructs none when the stack is full.
The native EOF trial uses this shared bound on the certified Scala blob when one Go path enters recovery.
The blob SHA-256 is `b319fb9e030c13c99c852cd0b09b76bc975fbd63c8b6d6999d711865ec9a5862`.
Other grammars use the prior exact EOF trial until a locked C differential certifies their production receipts.
An expanded Scala frontier uses that exact trial for the rest of the parse.
Native sentinel parity remains unproven for those frontiers and the other grammars.

The Scala differential reached six reduction candidates and one missing-token trial per recovery call.
Its peak Go stack count was 11.
The peak arena allocation was 7,658,888 bytes; the peak scratch allocation was 4,091,044 bytes.
The work ceilings recorded no hits. The 57 cases recorded no EOF fallbacks.

The Go malformed-recovery receipt and the SQL dispatcher receipt pass with the certification gate.
The unrestricted sentinel trial changed Go's raw tree away from locked C.
It changed SQL's raw tree and production dispatch counts, although SQL's production tree still matched C.
The exact-row fallback preserves Accept viability outside the potential-reduction collector.
Both receipts pass on main at `d25623f69` and on the gated candidate.
The parser layout assertions pass with the new 24-byte EOF recovery state and eight-byte runtime counter.

## Swift memory witness

The 104,681-byte Swift witness previously reached 3,370,924 KiB and took over 19 minutes with an unbounded sentinel trial.
The witness file has SHA-256 `ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc`.
The bounded candidate completed the final Docker test in 0.93 seconds with no truncation or memory stop.
The production tree kept main's deep digest, `5845d485aa44b55a82b1edac8d55ac7124e9ecb86dafc98875b3d1d356b9f44c`.
The existing Swift test still expected an older Go digest, so this change updates its pin to the measured main digest.
The locked C deep digest remained `dd81933bab64e72135317be11daf77ffd6e15531819f82e8dc15dea1aba1b8be`.
The production test requires at least one EOF fallback and at most 200,000 nodes.
The unrestricted sentinel trial changed both Swift prefix digests during development.
The exact-row fallback restored both production digests to main's values.

| Swift source prefix | Main Go digest | Candidate Go digest |
| --- | --- | --- |
| 8,192 bytes | `89f975bab6c0f4b13b61b726aafbfc9a54b13db96db6391a42e852ac5d72202c` | `89f975bab6c0f4b13b61b726aafbfc9a54b13db96db6391a42e852ac5d72202c` |
| 16,384 bytes | `d49bb7a635bd849c39515aa1472c67037f8e6bd668f27ff1ace636b6d6df6a6f` | `d49bb7a635bd849c39515aa1472c67037f8e6bd668f27ff1ace636b6d6df6a6f` |

The 16,384-byte prefix still has a Go `source_file` root and a locked C `ERROR` root.
That prefix has SHA-256 `a2f183ebd4461a33c9a1f3a12f142967a59d9f9bb2385e7645f064715ce39fcc`.
The 16,384-byte locked C prefix has deep digest `b91fecb142dd879bfaba7ed9dca4b0bdb13deeb4426a08ffb6981f4a3445edb7`.
The prefix regression test pins this known gap and a 40,000-node budget.

Warm runs of the same 104,681-byte Swift test gave these maximum resident set sizes.
The measurement includes the `go test` process.

| Main | Candidate | Stop or memory failure |
| ---: | ---: | --- |
| 257,440 KiB | 257,332 KiB | None |

## Performance

The repository runner used 20 alternating shuffle seeds, `GOMAXPROCS=1`, and a 750 ms benchmark time.
Both sides used the `gts_parsercorephase0` build tag.
The host CPU was an Intel Core Ultra 9 285.

| Benchmark | Main `ns/op` | Candidate `ns/op` | Main `B/op` | Candidate `B/op` | `allocs/op` on both |
| --- | ---: | ---: | ---: | ---: | ---: |
| Full Go parse | 4,770,000 | 4,883,000 | about 1,257 | about 1,258 | 8 |
| One-byte Go edit | 119,200 | 123,800 | 386.5 | 387 | 5 |
| No-edit Go parse | 4.348 | 4.129 | 0 | 0 | 0 |

The full benchmark set found no significant timing difference in these three rows.
Its baseline output has SHA-256 `e2d1f91f7edc7da0df9cd1aba7c34c3e15fd5c0f4985634122e51cfeeb67ead5`.
Its candidate output has SHA-256 `052a372c20059b6ee5929f9ae28b0bdbbd3462a2073c6762856f8fa428f7261f`.
The complete `benchstat` output has SHA-256 `1288d4de15304498a69ac3e37945d5adec0f0445626b554ffa62a686de7ad4c5`.

An isolated 20-seed run of the same three benchmarks also found no significant timing difference.
It measured one-byte edits at 112,100 `ns/op` on main and 120,900 `ns/op` on the candidate.
It measured 386 `B/op` and five allocations on both sides.
The isolated baseline and candidate output hashes are `5b8889ae3ae5fc231d33180dbadae102c534ea92afaeb73304ec58330a0fa5c1` and `271b5a7c07591086d0473a55a4a5c884f58c46b6aafabdd9de726937606c21bf`.

Earlier 20-seed runs against `170ee57a5` found significant one-byte edit slowdowns between 5.88% and 13.11%.
The final run had wide host variation, so the directional edit latency risk remains open.
The final runs found no allocation-count regression.

## Correctness gate status

The direct root tests and the three focused Scala locked-C tests passed in Docker on September 24, 2026.
The two focused Swift memory tests passed in the next isolated Docker run.
The Go and SQL locked-C receipt tests passed after the Scala certification gate.
The parser layout assertions passed after the EOF state moved before the tail scratch map.
The blob gate rejected a same-name language without the certified Scala blob hash.
The broader Scala generator runner stopped before parsing any case.
Its importer rejected `_end_marker_named_tail` because helper `endMarkerTail` contains an unsupported numeric rule expression.
That result supplies no recovery parity verdict; issue #1067 owns the Scala generator baseline.

## Reproduction

Run each Docker command under `/home/draco/.local/state/nightwatch/gts-docker.lock`.
Set `GOWORK=off` for every Go command.

```sh
cd cgo_harness
GOWORK=off go test -tags treesitter_c_parity . -run '^(TestPackage2ScalaFalsifierTrueEOFComposition|TestScalaEOFRecoveryNativeVersionTrace|TestScalaEOFRecoverySuffixDifferential)$' -count=1 -v
```

Run the full performance comparison from the candidate root with `--baseline-root` set to a clean `d25623f69` worktree.

```sh
GOWORK=off bash scripts/run_randomized_benchmarks.sh --baseline-root <main-worktree> --baseline-output <baseline-output> --output <candidate-output>
```
