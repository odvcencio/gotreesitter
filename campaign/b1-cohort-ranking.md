# B1 cohort ranking — depth-blocking mechanism cohorts by size and leverage

Sources (every claim below cites one of these):

- **[C1]** `campaign/b1-depth-census.md` — fresh executed run, 2026-08-23, HEAD
  `af9ded2b`, all 48 currently-PASSING languages exercised at depth against the
  fixtures in `campaign/b1-depth-fixtures.md`; raw log
  `campaign/fixtures/b1/depth-census-run.log` (`PASS=34 DIVERGE=0 FALLBACK=14
  SKIP=0 ERROR=0 total=48`, log line 99).
- **[C2]** `docs/compact-route-coverage-census.md` — 2026-07-20 census over all
  206 registered grammars (153 FALLBACK broken into eleven `[mechanism=…]`
  classes; class-size table lines 118–121; burn-down discussion lines
  160–205, 271–291, 359–362).
- **[C3]** `campaign/smoke-census-summary.md` — summary of [C2] including the
  corrected interpretation that every `eof-byte-short-frontier` decline has
  exactly one derivation and fails only because the accepted head's boundary
  ends one byte short of the source.

## Scope note: two complementary rankings

The three named classes (`zero-width-shift`, `repetition-shift-class`,
`eof-byte-short-frontier`) come from the whole-campaign census [C2]. The fresh
depth run [C1] re-exercised only the 48 currently-PASSING languages and found
**none** of those three tags on any decline — the declines there carry newer,
finer shapes (`scheduler-frontier-shape`, `recovery-entered`) plus one
unclassified case. Both views are ranked below; they rank different things:

1. **Depth-blocked languages among the current PASS set** ([C1]) — what blocks
   us from converting PASS-at-smoke into PASS-on-real-files-today.
2. **Campaign-wide leverage** ([C2]/[C3]) — what converts the most of the 206
   grammars overall.

---

## Ranking A — languages blocked at depth among the 48-PASS set [C1]

| Rank | Cohort | Languages blocked | Members |
|--:|---|--:|---|
| A1 | `scheduler-frontier-shape` | 8 | cylc, dtd, earthfile, elixir, julia, nushell, purescript, v |
| A2 | `recovery-entered` (production error tree) | 5 | bitbake, chatito, disassembly, mermaid, templ |
| A3 | unclassified decline | 1 | ledger ("accepted compact root is incomplete … span=0..932 expected=1..932", log line 77; no `[mechanism=…]` tag) |

(Note: `b1-depth-census.md` says "6" for this bucket but enumerates five rows;
the row list is authoritative — 34 + 8 + 5 + 1 = 48.)

### Campaign-leverage overlay within Ranking A

- The 8-language `scheduler-frontier-shape` bucket is one uniform shape family:
  six share the same "converged-path reduction split no-action drop lacks
  alternative-set coverage" detail or its shorthand "no-action drop" (cylc,
  dtd, earthfile, elixir, purescript, v — [C1] rows 8, 12, 13, 15, 38, 46),
  plus julia ("unproved historical boundary resurrection") and nushell ("EOF
  recovery admission requires scanner quiescence"). One fix shape touches 6 of
  8 immediately.
- All declines are fail-closed: DIVERGE=0 [C1], so none of these is a
  correctness risk — pure upside.

## Ranking B — campaign-wide leverage over all 206 grammars [C2][C3]

| Rank | Cohort | Languages declined | Share of FALLBACK | Tractability per [C2] |
|--:|---|--:|--:|---|
| B1 | `zero-width-shift` | 33 | 21.6% | High — one uniform shape, confirmed to recur at real-corpus depth (line 119, 171) |
| B2 | `repetition-shift-class` | 27 | 17.6% | Medium — scheduler explicitly documents it as unimplemented (lines 120, 178, 291) |
| B3 | `eof-byte-short-frontier` | 90 | 58.8% | Certain but narrow — every decline is a single-byte trailing-newline frontier gap, exactly one derivation (lines 134, 160, 271; [C3]) |

Leverage rationale (per [C2] lines 199–205, 247, 359–362): although
`eof-byte-short-frontier` is the largest raw bucket, the depth check showed it
does **not** block real-file depth for flagship languages — the depth-persistent
mechanisms are `repetition-shift-class` and `zero-width-shift`. Hence B1/B2 outrank B3 despite B3's raw count.

---

## Recommended burn-down order

1. **A3 unclassified (ledger)** — cheapest, unblocks measurement itself: until
   this decline routes through `admission_census.go`'s
   `admissionCensusClassify`, every future census will have an unexplained cell.
   *Named first fix:* route the "accepted compact root is incomplete"
   exit path through `admissionCensusClassify` so it emits a `[mechanism=…]`
   tag; then re-run the manifest-driven matrix (no routing change to the
   decision itself). Evidence base: [C1] row 27; `campaign/b1-harness-gaps.md`
   G-3.
2. **A1 `scheduler-frontier-shape`** — biggest depth-blocked bucket (8/14
   declines), and 6 of its members share one verbatim detail. *Named first
   fix:* give the converged-path reduction split's no-action drop
   alternative-set coverage by blending the surviving derivation with the
   certified accept, so `accept_without_materialization` sees one certified
   accepted derivation. Smallest repro: the 99–233-byte fixtures for cylc,
   earthfile, purescript, v (`campaign/b1-depth-fixtures.md`); dtd (198 KB)
   is the stress case. Evidence: [C1] rows 8, 12, 13, 15, 38, 46.
3. **A2 `recovery-entered`** — 5 depth-blocked languages, all fail-closed
   error trees from "scheduler has no table action for the elected token".
   *Named first fix:* teach the compact route to admit the recovery-elected
   token's derivation (certify the production error-tree root as an accepted
   derivation rather than declining), starting from chatito (99 bytes, the
   minimal repro). Evidence: [C1] rows 3, 4, 10, 32, 42.
4. **B1 `zero-width-shift` (33)** — highest campaign-wide leverage per [C2]
   line 171. *Named first fix:* add the missing scheduler table entry for the
   uniform "generic zero-width shift" shape so the head advances without
   consuming input. Evidence: [C2] lines 119, 171, 284.
5. **B2 `repetition-shift-class` (27)** — explicitly documented as an
   unimplemented scheduler feature. *Named first fix:* implement the
   repetition-shift table action the scheduler already names in its decline.
   Evidence: [C2] lines 120, 178, 291.
6. **B3 `eof-byte-short-frontier` (90)** — largest but shallowest: every
   decline is a one-byte short accepted-head boundary on a single-derivation
   parse. *Named first fix:* when the accepted head's end offset is exactly
   source length − 1 and the final byte is a newline, extend the certified
   span to include it. Certain win, fixture-only scope [C2] lines 134, 271;
   deliberately last because it does not block real-file depth [C2] lines
   199–205.

## Verification hook

Every fix above is checkable with the existing harness, unchanged:
regenerate the manifest with
`campaign/fixtures/b1/run-depth-census.sh`, then

```
GTS_ADMISSION_REAL_CORPUS=1 \
GTS_ADMISSION_REAL_CORPUS_MANIFEST=campaign/fixtures/b1/depth-manifest.json \
GTS_ADMISSION_CENSUS=1 \
go test -tags gts_parsercorephase0 -run TestAdmissionCandidateRealCorpusMatrix -v .
```

and confirm the targeted rows flip FALLBACK→PASS with digest logged ([C1]'s
exact command). No code change is made by this document; it is evidence only.
