# Go identifier reuse: bounded admission and proof cost

The narrowed native-Go reuse prototype removes the earlier R4 token and reuse
regressions, and eliminates 3,528 of 3,543 added action-table lookups per clean
edit. **Keep the prototype held.** Four complete operations still add 15
lookups each. The fresh verifier remains active; this is correctness and work
qualification, with no new throughput result, runtime promotion, ledger reset,
compact graduation, or main merge.

The baseline is `9075463238e9f94e0091cfa938cb9e259a4d489e`. The current downstream
reference remains [Cognociente's v0.56.0 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850),
the latest of 41 issue comments when checked. The original declaration witness
is 140,368 bytes, with SHA-256
`5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`.
These Linux/amd64 checks do not measure the report's Windows latency.

## Qualified shadow: V5

The earlier native-producer contract now admits only a bounded, surviving ASCII
identifier. It checks the declared edit against both complete buffers, probes
the real DFA in isolated lexer copies, and rejects keyword reclassification.
The old token must belong to the actual native producer. Producer, source,
read-history, parse-state, recovery and publication guards remain separate.
Compact-produced old trees remain outside this contract.

`Tree.Edit` uses the same necessary old-token condition to select stronger
lookahead invalidation. Padding and punctuation edits retain their established
policy. Newly inserted suffix bytes cannot serve as an old-tree anchor; the
adapter anchors in a surviving old byte instead.

A per-parse memo stores only immutable action-table facts for the last exact
state/symbol pair. Every node-specific eligibility check still runs. The memo
clears between parses and preserves negative facts. It does not cache a reuse
decision or grant publication authority. New policy code lives in `internal/incr`;
the engine adds no language-name comparison.

## Deterministic work

All four actual Go/YAML default/candidate ledger exports now match the baseline
byte for byte, including full parse and first edit. The earlier broad shadow is
retained in the [previous receipt](../20261025-go-reuse-shadow/README.md).

| Default Go R4 first edit | Baseline | Earlier broad shadow | V5 |
| --- | ---: | ---: | ---: |
| Tokens | 572 | 614 | 572 |
| New nodes | 3,810 | 3,219 | 3,810 |
| Maximum live versions | 12 | 8 | 12 |
| Multi-version token share, ppm | 531,468 | 495,114 | 531,468 |
| Reused bytes | 1,166 | 1,044 | 1,166 |
| Block splices | 296 | 260 | 296 |

The unchanged complete-operation capture measures fresh parse plus release, or
`Tree.Edit` plus profiled incremental parse and both releases, including hidden
verification and retry work. Declaration and assignment fixtures exercise
insert, replace and delete on both routes: 24 records per arm.

| Four clean native insert/replace operations | Baseline | V4, before memo | V5 |
| --- | ---: | ---: | ---: |
| Complete-operation table lookups | 345,393 | 348,936 | 345,408 |
| Incremental-attempt table lookups | 12,526 | 16,069 | 12,541 |

The remaining increase is **0.0043%** of complete-operation lookups and **0.120%**
of the incremental attempt. All other primitive work counters match. The other
20 complete work records match exactly: 12 fresh operations, six compact-route
edits, and both native malformed deletes. Selected reuse counters also match.
Arena accounting still differs in selected profiles; those values are neither
benchmark B/op nor a memory improvement. Every difference and every attempt's
phase partition remains in [summary.json](summary.json) and the raw logs.

The standard ledger's 2% failure tolerance is satisfied, but D10's work
direction remains unresolved. No owner exception is inferred and no randomized
timing is claimed. The existing assignment-recovery approval covers different
changes. V6 hoists an unconditional no-lookahead rejection ahead of the table
lookup; it passes its release checks but changes none of these work records,
so it remains a separate diagnostic.

## Correctness and resource checks

- V5 passes both 72-edit clean chains on the original witness, with 144 actual
  verifier events and 144 proposed certificates. Incoming and final native
  trees match fresh structure, states and internal precedence. Every no-edit
  check allocates zero objects.
- Both ordinary 72-step Go R4 sessions pass. Twelve token-boundary cases cover
  insert/replace/suffix admission and rejection of keywords, removed tokens,
  punctuation, whitespace, Unicode, split tokens, literals and oversized names.
- Another 108 edits span four real Go sources. The proposed certificate
  activates in 62 cases, each requiring strict incoming/fresh equality. Every
  returned result matches an independent fresh parse.
- The six producer classes and 19 actual producer-context dimensions pass.
  Copy, transformed factories, wrappers, compact producers and absent artifact
  capabilities never acquire the new certificate.
- V5 passes the same three C families / 127 records against both locked C
  0.25.1 and pinned C 0.27.1 (`10eb5ac119eec9cb4267a8e035066473983f5f3a`).
  This includes malformed declarations and 96 assignment-target edits. The
  C027 header/link probe and unchanged binding-body guards pass.
- V4 passes the full chained-edit race run. V5 separately passes race checks
  for the memo, real-source edits, token boundaries and producer controls.
  The memo change also repeats both complete clean chains in the release build.
- Every container uses one pinned CPU, 4 GiB, no network and read-only project
  sources. Exit/OOM metadata and before/after source/control guards are retained.
  No container reports OOM. This does not establish the large-file RSS gate.

V2's conservative suffix-admission failure and the first C027 mount setup
failure remain archived. Neither was a returned-tree mismatch. No assertion
was weakened to admit the suffix: the adapter's anchor was corrected, and the
same boundary test passed afterward.

The next architectural step is qualification of a certificate-backed path that
can avoid the duplicate full parse. Full-fleet counters and memory, large-file
RSS, native Windows/ARM, compact-producer ownership, and the E-A/M1 graduation
gates remain open. The present evidence does not authorize bypass or graduation.

## Reproduce the review

The [archive](evidence.tar.gz) contains the frozen overlays, exact Docker
commands, tests, raw counters, failures and successful logs. Its
[index](contents.json.gz) hashes every member. The two pinned inherited archives
in [summary.json](summary.json) supply the already-published C027 adaptation.
No cache, binary, credential or unrelated file tree is included.

Validate the recorded evidence without executing archived Go or C code:

```sh
python3 docs/benchmarks/20261026-go-identifier-admission/validate-evidence.py --source-root "$PWD"
```

[Offline validation result](verification.json). Main merges remain held.
