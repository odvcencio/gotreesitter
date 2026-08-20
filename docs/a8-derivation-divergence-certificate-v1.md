# A8 derivation divergence certificate v1

Status: evidence certificate

This certificate records the accepted D0 candidate-set evidence.
It does not change parser behavior or grant a compatibility exception.

## Current re-cut: 2026-08-13

The baseline below is historical. The current constructed D0 run uses the
same 104-source and 95-compared denominator, but reports 15 extra, 1 missing,
and 5 different derivations. It reports 21 total differences, with 13
condense-class, 4 token-class, and 4 unattributed mechanisms. The order axis
remains zero.

The full current corpus contains 113 sources and compares 100. It reports 15
extra, 1 missing, and 6 different derivations, for 22 total differences.

The focused D1 arrival-order counterexample remains green and falsifies an
arrival-order-only collapse. Apex and Objective-C require opposite C survivor
positions. Do not implement that collapse.

The current M0 witness reproduction reaches all nine witnesses. Two Ada
expected `DIFFERENT` classifications are stale; the current run reports
`EXTRA` only. Keep the test pin unchanged until the evidence owner accepts the
classification correction.

The current run used gotreesitter revision
`240c5ef17fec3403b33eeca0c7b81abf5370a5a0`. The test exited nonzero for the
stale constructed total and the two stale Ada expectations. It reported no
out-of-memory event and no wall timeout. This is evidence drift, not a parser
route change.

Receipts:

- D0 Docker artifact: `20260813T101032Z-a8-d0-baseline-current`
- Current D0 Docker artifact: `20260813T153029Z-a8-d0-current-route-v2`
- Current D0 log SHA-256: `886b554722647381597b7af113f3cc5473c8751aeb68cfb5ab70de6bee7d6d0b`
- Current D0 inspection SHA-256: `d567cc2393533584e1685e98abfc0503e79abd4c1e67db6017b236858b319290`
- Current D0 metadata SHA-256: `1bf61f39bd4eb512552019b4b8ab4bf36f4e4c724ebdb792b9b7a66173436330`
- D1 Docker artifact: `20260813T100813Z-a8-d1-current`
- M0 Docker artifact: `20260813T100936Z-a8-m0-witness-current`

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

## Historical baseline certificate

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

These historical findings do not authorize the D1 collapse. The current
order of work is:

1. Keep arrival-order collapse retired by the counterexample.
2. Keep the token-class mechanism separate from condense analysis.
3. Re-run D0 after an accepted generic mechanism changes the candidate set.

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

Do not implement D1 from the historical count. Keep D2 separate so token and
condense effects remain attributable if a generic mechanism is later admitted.

## Certificate boundary

This certificate establishes the first divergence mechanism split.
It does not establish zero divergence.
It does not authorize a parser route change.
It does not authorize a language-specific exception.

The next receipt must include the exact D0 command, source revision, corpus digest, and full difference table.

## Current-head recheck

The current-head Docker recheck used commit
`7cf1cb3d5b65ad877bb8d2143aa049d1fd174af8` and the D0, D1 safety, and
singleton-version tests. It produced the current recut already described
above:

- constructed corpus: 104 sources, 95 compared, 15 extra, 1 missing, 5
  different, 0 order, and 21 total set differences;
- full corpus: 113 sources, 100 compared, 15 extra, 1 missing, 6 different,
  0 order, and 22 total set differences;
- mechanism counts: 13 condense-class, 4 token-class, and 4 unattributed on
  the constructed corpus;
- order-undetermined records: 5.

The D1 arrival-order counterexample and singleton-version tests passed. The
D0 witness and baseline tests exited nonzero because their committed
expectations still contain the historical Ada classifications and the old
constructed total of 32. The output identifies two Ada classifications that
now report `EXTRA` instead of `EXTRA DIFFERENT`. This is evidence drift, not a
parser route change.

Run artifact:
`harness_out/docker/20260813T183757Z-a8-current-head-v1/`.

- Container log SHA-256: `6510729df18f22b2c366d34854e8d3ea3aa06f7239e0b11091f91c55b5a228bf`.
- Metadata SHA-256: `968e4a69547a8bcae22088e8039f73e4df6d7e3f055aed1ea18aff51bbc3e9e6`.
- Inspection SHA-256: `195b4a52bb06d71f1f91e8f0d73addd29685bacefbd10c36d1d1127097c73ac3`.

Keep the historical test pins unchanged until the evidence owner accepts the
recut. Do not use this receipt to admit B15 grants, change routing, or claim
performance credit.
