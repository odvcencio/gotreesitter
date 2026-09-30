# O4 grammar receipt run

Receipts: 207 (target 207). These record current state; they do not graduate grammars.
Fresh locked-C parity: 130/207 pass; incremental locked-C parity: 36/207 pass; both: 31/207 pass.
Invariant gate: 187/207 pass.
Recorded timeouts: 0; unavailable samples: 20; generator errors: 0.
Go revisions: `cd785434b5608139d8e481455692f328b771a382`
C runtime revisions: `6070dbfefd326bd735e5683eb128cc1b57dad0c0`
C incremental/fresh disagreements: 165 steps; Go/C incremental disagreements: 8782 steps. Receipts with reference=fresh_c keep these axes diagnostic.

Compact route counts: accepted=191, declined=5, forest_route=11
Cohort counts: 1a=1, 1b=6, 2=15, 3=28, 4-A=69, 4-B=24, 4-C=51, 4-D=1, 4-E=4, 4-F=7, Lean 4=1

| Grammar | Cohort | Route today | Decline reason | Fresh C | Go incremental vs fresh C | Go invariants | C incremental vs fresh | Go vs C incremental | Unavailable reason |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| ada | 4-E | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| agda | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| angular | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| apex | 4-E | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 64/72 steps |  |
| arduino | 4-F | forest_route |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for arduino |
| asm | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| astro | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 31/72 steps |  |
| authzed | 4-D | declined | custom token source factory | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| awk | 3 | forest_route |  | pass | fail | pass | 72/72 steps | 6/72 steps |  |
| bash | 2 | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| bass | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 56/72 steps |  |
| beancount | 4-C | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for beancount |
| bibtex | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 37/72 steps |  |
| bicep | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 39/72 steps |  |
| bitbake | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 3/72 steps |  |
| blade | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | pass | pass | 72/72 steps | 72/72 steps |  |
| brightscript | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 47/72 steps |  |
| c | 2 | declined | custom token source factory | pass | fail | pass | 72/72 steps | 5/72 steps |  |
| c_sharp | 1b | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core phase zero: shared (15,237) live-link cap exceeded: 9 > 8 | fail | fail | pass | 72/72 steps | 6/72 steps |  |
| caddy | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | pass | pass | 72/72 steps | 72/72 steps |  |
| cairo | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| capnp | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for capnp |
| chatito | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| circom | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 13/72 steps |  |
| clojure | 3 | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| cmake | 3 | accepted |  | pass | fail | pass | 49/72 steps | 48/72 steps |  |
| cobol | 4-C | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for cobol |
| comment | 4-B | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| commonlisp | 4-A | accepted |  | fail | pass | pass | 72/72 steps | 72/72 steps |  |
| cooklang | 4-C | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for cooklang |
| corn | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 23/72 steps |  |
| cpon | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 1/72 steps |  |
| cpp | 2 | declined | custom token source factory | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| crystal | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| css | 2 | accepted |  | pass | fail | pass | 72/72 steps | 33/72 steps |  |
| csv | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| cuda | 4-C | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for cuda |
| cue | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| cylc | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection; compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 1/72 steps |  |
| d | 3 | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| dart | 2 | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | fail | fail | pass | 72/72 steps | 11/72 steps |  |
| desktop | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for desktop |
| devicetree | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 8/72 steps |  |
| dhall | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| diff | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| disassembly | 4-C | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for disassembly |
| djot | 4-C | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| dockerfile | 4-C | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for dockerfile |
| dot | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 8/72 steps |  |
| doxygen | 4-C | accepted | compact route declined at accept_without_materialization: accepted-leaf-tiling-gap: compact subtree symbol=65 span=145..291 has an unaccounted byte range 214..218 not covered by any child; compact route declined at unsupported_route: generic scheduler root reduction on no-lookahead was not followed by authenticated EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| dtd | 4-B | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for dtd |
| earthfile | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| ebnf | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| editorconfig | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| eds | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for eds |
| eex | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| elisp | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| elixir | 1b | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| elm | 3 | accepted |  | pass | fail | pass | 71/72 steps | 68/72 steps |  |
| elsa | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 32/72 steps |  |
| embedded_template | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| enforce | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core phase zero: shared (128,492) live-link cap exceeded: 9 > 8 | fail | fail | pass | 71/72 steps | 0/72 steps |  |
| erlang | 3 | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| facility | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for facility |
| faust | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| fennel | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 61/72 steps |  |
| fidl | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| firrtl | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| fish | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 4/72 steps |  |
| foam | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| forth | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| fortran | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 11/72 steps |  |
| fsharp | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection; compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | fail | fail | pass | 70/72 steps | 0/72 steps |  |
| gdscript | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| git_config | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 15/72 steps |  |
| git_rebase | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| gitattributes | 4-F | forest_route |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| gitcommit | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | pass | pass | 72/72 steps | 72/72 steps |  |
| gitignore | 4-F | forest_route |  | pass | fail | pass | 72/72 steps | 15/72 steps |  |
| gleam | 4-B | accepted | compact route declined at no_action: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection; compact route error: parser-core phase zero: shared (7,936) live-link cap exceeded: 9 > 8 | fail | fail | pass | 72/72 steps | 3/72 steps |  |
| glsl | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| gn | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 9/72 steps |  |
| go | 1a | accepted |  | pass | fail | pass | 72/72 steps | 8/72 steps |  |
| godot_resource | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| gomod | 3 | accepted |  | pass | fail | pass | 72/72 steps | 1/72 steps |  |
| graphql | 3 | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 12/72 steps |  |
| groovy | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| hack | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| hare | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 11/72 steps |  |
| haskell | 3 | accepted | compact route error: parser-core phase zero: shared (9192,1015) live-link cap exceeded: 9 > 8 | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| haxe | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| hcl | 3 | accepted |  | pass | fail | pass | 72/72 steps | 14/72 steps |  |
| heex | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 32/72 steps |  |
| hlsl | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| html | 1b | accepted |  | pass | fail | pass | 72/72 steps | 58/72 steps |  |
| http | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 15/72 steps |  |
| hurl | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 7/72 steps |  |
| hyprlang | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for hyprlang |
| ini | 3 | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| janet | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 34/72 steps |  |
| java | 2 | declined | custom token source factory | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| javascript | 2 | forest_route |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| jinja2 | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 15/72 steps |  |
| jq | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 1/72 steps |  |
| jsdoc | 4-E | accepted | compact route declined at unsupported_route: generic scheduler root reduction on no-lookahead was not followed by authenticated EOF | fail | fail | pass | 72/72 steps | 64/72 steps |  |
| json | 2 | declined | custom token source factory | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| json5 | 3 | forest_route |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for json5 |
| jsonnet | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 2/72 steps |  |
| julia | 3 | accepted |  | pass | fail | pass | 72/72 steps | 36/72 steps |  |
| just | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 44/72 steps |  |
| kconfig | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| kdl | 4-F | forest_route |  | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| kotlin | 2 | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| lean | Lean 4 | accepted |  | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| ledger | 4-A | accepted | compact route error: parser-core phase zero: accepted compact root is incomplete or erroneous: span=0..932 expected=1..932 error=false allowErrorRoot=false | pass | fail | pass | 72/72 steps | 3/72 steps |  |
| less | 4-B | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| linkerscript | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| liquid | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 36/72 steps |  |
| llvm | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 54/72 steps |  |
| lua | 3 | accepted |  | pass | fail | pass | 72/72 steps | 52/72 steps |  |
| luau | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 36/72 steps |  |
| make | 3 | accepted |  | pass | fail | pass | 72/72 steps | 15/72 steps |  |
| markdown | 1b | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| markdown_inline | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| matlab | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 2/72 steps |  |
| mermaid | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 39/72 steps |  |
| meson | 4-E | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core phase zero: shared (1159,77) live-link cap exceeded: 9 > 8; compact route error: parser-core phase zero: shared (247,109) live-link cap exceeded: 9 > 8; compact route error: parser-core phase zero: shared (343,79) live-link cap exceeded: 9 > 8 | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| mojo | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 10/72 steps |  |
| move | 4-B | accepted | compact route declined at no_action: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection | pass | fail | pass | 72/72 steps | 2/72 steps |  |
| nginx | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 2/72 steps |  |
| nickel | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 64/72 steps |  |
| nim | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 14/72 steps |  |
| ninja | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| nix | 3 | forest_route |  | pass | fail | pass | 72/72 steps | 10/72 steps |  |
| norg | 4-C | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for norg |
| nushell | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| objc | 3 | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | fail | pass | pass | 72/72 steps | 72/72 steps |  |
| ocaml | 3 | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| odin | 4-B | accepted | compact route declined at no_action: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| org | 4-C | accepted |  | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| pascal | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 3/72 steps |  |
| pem | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| perl | 3 | accepted |  | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| php | 1b | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 9/72 steps |  |
| pkl | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 8/72 steps |  |
| powershell | 3 | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| prisma | 4-F | forest_route |  | pass | fail | pass | 72/72 steps | 15/72 steps |  |
| prolog | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 0/72 steps | 0/72 steps |  |
| promql | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for promql |
| properties | 4-C | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| proto | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 34/72 steps |  |
| pug | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| puppet | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 2/72 steps |  |
| purescript | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core phase zero: shared (7602,1003) live-link cap exceeded: 9 > 8; compact route error: parser-core phase zero: shared (7602,967) live-link cap exceeded: 9 > 8; compact route error: parser-core phase zero: shared (8035,562) live-link cap exceeded: 9 > 8 | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| python | 1b | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| ql | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| r | 3 | accepted |  | pass | fail | pass | 72/72 steps | 13/72 steps |  |
| racket | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 34/72 steps |  |
| regex | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for regex |
| rego | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 64/72 steps |  |
| requirements | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| rescript | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| robot | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 64/72 steps |  |
| ron | 4-B | accepted |  | pass | fail | pass | 72/72 steps | 3/72 steps |  |
| rst | 4-C | accepted |  | pass | fail | pass | 8/72 steps | 7/72 steps |  |
| ruby | 2 | accepted |  | pass | fail | pass | 72/72 steps | 8/72 steps |  |
| rust | 2 | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| scala | 3 | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 8/72 steps |  |
| scheme | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 60/72 steps |  |
| scss | 3 | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 7/72 steps |  |
| smithy | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 29/72 steps |  |
| solidity | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| sparql | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 3/72 steps |  |
| sql | 2 | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 48/72 steps |  |
| squirrel | 4-F | forest_route |  | pass | fail | pass | 71/72 steps | 12/72 steps |  |
| ssh_config | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for ssh_config |
| starlark | 4-C | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| svelte | 3 | accepted |  | pass | fail | pass | 72/72 steps | 36/72 steps |  |
| swift | 2 | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| tablegen | 4-B | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| tcl | 4-B | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 5/72 steps |  |
| teal | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 13/72 steps |  |
| templ | 4-C | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| textproto | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| thrift | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| tlaplus | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 1/72 steps |  |
| tmux | 4-A | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for tmux |
| todotxt | 4-A | accepted |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| toml | 3 | accepted |  | pass | fail | pass | 72/72 steps | 48/72 steps |  |
| tsx | 2 | accepted |  | pass | fail | pass | 71/72 steps | 0/72 steps |  |
| turtle | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 29/72 steps |  |
| twig | 4-A | accepted |  | pass | fail | pass | 72/72 steps | 24/72 steps |  |
| typescript | 2 | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | fail | fail | pass | 72/72 steps | 4/72 steps |  |
| typst | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| uxntal | 4-F | forest_route |  | pass | pass | pass | 72/72 steps | 72/72 steps |  |
| v | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection; compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor; compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 33/72 steps |  |
| verilog | 4-A | accepted | compact route declined at no_action: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| vhdl | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 0/72 steps |  |
| vimdoc | 4-A | accepted |  | fail | fail | pass | 72/72 steps | 4/72 steps |  |
| vue | 4-C | accepted |  | pass | fail | pass | 72/72 steps | 60/72 steps |  |
| wat | 4-A | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 35/72 steps |  |
| wgsl | 4-B | accepted | compact route error: parser-core fresh-full runner did not accept EOF | pass | fail | pass | 72/72 steps | 2/72 steps |  |
| wolfram | 4-C | accepted | compact route error: parser-core fresh-full runner did not accept EOF | fail | fail | pass | 72/72 steps | 6/72 steps |  |
| xml | 3 | accepted |  | pass | fail | pass | 72/72 steps | 0/72 steps |  |
| yaml | 3 | accepted | compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor | pass | fail | pass | 72/72 steps | 7/72 steps |  |
| yuck | 4-B | accepted |  | unavailable | unavailable | unavailable | unavailable | unavailable | no corpus files selected for yuck |
| zig | 3 | accepted |  | pass | fail | pass | 72/72 steps | 1/72 steps |  |

Generator CPU: 513.980 s (0.1428 h).
Summed per-grammar runner time: 1189.010 s (0.3303 h; 207 timed attempts for 207 receipts).
Full local batch wall time: 1315 s.
