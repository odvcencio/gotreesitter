# C3 sealed-share join note

Evidence-only note. It defines how C0e board ratios and C0f fleet shares may
be cited together without conflation, resolving the citation-side practice for
the unresolved conflict `sealed-share-join` in
`docs/c0e-c0f-attribution.md` (line 53):

> The sealed board contains ratios and A/A nulls, but no component profile.
> The local C0 profile is a different host and diagnostic instrument. Keep C0e
> ratios and C0e noise evidence separate from C0f fleet shares.

## The two instruments

| | C0e sealed equal-fixture board | C0f fleet board |
|---|---|---|
| Epoch / commit | `strictboundary-20260802T062212Z-v9`, commit `492cd600…` (`docs/c0e-c0f-attribution.md` line 11) | V10 scoreboard, revision `5003ffba…` (`docs/v10-fleet-manifest.md` line 5; also `docs/v10-fleet-manifest.md` lines 5–8) |
| Fixture scope | 4 compact Go/C fixtures (`docs/c0e-c0f-attribution.md` lines 13–18) | 1,315 ratio-eligible signal rows across 206 languages (`docs/v10-fleet-manifest.md` line 10; `docs/c0e-c0f-attribution.md` line 28) |
| What it publishes | Per-fixture ratios and a geomean (3.986x) | Class totals, shares, distributions, cohort ceilings |
| Noise evidence | Local WSL2 A/A p95 floor 7.367%–10.803% (`docs/c0e-c0f-attribution.md` line 22) | None published for its own host |

## Citation rules

1. **Never derive a share from a ratio or a ratio from a share.** The C0e
   geomean 3.986x describes four named fixtures; the C0f ratio-by-total
   10.950130x describes fleet signal totals (`docs/c0e-c0f-attribution.md`
   lines 11, 28). They measure different populations on different hosts and
   must not be averaged, blended, or used to scale each other.
2. **Tag every number with its board.** Acceptable forms:
   "C0e(v9) fixture ratio 4.348x (`rewrite.go`)" and
   "C0f(V10) error-class Go share 72.2586%". Unacceptable: "the ratio is
   3.986x so recovery saves …".
3. **Keep each board's noise with it.** The 7.367%–10.803% A/A floor belongs to
   the local C0 host only; it may qualify C0e fixture deltas but never C0f
   fleet shares (`docs/c0e-c0f-attribution.md` line 22).
4. **A join sentence is permitted only in the form of adjacency, not
   arithmetic**: e.g. "C0e(v9) reports compact geomean 3.986x against its
   ≤2.0x target (geomean pass false); separately, C0f(V10) attributes
   72.2586% of signal Go time to the error class." Both clauses cite their own
   source; neither implies the other.
5. **No mechanism admission via join.** Citing a C0e ratio next to a C0f cohort
   share does not satisfy the `fleet-mechanism-facts` gate
   (`docs/c0e-c0f-attribution.md` line 54): runtime trace facts are still
   required before any cohort's projected_saved_go_ns numerator is evaluable.
6. **Byte denominators stay separate too.** The R1 39.0% error-byte share and
   the F0 signal-byte 32.2343% (84,094,345 / 260,884,819) must both be
   published with their own denominators and never reconciled by fiat
   (`docs/c0e-c0f-attribution.md` line 56).

## Worked examples

Each example states the sentence form, why it passes or fails, and its anchors.
All values are quoted unchanged from their own boards
(`docs/c0e-c0f-attribution.md` lines 11, 15, 20, 22, 28, 33, 48, 54, 56).

### Correct claims

**C1 — C0e ratio cited alone, board-tagged.**
> "C0e(v9) reports fixture `rewrite.go` at compact Go/C **4.348x**
> (`docs/c0e-c0f-attribution.md` line 15), against the sealed target of
> ≤3.0x per fixture (line 20)."

*Passes:* single board, named fixture, ratio kept as a ratio, target cited
from the same board. No share is implied.

**C2 — C0f share cited alone, denominator explicit.**
> "C0f(V10) attributes **72.2586%** of measured signal Go time (436,257,955,255 ns
> over 1,315 rows) to the error class (`docs/c0e-c0f-attribution.md` lines
> 28, 33)."

*Passes:* the share names its denominator (signal Go ns) and row population,
both from the C0f board.

**C3 — Adjacency join, no arithmetic.**
> "C0e(v9) reports a compact Go/C geomean of **3.986x**, failing its ≤2.0x
> target (geomean pass false, `docs/c0e-c0f-attribution.md` lines 11, 20);
> separately, C0f(V10) attributes **72.2586%** of signal Go time to the error
> class (line 33). The two statements concern different hosts and populations;
> neither scales the other."

*Passes:* rule 4's permitted form. Each clause cites its own board; the final
sentence restates non-derivability rather than computing anything.

