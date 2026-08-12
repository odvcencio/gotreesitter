# B15 stage 1 grant inventory and shadow receipt

Status: implemented

This receipt records the B15 stage 1 inventory at gotreesitter commit
`65c9472806bdaa9f98d7eff0e19c0b2d53ef84d5`.

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
