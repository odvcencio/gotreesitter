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
| Epoch / commit | `strictboundary-20260802T062212Z-v9`, commit `492cd600…` (`docs/c0e-c0f-attribution.md` line 11) | V10 scoreboard, revision `5003ffba…` (`docs/c0e-c0f-attribution.md` line 28; `docs/v10-fleet-manifest.md` lines 5–8) |
| Fixture scope | 4 compact Go/C fixtures (`docs/c0e-c0f-attribution.md` lines 13–18) | 1,315 ratio-eligible signal rows across 206 languages (`docs/v10-fleet-manifest.md` line 12; `docs/c0e-c0f-attribution.md` line 29) |
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

## Join table template

Any document citing both boards should use rows of this shape:

| Claim | Board | Value | Source anchor |
|---|---|---|---|
| Compact equal-fixture geomean | C0e(v9) | 3.986x | `docs/c0e-c0f-attribution.md` line 11 |
| Fleet signal ratio-by-total | C0f(V10) | 10.950130x | `docs/c0e-c0f-attribution.md` line 28 |
| Error-class Go share of signal | C0f(V10) | 72.2586% | `docs/c0e-c0f-attribution.md` line 33 |

Rows from different boards may appear in one table but never combine into a
computed column.