**C4 — Byte denominators published side by side, unreconciled.**
> "R1 reports a **39.0%** error-byte share under an R1-defined byte
> denominator; F0 signal bytes give **32.2343%** (84,094,345 / 260,884,819)
> (`docs/c0e-c0f-attribution.md` line 56). Both are published with their own
> denominators; they are not reconciled here."

*Passes:* rule 6. Two byte figures coexist without blending because their
denominators differ and one is undefined in this artifact.

### Incorrect claims

**W1 — Deriving a fleet share from the C0e geomean (arithmetic conflation).**
> ✗ "The compact geomean is 3.986x and the error class holds 72.2586% of Go
> time, so error-class code is 3.986 × 0.722586 = **2.8802x slower** overall."

*Fails:* rules 1–2. It multiplies a four-fixture C0e(v9) geomean by a C0f(V10)
fleet share — different hosts, different populations. The product
**2.8802x** is meaningless and must not appear in any document. No source
supports it.

**W2 — Borrowing the C0e noise floor to qualify a C0f number.**
> ✗ "The C0f error-class share 72.2586% carries ±7.367%–10.803% measurement
> noise."

*Fails:* rule 3. The 7.367%–10.803% p95 A/A floor is a local WSL2 C0-host
result and is explicitly not a sealed-v9 or fleet noise floor
(`docs/c0e-c0f-attribution.md` line 22); per `campaign/c2-hygiene-provenance-plan.md`
(item 3) the fleet host's noise floor is unpublished until its own A/A receipt
attaches. Attaching the local floor to a C0f share fabricates provenance.

**W3 — Using a join to admit a mechanism or claim the selection ceiling.**
> ✗ "Since the error class is 72.2586% of signal and C0e shows ~4x compact
> ratios, the recovery cohort clears the 2% selection threshold and may be
> credited."

*Fails:* rule 5. The 0.02 selection formula needs an evaluable
`projected_saved_go_ns` numerator from runtime trace facts (retry counts,
recovery-cost counters), which V10/F0 do not contain
(`docs/c0e-c0f-attribution.md` lines 48, 54; conflict `fleet-mechanism-facts`).
Adjacency cannot substitute for the trace lane, and no performance credit may
be claimed before a C0f receipt exists.

**W4 — Dropping board tags so the two totals read as one scale.**
> ✗ "The boards show ratios of 3.986x and 10.950130x, i.e. roughly a 4x-to-11x
> improvement range across the project."

*Fails:* rules 1–2. 3.986x is a four-fixture compact geomean on C0e(v9)
(line 11); 10.950130x is a fleet-wide ratio-by-total over 1,315 signal rows on
C0f(V10) (line 28). Presenting them as points on one range implies a common
population that does not exist.

**W5 — Reconciling byte shares by fiat.**
> ✗ "The 39.0% R1 byte share and the 32.2343% F0 byte share agree within
> rounding after hygiene adjustments."

*Fails:* rule 6 and the unresolved `error-byte-denominator` conflict
(`docs/c0e-c0f-attribution.md` line 56): R1's byte denominator is undefined in
this artifact, so no reconciliation — including "within rounding" — is
licensed.

**W6 — Arithmetic blend of a C0e ratio with a C0f share.**
> ✗ "**3.986x × 72.2586%** gives the fleet-wide error-class ratio."

*Fails:* rule 1 (never derive a share from a ratio or a ratio from a share).
3.986x is a C0e(v9) four-fixture geomean and 72.2586% is a C0f(V10) fleet
share; multiplying them blends two boards, hosts, and populations into one
meaningless product.

**W7 — Transferring the C0e noise band onto fleet shares.**
> ✗ "Apply the 7.367%–10.803% band to the 72.2586% error-class share, giving a
> corrected fleet share range."

*Fails:* rule 3 (keep each board's noise with it). The band is the local C0
WSL2 A/A p95 floor and qualifies only C0e fixture deltas; the C0f fleet board
publishes no noise floor for its own host.

**W8 — Blending byte denominators.**
> ✗ "Combining R1's 39.0% error-byte share with F0's 84,094,345 / 260,884,819
> bytes yields an overall error-byte share of about 35.6%."

*Fails:* rule 6 (byte denominators stay separate). The two figures use
different — and in R1's case undefined-in-this-artifact — denominators, so any
combined byte share fabricates a denominator that no board publishes.

## Join table template

Any document citing both boards should use rows of this shape:

| Claim | Board | Value | Source anchor |
|---|---|---|---|
| Compact equal-fixture geomean | C0e(v9) | 3.986x | `docs/c0e-c0f-attribution.md` line 11 |
| Fleet signal ratio-by-total | C0f(V10) | 10.950130x | `docs/c0e-c0f-attribution.md` line 28 |
| Error-class Go share of signal | C0f(V10) | 72.2586% | `docs/c0e-c0f-attribution.md` line 33 |

Rows from different boards may appear in one table but never combine into a
computed column.
