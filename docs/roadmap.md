# Roadmap

gotreesitter v1 is one engine, the certified core in `internal/ccore`, serving all 206 grammars with trees that equal pinned C (tree-sitter 0.27.1).
The work has three stages: certify grammars one at a time, delete the two older engines, then release v1.

The [v1 design](v1-design.md) defines the rules and gates.
This page says where the work stands, what is left, and how progress is measured.
Counts below were taken on `main` at commit `bfb2983e2`; each row names the file to recount from.

## Where we are

| Measure | Now | Target at v1 | Source |
| --- | --- | --- | --- |
| Certified grammars | 71 of 206 | 206, less any owner-approved known differences | [`testdata/ccore_certificates.json`](../testdata/ccore_certificates.json); [`internal/ccore/certified/gen.go`](../internal/ccore/certified/gen.go) |
| Certified top-50 grammars | 35 of 50 | 50 | the same files, and the [top-50 membership](v1-design.md#top-50-membership) |
| Root Go files | 621 | 120, then 25 | [`cmd/v1layout/root_files.txt`](../cmd/v1layout/root_files.txt); `rootBudget` in [`cmd/v1layout/main.go`](../cmd/v1layout/main.go) |
| Stale grammar blobs | 192 | 0 | [`cmd/ts2go/testdata/blob_freshness_allowlist.txt`](../cmd/ts2go/testdata/blob_freshness_allowlist.txt) |
| Known differences from the C oracle | 20 | 0, or owner-approved | [`cgo_harness/c_oracle_known_differences_test.go`](../cgo_harness/c_oracle_known_differences_test.go) |
| Language-name comparisons in engine files (D6) | 285 rows | 0 | [`cmd/v1guard/language_allowlist.txt`](../cmd/v1guard/language_allowlist.txt) |

A certificate says that the core serves a grammar's blob by default, in the listed modes (fresh, recovery, scanner, incremental, ranges), because the grammar passed the [graduation gate](v1-design.md#graduation-gate-one-language) against pinned C.
A grammar without a certificate runs on the older engines.
Today every certificate lists the fresh, recovery and incremental modes. None lists included ranges, so an included-range parse still runs on the legacy engine for all 71 grammars.

Two older engines remain:

- The **compact scheduler** (`internal/parsercorephase0` and the root `parsercore_phase0*.go` files) is frozen and off by default.
- The **legacy engine** is the root GLR parser. It still serves every uncertified grammar and every decline from the core.

## Stage 1: certify grammars

Each grammar passes the graduation gate on its own, and its certificate lands in its own PR.
Certificate-only grammars and scanner ports can proceed beside the table work.

### The remaining 15 of the top 50

| Grammar | What blocks it |
| --- | --- |
| SQL, Julia, Nix, SCSS, XML | Gate rerun on current `main`, then a certificate. Nix is a forest-route grammar and needs the Forest bit. Julia has a query-capture gap. XML and SCSS have stale blobs. |
| Go | Table equality with upstream's grammar, then a blob swap that touches the shipped Go grammar. The blob swap is approved; the release follows Go's certificate. |
| Swift | Table equality with an oracle built from the newest generator and the upstream grammar, a scanner port, and an end-of-input gap. |
| Haskell, Perl, Scala, Svelte, Vue, CMake, MATLAB | Scanner port. None of these scanners is registered with the core's scanner layer yet. Scala and Vue also have query differences. |
| PowerShell | Policy, not code. It waits until the core reuses after an error edit as pinned incremental C does. |

### The other 120 grammars

Each is either certified or recorded as a known difference.
A known difference needs a minimal witness, the blocking mechanism traced to a table or grammar property, the reason no generic mechanism serves it, and the repository owner's approval ([D7 and the known-difference rule](v1-design.md#rules)).
The blocker class of each grammar comes from its gate run. The usual classes are a scanner port, table equality with the upstream grammar, a stale blob, and a query difference.
The 192 stale blobs are the largest single queue; a certificate for a grammar on that list comes with its regenerated blob.

Lean 4 is opt-in and needs an admission census run before it joins the count.

## Stage 2: delete the older engines

Deleting an engine is an owner decision. The order is below; each step ends when the listed check passes.

1. **Record which engine serves each case.** Add a test that lists, for each certified blob, entry point and option, whether the core or the legacy engine served it. Ratchet it so the legacy cases only shrink.
2. **Make the core total for a certified blob.** Today an included-range parse, a parse with an observer or work limits, and a stop (deadline, cancel, memory, depth) fall back to legacy. One PR per reason turns each fallback into a core result. Order: a ranges item in the gate and the Ranges bit, core stop results and work limits, observers, then reuse after an error edit.
3. **Delete the compact scheduler.** It is off by default with an empty allowlist. The `gts_no_parsercorephase0` build already compiles it out; `cmd/v1layout/api/gts_no_parsercorephase0.txt` lists the API lines that go with it.
4. **Delete the legacy arms of certified grammars.** Possible once step 2 makes them unreachable and the gate's baseline no longer depends on the legacy engine's default route.
5. **Delete the legacy engine.** The design allows this only after every grammar it supports has graduated ([v1 design](v1-design.md#rules)).
6. **Build the root facade.** Generated defined types and forwarding methods (D9) replace the root engine files; the public `Node` must stop being the legacy node first.

Root file targets, which `cmd/v1layout` enforces as ceilings that only fall (see the [repository map](repository-map.md#root-file-limits)):

| Root `.go` files at most | After |
| --- | --- |
| 621 | now (`rootBudget`) |
| 550 | `tests/regress/` and a corpus store exist; generated grammar shims are merged |
| 500 | an existing engine subsystem moves into `internal/` |
| 120 | the legacy engine files and their tests are deleted |
| 25 | only the generated facade, examples and a contract test remain |

Two choices stay with the owner because they decide how much of the legacy lexer survives: whether v1 keeps ordinary `TokenSource` parses and `ParseForestExperimental`, and what the "global default flip" of D7 means once certificates route grammars one at a time.

## Stage 3: v1

The [v1.0 release gate](v1-design.md#v10-release-gate) is the list. In short:

- Every top-50 grammar certified, and the legacy engine deleted.
- Every other grammar certified, or recorded as an owner-approved known difference.
- The D13 speed targets met: engine floor for all 206, tuned for the top 50.
- Subtree reuse for every top-50 grammar, and 99th-percentile edit latency at most 20 ms at 137 KiB.
- Recovery parity on at least 60 of 79 cases, and the same `HasError` result as C.
- The v1 API, the versioning policy, and grammar format v1 published.
- The dashboard for all 206 grammars published.
- Tests passing on the six platform targets.

Three of these items have no recurring measurement yet: the recovery board, the dashboard, and the D13 speed targets for the core.
Some wording in the design predates certificates: the baseline for gate items 6 and 7 is defined by a release tag that does not exist, and a sentence says no public API reaches the core. Changing gate wording needs the owner, so this page does not change it.

## How progress is measured

Progress is the count of certified grammars, the shrinking of the lists above, and the nightly runs.
Recount each number from its source file rather than from this page.

- **Certified grammars and certified top-50 grammars**: the certificate file, which rises as each certificate lands.
- **Root Go files**: `go run ./cmd/v1layout`; `rootBudget` falls with each move.
- **Stale blobs and known differences**: their allowlists may only shrink.
- **D6 language-name rows and environment rows**: `go run ./cmd/v1guard` against its baselines.
- **Counters before timing**: work counters must not rise and reuse counters must not fall (D10). A PR may move a guarded counter only under the [R3 rule](v1-design.md#r3-counter-ledger-changes), and agents never approve an exception.
- **Nightly runs**: the fuzz and heavy-tier nights in a row that finish green, from the Actions history.

## References

- [v1 design](v1-design.md) for engine requirements and validation gates.
- [Repository map](repository-map.md) for file locations and ownership.
- [AGENTS.md](../AGENTS.md) for the required workflow and prose rules.
- [Release process](releasing.md) for publication procedures.
- [CHANGELOG.md](../CHANGELOG.md) and its [archive](changelog/) for shipped evidence.

Keep status in pull requests and their linked issues.
Attach reproducible evidence to PRs or release assets.
