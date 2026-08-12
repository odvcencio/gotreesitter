# A8 derivation divergence certificate v1

Status: evidence certificate

This certificate records the accepted D0 candidate-set evidence.
It does not change parser behavior or grant a compatibility exception.

## Authority

Use Revision R1 of `spec.campaign.v7` as the campaign authority.
Use `spec.derivation-set-equivalence.v1` as the mechanism specification.

The D0 instrument landed in PR [#646](https://github.com/odvcencio/gotreesitter/pull/646).
The merge commit is `8070e1ad51f4cfdb7acf6008ac7695cf7e97cd66`.

The instrument is inert in the default build.
It runs only with the `gts_derivation_set_census` build tag.

## Equivalence claim

At each accepted end-of-file event, compare these sets:

- Compact `D`: every live derivation through the compact link graph.
- C `V`: every root produced by every live C version and pop slice.

The required result is a bijection.
Each pair must preserve:

- Tree content and byte spans.
- Error cost.
- Dynamic precedence.
- Fold order.

Storage differences do not invalidate the claim.
Content differences invalidate the claim.

## Certified reconstruction

The C logger reconstructs `V` without patching the vendored C source.
It counts the root selected at each accept and subsequent contiguous selection records.

The compact side dumps `D` from the existing diagnostic derivation records.
The instrument reports candidate counts and difference classes.

The order axis remains undetermined when C logs identical root symbols without enough fold context.
The certificate records those cases as undetermined.

## Baseline certificate

The pinned constructed corpus contains 104 sources.
The instrument compares 95 sources.

| Difference class | Count |
|---|---:|
| Extra compact derivation | 16 |
| Missing compact derivation | 1 |
| Different derivation content | 15 |
| Order difference | 0 |
| Total classified difference | 32 |

Mechanism attribution covers 30 of 32 differences:

| Mechanism | Count | First target |
|---|---:|---|
| Condense-class | 26 | D1 condense-time tie collapse |
| Token-class | 4 | D2 first-leaf token cache |
| Unattributed | 2 | D0 follow-up trace |

The generated real-corpus extension contains 113 sources.
It compares 100 sources and reports 35 differences.

The certificate does not combine constructed and generated counts.
Each corpus remains a separate receipt.

## Witness findings

The apex witness is condense-class.
Both compact candidates share the same root and leaf sequence.
They differ in grouping.

The shipped Kotlin profile does not reach an accept-time candidate set.
The forced certification path reaches an equal-cardinality difference.
Keep this witness in the no-action and candidate-set lanes.

The Perl `join_bare` witness is token-class.
The compact tree misses the C `escape_sequence` leaf.

The other Perl witnesses share leaf content with C and remain condense-class candidates.

These findings change the order of work:

1. Prove safe condense-time tie collapse for comparable fork order.
2. Port the C first-leaf token reuse predicate.
3. Re-run the D0 census and classify the residual two cases.

Do not infer a mechanism from a ratio or from a language name.

## Downstream use

Use this certificate to select evidence work for four lanes.
Do not treat it as a completion receipt for any lane.

### A2 retirement

Retire an A2 arm only after its witness has equal compact and C candidate sets.
Require the existing byte-identity and route gates.

Do not retire an arm because its final tree happens to match.

### B15 proof

Use equal candidate sets as a prerequisite for generic derivation proofs.
Require leaf coverage, derivation-set equivalence, scanner quiescence, and recovery-record coverage.

Do not admit a profile grant from this certificate alone.

### B12 reuse

Permit incremental reuse only for spans whose candidate-set equivalence is proven.
Carry scanner state and collapse-class proof requirements into the reuse receipt.

### C7 merge alignment

Keep C7 conditional on the C0f alternative-lifetime cohort reaching two percent.
Require candidate-set equivalence before comparing merge triggers.

Do not claim performance credit from this certificate.

## D1 safety boundary

The committed D1 counterexample test
(`cgo_harness/condense_tie_arrival_order_counterexample_test.go`, commit
`689a1120`) falsifies an arrival-order collapse.
It compares two ties with the same fork provenance and opposite C survivors:

| Witness | Compact set | C set | C survivor position |
| --- | ---: | ---: | ---: |
| Apex `class_literal_alias` | 2 | 1 | first |
| Objective-C `protocol_argument` | 2 | 1 | second |

Both witnesses use an unstamped incumbent and an incoming link with fork
stamp 1. A rule that reads only that provenance must choose the same side.
One witness would therefore remain wrong.

The same test records a singleton C version set for Apex, Objective-C, Ada,
and Perl. The C runtime does not collapse two versions at these witnesses.
The extra compact candidate is therefore not an arrival-order problem.

The focused Docker run passed on 2026-08-12 at the current A8 source:

```text
cd cgo_harness
GOFLAGS=-buildvcs=false GTS_PARITY_ALLOW_HOST=1 \
  GTS_PARITY_C_REF_BUILD_CACHE=$PWD/../harness_out/parity_c_ref_cache \
  CGO_ENABLED=1 \
  go test -tags "cgo treesitter_c_parity gts_derivation_set_census" \
  -run '^TestCondenseTieArrivalOrder' -count=1 -v -timeout 40m .
```

The run passed in 15.113 seconds without an out-of-memory kill or wall
timeout. Keep D1 evidence-only until a mechanism explains the missing C
version, rather than its apparent fold order.

## Gates for the next A8 tranche

The next D1 or D2 change must pass these gates:

- The D0 instrument reports fewer classified differences.
- Unproven fork order continues to fail closed.
- The five A3 sweeps report no unadjudicated divergence.
- Canonical tree digests remain unchanged.
- The 206-row admission scorecard keeps zero divergence.
- The material-acceptance census does not increase.
- The smallest Docker parity suite remains green.

Run D1 before D2 because D1 owns 26 of the 32 classified differences.
Keep D2 separate so token and condense effects remain attributable.

## Certificate boundary

This certificate establishes the first divergence mechanism split.
It does not establish zero divergence.
It does not authorize a parser route change.
It does not authorize a language-specific exception.

The next receipt must include the exact D0 command, source revision, corpus digest, and full difference table.
