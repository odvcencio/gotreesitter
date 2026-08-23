# Issue #454 compact-parser correctness fix

Status: **GO for publication**. The candidate fixes the recorded locked-C
difference. Do not close issue
[#454](https://github.com/odvcencio/gotreesitter/issues/454) until this change
merges and main CI passes.

## Scope

The audit uses base commit
`60b7b41c06443627ad063497f974ef50aca2fa85`.

The candidate diff SHA-256 is
`71fdb2ab00f8f31e74b7e165f381c0856bd3720abdeb4d1556454d0cc75c50fa`.
The candidate changes exactly these files:

- `parser_recover_c.go`
- `cgo_harness/issue454_c_compact_blocker_parity_test.go`

The receipt patch changes four files. It updates this document and the changelog.

The patch changes no grammar registry or grammar blob. The deterministic C
witness uses a repeated source with 140,288 bytes. The edit removes the first
`x` from `x0`. The edited source has 140,287 bytes.

## Producer proof

The first difference came from generic recovery materialization.

1. `cRecoverDispatchInError` calls `cAbsorbTokenIntoError`.
   The absorbed visible leaf now remains clean. The enclosing `ERROR` node
   still reports an error.
2. `cRecoverStrategy1Election` calls `cRecoverToState`.
   Hidden nodes with field metadata now use the field-preserving flatten path.
   The materializer copies field identifiers and field sources into the new
   recovery node.

Direct no-cache Canopy queries confirmed both call paths. Other recovery
entry points remain unchanged.

## Locked-C evidence

Before the fix, the first difference was:

```text
/translation_unit/function_definition[0]/compound_statement[2]/ERROR[2]/number_literal[0]
category=error Go=true C=false
```

After the fix, the fresh Go tree matches locked C at every tested size:

| Source size | Result |
| --- | --- |
| 1 KiB (1,024 bytes) | Exact tree parity |
| 4 KiB (4,096 bytes) | Exact tree parity |
| 16 KiB (16,384 bytes) | Exact tree parity |
| 64 KiB (65,536 bytes) | Exact tree parity |
| 137 KiB (140,288 bytes) | Exact tree parity |

The audit-only five-size guard passed in Docker. Its artifact is
`/tmp/gts-issue454-independent-artifacts/20260823T003842Z-five-sizes`.

The committed 1 KiB guard is
`TestIssue454COneKiBLockedCParity`.
Its artifact is
`/tmp/gts-issue454-independent-artifacts/20260823T004435Z-committed-1k`.

## Incremental evidence

`TestIssue454CIncrementalDeleteMatchesFresh` passed its replace, insert, and
delete subtests. Each incremental tree matched its fresh tree.

The replace and insert cases retained old-tree reuse. The delete case retained
the fail-closed reason
`incremental_parse_memory_budget_full_retry`.

The artifact is
`/tmp/gts-issue454-independent-artifacts/20260823T003852Z-incremental-edit-classes`.

The focused recovery and field tests passed in
`/tmp/gts-issue454-independent-artifacts/20260823T003826Z-parser-core-unit`.

The direct grammargen-to-C preset passed 20 of 20 cases. Its artifact is
`/tmp/gts-issue454-field-followup-audit-20260823/harness_out/grammargen_cparity/20260822_174020-focus-c`.

The C real-corpus preset reported 23 of 25 no-error cases and 20 of 25 deep
parity cases. The unmodified base reported the same counts and three type
divergences. This is a known baseline result, not a regression from this fix.

## Controlled memory audit

The audit used three alternating base and candidate order pairs. Each pair ran
one C language in one Docker container. The container used 8 GiB memory, one
CPU, 512 process IDs, and one Go test worker. The command used 25 cases and a
15-minute timeout.

| Pair | Base RSS | Candidate RSS | Candidate minus base |
| --- | ---: | ---: | ---: |
| 1 | 566,680 KiB | 609,464 KiB | +42,784 KiB |
| 2 | 628,852 KiB | 606,980 KiB | -21,872 KiB |
| 3 | 593,812 KiB | 620,544 KiB | +26,732 KiB |

The base mean was 596,448 KiB. The candidate mean was 612,329 KiB. The
candidate mean was 2.66% higher. The pair results include one lower candidate
run. The earlier 1,089,364 KiB candidate result did not reproduce.

The focused workload used `TestIssue454CIncrementalDeleteMatchesFresh`. It
passed the replace, insert, and delete cases. Without heap profiling, RSS was
857,624 KiB for base and 842,900 KiB for candidate. A paired heap-profile run
measured 858,400 KiB for base and 863,336 KiB for candidate.

The base `alloc_space` profile measured 1,180.79 MB. The candidate measured
1,167.62 MB. The base `inuse_space` profile measured 129.30 MB. The candidate
measured 128.66 MB. The arena breakdown was equal for both trees:

- Fresh field storage: 196,608 bytes, with 41,745 field identifiers and 41,745 field sources.
- Incremental-delete field storage: 393,216 bytes, with 83,472 field identifiers and 83,472 field sources.

The standard 20-seed primary Go trio was not run. The candidate changes only C
recovery paths. The focused C workload is the relevant performance gate.

No release-blocking memory regression reproduced. All Docker runs completed
without an out-of-memory failure or timeout.

The real-corpus logs are:

- `/tmp/gts-issue454-rss-audit/rep1-base/20260823T004613Z/real_corpus/diag_c_lang.log`
- `/tmp/gts-issue454-rss-audit/rep1-candidate/20260823T004629Z/real_corpus/diag_c_lang.log`
- `/tmp/gts-issue454-rss-audit/rep2-candidate/20260823T004646Z/real_corpus/diag_c_lang.log`
- `/tmp/gts-issue454-rss-audit/rep2-base/20260823T004702Z/real_corpus/diag_c_lang.log`
- `/tmp/gts-issue454-rss-audit/rep3-base/20260823T004719Z/real_corpus/diag_c_lang.log`
- `/tmp/gts-issue454-rss-audit/rep3-candidate/20260823T004917Z/real_corpus/diag_c_lang.log`

The focused run artifacts are:

- `/tmp/gts-issue454-rss-audit/20260823T005027Z-issue454-focused-baseline-timed`
- `/tmp/gts-issue454-rss-audit/20260823T005043Z-issue454-focused-candidate-timed`
- `/tmp/gts-issue454-heap-audit/base/issue454.pprof`
- `/tmp/gts-issue454-heap-audit/candidate/issue454.pprof`
- `/tmp/gts-issue454-rss-audit/20260823T005555Z-issue454-arena-breakdown-baseline`
- `/tmp/gts-issue454-rss-audit/20260823T005611Z-issue454-arena-breakdown-candidate`

## Issue-closing condition

Keep issue #454 open during publication. Close it only after all conditions
pass:

1. Merge this candidate.
2. Pass main CI.
3. Keep the five-size locked-C guard green.
4. Keep the replace, insert, and delete guards green.
5. Preserve the memory-budget fallback as a separate performance concern.
