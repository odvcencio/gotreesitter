# B15 stage 1 grant inventory and shadow receipt

Status: implemented

This receipt records the B15 stage 1 inventory at gotreesitter commit
`5d2924139b67a15570c1defb534391c57a4ba556`.

Revision R1 remains the operative authority. Decision D2 freezes all
admission exceptions. This receipt changes no grant, route, language name,
grammar identity, or parser code.

## Scope and result

- Inventory: 18 sites.
- Profile sites: 17 grants across 14 languages.
- Driver-local sites: one duplicate Go grant.
- Generic proof complete at a grant boundary: none.
- Shadow result: keep every site fail-closed until its named generic proof
  exists and passes the B0 and B1 gates.

The generic code already contains partial proof machinery. The profile grants
still permit admission when that machinery cannot prove the required property.

## Site receipt

| # | Site and mechanism | Exact grant path | Shadow class | Missing generic proof |
|---:|---|---|---|---|
| 1 | Go, converged split drop | `grammars/runtime_profiles.go:54` | profile / split-drop | Derivation-set equivalence and alternative-set containment for every dropped head. |
| 2 | Erlang, converged split drop | `grammars/runtime_profiles.go:58` | profile / split-drop | Derivation-set equivalence and alternative-set containment for every dropped head. |
| 3 | HTTP, EOF accept with dead siblings | `grammars/runtime_profiles.go:79` | profile / EOF frontier | A grammar-independent accepting-frontier proof, including exact EOF derivation and leaf coverage. |
| 4 | Robot, EOF accept with dead siblings | `grammars/runtime_profiles.go:83` | profile / EOF frontier | A grammar-independent accepting-frontier proof, including exact EOF derivation and leaf coverage. |
| 5 | HTML, strategy-2 recovery | `grammars/runtime_profiles.go:91` | profile / experimental recovery | Scanner quiescence, an exact recovery transition record, and leaf coverage. S3 remains experimental and fail-closed until this proof exists. |
| 6 | JavaScript, converged split drop | `grammars/runtime_profiles.go:107` | profile / split-drop | Derivation-set equivalence and alternative-set containment for every dropped head. |
| 7 | Kotlin, primary acceptance derivation | `grammars/runtime_profiles.go:227` | profile / primary election | A generic derivation-set proof that the selected primary is C-equivalent to every live secondary, independent of grammar identity. |
| 8 | Apex, primary acceptance derivation | `grammars/runtime_profiles.go:238` | profile / primary election | A generic derivation-set proof that the selected primary is C-equivalent to every live secondary, independent of grammar identity. |
| 9 | Python, converged split drop | `grammars/runtime_profiles.go:285` | profile / split-drop | Derivation-set equivalence and alternative-set containment for every dropped head. |
| 10 | Python, primary acceptance derivation | `grammars/runtime_profiles.go:286` | profile / primary election | A generic derivation-set proof that the selected primary is C-equivalent to every live secondary, independent of grammar identity. |
| 11 | Perl, converged split drop | `grammars/runtime_profiles.go:295` | profile / split-drop | Derivation-set equivalence and alternative-set containment for every dropped head. |
| 12 | Perl, primary acceptance derivation | `grammars/runtime_profiles.go:296` | profile / primary election | A generic derivation-set proof that the selected primary is C-equivalent to every live secondary, independent of grammar identity. |
| 13 | Ada, converged split drop | `grammars/runtime_profiles.go:306` | profile / split-drop | Derivation-set equivalence and alternative-set containment for every dropped head. |
| 14 | Ada, primary acceptance derivation | `grammars/runtime_profiles.go:307` | profile / primary election | A generic derivation-set proof that the selected primary is C-equivalent to every live secondary, independent of grammar identity. |
| 15 | Bash, converged split drop | `grammars/runtime_profiles.go:335` | profile / split-drop | Derivation-set equivalence and alternative-set containment for every dropped head. |
| 16 | Meson, primary acceptance derivation | `grammars/runtime_profiles.go:387` | profile / primary election | A generic derivation-set proof that the selected primary is C-equivalent to every live secondary, independent of grammar identity. |
| 17 | Haskell, converged split drop | `grammars/runtime_profiles.go:419` | profile / split-drop | Derivation-set equivalence and alternative-set containment for every dropped head. |
| 18 | Go, converged split drop | `parsercore_phase0_driver.go:6498` | driver-local / split-drop | The same derivation-set proof as site 1, plus removal of the driver-local grammar identity and scanner-type admission check. |

## Proof anchors and shadow interpretation

- Split-drop proof machinery exists in
  `parsercore_phase0_alternative_set_census.go:93` and is consulted by
  `parsercore_phase0_driver.go:5264`. A profile grant can waive a failed
  alternative-set proof, so this is not yet a generic admission proof.
- EOF acceptance currently permits the profile-controlled dead-sibling shape
  in `parsercore_phase0_driver.go:4203`. The shape check does not prove C
  derivation equivalence or leaf coverage.
- Primary election uses a profile-controlled entry at
  `parsercore_phase0_driver.go:4918`. The materiality comparator at
  `parsercore_phase0_driver.go:5050` proves equal public trees only after the
  profile permits the election. It does not remove the grant dependency.
- Strategy-2 recovery remains profile-gated at
  `parsercore_phase0_driver.go:4287`. Its generic proof must record the
  scanner and recovery transition facts before B15 can admit it.

The 18 sites therefore remain shadow-classified as exception sites. Stage 2
may evacuate a site only after its generic proof receipt passes and the site
continues to pass exact locked-C parity.

This refresh changes documentation only. It confirms that the 18 grant paths
and their proof gaps still match the current source. No grant receives credit.

The durable signed receipt is
`hypha-receipt:2026-08-13:b15-stage1-current-v1`.

## Current-head shadow recheck

The current-head Docker recheck used gotreesitter commit
`a19776dc625adce558520f66073d50a2ecaea947` and the focused runtime-profile
shadow suite. It passed the exact-blob attachment checks for the compact
acceptance, converged split, runtime-profile, and external-scanner surfaces.
It also passed the negative exact-blob checks for adapted or wrong-identity
languages.

The run did not change a grant or parser route. It confirms the inventory's
shadow interpretation: the 18 sites remain exception sites until generic
derivation, leaf-coverage, scanner, and recovery proofs exist.

Run artifact:
`harness_out/docker/20260813T182734Z-b15-current-profile-shadow-v1/`.

- Container log SHA-256: `1dba96c1681faeff2205ad953d0cdb198228ca3343c0be34138f2261a36f9f64`.
- Metadata SHA-256: `a5814fdc06ddd630c73444a543eb6643a71d5c034d5a09ed1239f001e56242db`.
- Inspection SHA-256: `82cb2054ec00b3e2cc53e7c7c316d1ea2760aa0aca344aaf0e2abd9e1a2ea30c`.

This recheck grants no B15 stage-2 credit and does not close B15.
