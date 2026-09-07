# Issue 454 compact route default

Date: 2026-09-07.

## Summary

A downstream editor compared v0.52.0 with v0.48.1 on 137 KiB synthetic fixtures
([issue #454](https://github.com/odvcencio/gotreesitter/issues/454)).
Clean full parses ran 2 to 3 times slower on 41 of 48 grammars.
Seven grammars lost most incremental reuse.
A single-byte Go delete did not terminate.

This report reproduces those results on Linux and attributes them to one cause.
The compact candidate route was the process default for fresh full parses.
Pull request [#631](https://github.com/odvcencio/gotreesitter/pull/631) removed
the 64 KiB size decline on 2026-08-02. Files above 64 KiB then reached the
compact route for the first time in v0.49.0.
Forcing the production route with `GTS_ADMISSION_CANDIDATE=0` restores
v0.48.1-class results on every reported item.

The accompanying change makes the compact route opt-in.
It does not change compact parser graduation status.

## Environment and method

- linux/amd64 under Windows Subsystem for Linux, 20 logical CPUs, go1.25.1.
- Shared development host. Other work ran during some samples.
- Harness: `go run ./cmd/issue454bench <lang> 137 <full|replace|insert|delete>`.
- Fixtures: generated repetitive sources, 137 KiB, one language per process.
- Full parse: median of five parses in one process.
- Edits: one single-byte edit at the first near-top identifier, as in the report.
- Correctness: node count of the incremental tree against a fresh parse of the edited bytes.
- Every row is one process run. These are attribution samples, not paired benchmark evidence.

Measured revisions:

- `v0.48.1` at `bbd24cda`.
- `v0.52.0` at `22958710`.
- `main` at `a50f1532`, the base of this change.
- `fix`: this change, with `GTS_ADMISSION_CANDIDATE` unset.

## Full parse, 137 KiB, median milliseconds

| Grammar | v0.48.1 | v0.52.0 | main | v0.52.0, compact off | fix |
| --- | ---: | ---: | ---: | ---: | ---: |
| go | 70.5 | 138.1 | 150.2 | 93.3 | |
| rust | 57.1 | 140.4 | 137.5 | 73.8 | 69.9 |
| scala | 74.5 | 212.1 | 195.6 | 75.0 | 75.6 |
| cmake | 89.7 | 235.9 | 235.2 | 105.0 | 109.5 |
| toml | 43.4 | 94.5 | 100.0 | 57.4 | 56.7 |
| make | 48.8 | 91.4 | 90.3 | | |
| css | 35.6 | 79.2 | 88.6 | 41.6 | |
| scss | 33.9 | 75.7 | 85.6 | | |
| less | 58.7 | 126.4 | 173.1 | | |
| typescript | 63.4 | 116.3 | 111.7 | | |
| tsx | 51.0 | 106.5 | 109.5 | | |
| ini | 37.8 | 74.2 | 73.2 | | |
| diff | 24.8 | 54.0 | 54.4 | | |
| json | 44.7 | 105.5 | 108.2 | 56.0 | |
| hcl | 64.0 | 192.8 | 195.1 | 75.4 | |
| haskell (fixture has an error) | 97.1 | 102.3 | 116.8 | | |
| c | 124.4 | 107.1 | 111.2 | | |
| python | 180.5 | 197.8 | 199.1 | | |
| javascript | 98.0 | 94.7 | 88.4 | | |

The four flat grammars do not use the compact route for these fixtures.
That matches the seven flat grammars in the downstream report.

## Incremental edits, 137 KiB

Reuse is reused bytes as a percentage of the edited source.
Time is the incremental parse in milliseconds.

| Grammar, edit | v0.48.1 | v0.52.0 | main | fix |
| --- | --- | --- | --- | --- |
| toml insert | 100.0%, 3.1 ms | 62.5%, 61.1 ms | 62.5%, 60.3 ms | 100.0%, 3.7 ms |
| make insert | 100.0%, 3.7 ms | 29.9%, 56.1 ms | 29.9%, 57.0 ms | 100.0%, 13.2 ms |
| css delete | 95.1%, 3.9 ms | 68.0%, 26.2 ms | 95.1%, 5.0 ms | 95.1%, 6.2 ms |
| scss insert | 95.1%, 3.0 ms | 56.5%, 26.6 ms | 95.1%, 4.7 ms | 95.1%, 8.0 ms |
| typescript insert | 98.1%, 4.8 ms | 54.3%, 67.9 ms | 54.3%, 71.3 ms | 98.1%, 11.6 ms |
| ini insert | 97.3%, 10.4 ms | 0.0%, 58.2 ms | 0.0%, 57.8 ms | 97.3%, 17.1 ms |
| diff insert | 99.9%, 1.4 ms | 25.1%, 36.4 ms | 25.1%, 38.3 ms | 99.9%, 2.6 ms |
| json insert | 69.2%, 48.7 ms | 69.2%, 203.9 ms | 69.2%, 212.7 ms | 69.2%, 70.2 ms |

At v0.52.0 the `ini` refusal reason is
`old tree was compact-materialized without a scanner-quiescence proof`.
Every row above matched its fresh parse by node count.

## Go single-byte delete, transient syntax error

The downstream report calls this the item that blocks adoption.
The cost sits in the fresh parse of the edited bytes, not in reuse selection.

| Size | v0.48.1 incremental | v0.52.0 incremental | v0.52.0 fresh parse of edited bytes | fix incremental |
| --- | ---: | ---: | ---: | ---: |
| 2 KiB | 1.5 ms | 31.2 ms | 32.2 ms | |
| 4 KiB | 1.6 ms | 118.3 ms | 127.9 ms | |
| 8 KiB | 2.1 ms | 669.2 ms | 731.3 ms | |
| 16 KiB | 3.0 ms | 4,179.8 ms | 4,291.5 ms | 2.2 ms |
| 137 KiB | 11.4 ms (downstream) | no termination in 420 s (downstream) | | 26.2 ms |

With `GTS_ADMISSION_CANDIDATE=0`, v0.52.0 completes the 137 KiB delete in 15.5 ms.
With `GTS_ADMISSION_CANDIDATE=1`, the fix reproduces the 16 KiB blowup at 4,548.7 ms.
The opt-in switch therefore selects the compact route as documented.

## Small files, both routes on this change

At v0.48.1 the compact route already served inputs at or below 64 KiB.
This table checks whether the production default costs anything on those sizes.
Each cell is the median of seven full parses in one process.

| Grammar | 4 KiB production | 4 KiB compact | 16 KiB production | 16 KiB compact | 48 KiB production | 48 KiB compact |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| go | 3.7 ms | 4.9 ms | 8.6 ms | 17.2 ms | 23.0 ms | 49.2 ms |
| rust | 3.2 ms | 4.9 ms | 7.8 ms | 16.2 ms | 19.5 ms | 46.1 ms |
| json | 2.7 ms | 4.5 ms | 7.6 ms | 13.9 ms | 16.3 ms | 39.2 ms |
| toml | 2.8 ms | 4.2 ms | 5.4 ms | 11.8 ms | 15.5 ms | 33.5 ms |
| css | 2.5 ms | 3.5 ms | 6.5 ms | 10.1 ms | 14.0 ms | 28.6 ms |
| typescript | 3.8 ms | 4.0 ms | 8.0 ms | 12.5 ms | 19.3 ms | 38.5 ms |
| scala | 4.1 ms | 5.4 ms | 9.1 ms | 21.3 ms | 24.8 ms | 63.6 ms |
| python | 6.9 ms | 7.0 ms | 23.3 ms | 22.8 ms | 61.8 ms | 68.0 ms |

The production route is faster or equal at every measured size.
The compact-to-production ratio grows with input size, from about 1.0 to 1.6 at 4 KiB to about 2.0 to 2.6 at 48 KiB.

## Behavior that changes for inputs at or below 64 KiB

Inputs at or below 64 KiB used the compact route at v0.48.1 and now use the production route.

- Compact-only conflict policies no longer apply by default. The Markdown inline route
  carries four such policies from v0.52.0. Those inputs return the production tree,
  which is the v0.48.1 tree, because the compact route declined them before v0.52.0.
- Native compact error recovery no longer runs by default. The production recovery path
  serves error-bearing inputs, as it did at v0.48.1.
- Processes that opt in through `GTS_ADMISSION_CANDIDATE=1` or the API keep every v0.52.0 behavior.

## Attribution

- At v0.48.1, `admissionCandidateInputSizeEligible` declined inputs above 64 KiB.
  The 137 KiB fixtures used the production route.
- Pull request #631 removed that decline. The compact route was already the process default.
- v0.52.0 added native compact Go end-of-file recovery. The error-bearing fresh parse then stayed on the compact route instead of falling back to production.
- Tests that force the compact route or read its counters keep their coverage. The test binaries opt in when `GTS_ADMISSION_CANDIDATE` is unset.

## Production-route regression exposed by the default change

Running the grammars suite with `GTS_ADMISSION_CANDIDATE=0` exposed one production-route
regression that the compact default had masked since v0.49.0:
`static inline void f(int *v) {}` and every function definition with two consecutive
storage-class specifiers returned an ERROR node around the type.
The same test passes on both routes at v0.48.1.

A bisect names `02ee8546` (pull request #709, Apex class-literal election parity).
That change added a per-stack DFA re-lex to the plain multi-stack dispatch loop.
The GLR trace shows the mechanism:

1. At `inline`, the parser forks between a declaration-specifier reading and a
   constructor-specifier reading.
2. At `void`, the constructor fork has no action for `primitive_type`. Before #709 it died.
   The re-lex reads `void` as an `identifier`, the constructor name, so it survives.
3. At `f`, the constructor fork has no action again. The previous-shift recovery
   wraps `void` in an ERROR and keeps the fork alive next to the error-free sibling.
4. Both forks reach the same state at `*`, merge, and the recovered fork wins selection.

Tree-sitter C halts a version at a no-action point when a better version exists.
This change adds that rule before the previous-shift recovery: a sibling stack that
already shifted the lookahead, accepted the input, or carries an action for it is a
better version, so the failing stack falls through to the existing multi-stack kill.
`TestCppConsecutiveStorageClassSpecifiersProductionRouteMatchesCompact` pins five
witnesses on the production route against the compact route. The Apex parity suite
from #709 still passes on both routes.

## Remaining items outside this change

- TypeScript transient-error delete on the production route: 64.0 ms at v0.48.1, 318.5 ms at v0.52.0 with the compact route off.
  A bisect with a 2.5x incremental-to-full ratio threshold names `bf7ab056` (pull request #613, memory-budget fail-closed retry for incremental parse).
  The tree stays correct. The cost is a fail-closed full-parse retry.
- C transient-error delete at 137 KiB: 2.1 s with a wrong one-node tree at v0.48.1, 21.1 s at v0.52.0, 3.8 s on this change with a correct tree.
  The `incremental_parse_memory_budget_full_retry` route still explores about 3.2 million nodes before the retry. The C fallback attribution work on issue #454 owns this item.
- Production-route full parse costs 1.1 to 1.3 times v0.48.1 on several grammars, for example rust at 57.1 ms against 69.9 ms. This report does not attribute that residual.
- `ReusedBytes` exceeds the source length on the delete arm at both versions, for example 194.9 percent for Go. This is a profiler accounting defect, not a parse defect.
