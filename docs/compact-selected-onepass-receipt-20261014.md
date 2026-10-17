# One-pass selected-store qualification

The existing diagnostic one-pass occurrence folder replaces the staged builder
for ordinary selected-store construction. Twenty shuffled pairs show selected
Go parse/traversal is **2.08% faster at 20 KiB** and **3.06% faster at 235 KiB**.
Retained selected backing falls **32.85% / 33.68%**, with exact public digests,
node counts and committed work. Owned YAML selected projection is **0.83%
faster**, uses two fewer allocations, and retains **20.79% less** store backing.
This advances the output store; it does not close ordinary YAML ownership or
graduate compact, and it does not claim these gains for native `*Node` output.

## Mechanism and scope

Both existing builder facades call the same one-pass implementation. Its body
moves to an untagged internal file; the diagnostic facade remains available.
The ordinary facade selects the implementation already tested behind
`gts_parsercorephase0_selectedstore_onepass`. The staged implementation stays
available for direct differential checks. The folder walks compact occurrences
in postorder and emits logical records, avoiding the staged raw-occurrence and
child-window pass. It retains alias/field/mask occurrence ownership, root
framing, error and precedence propagation, cancellation, byte/occurrence caps,
pool release order and fail-closed unsupported-field/reused-subtree behavior.
The existing one-pass cap/cancellation regressions now run in ordinary core
builds; their bodies are unchanged.
No language-name comparison, environment read or routing switch is added.

Baseline: the pinned root-framing overlay over
`78f24bcf06e71f758d0e7bf605a1669c50510a2d`. The only additional runtime changes
are the two internal facades and the new shared implementation. Both measured
roles include exactly the same supertype/root corrections and test-only YAML
benchmark. Earlier independent reductions and root-framing timing are not
added to this result.

## Counter and correctness gates before timing

The focused exactness/cap/lifecycle suite, four canonical Go selected/native
comparisons and queries, owned YAML R4/padding/8 KiB root comparisons, and all
412 work/reuse rows pass before timing. Every reported benchmark work and
completion metric is independently compared and remains exact. Full subsequent
qualification passes all core tests, both builder tags, core/root races,
**206 invariant sessions / 618 C fixtures**, and owned YAML C document/padding,
version-ownership and recover-EOF checks. No ledger reset or exception is used.

Selected nodes remain **7,524 / 71,768** for the two Go sources and **231 / 925**
for YAML R4 / 8 KiB. Canonical digests remain pinned. Go work remains
**5,546 / 7,415 / 14,662** and **53,896 / 75,769 / 148,367** leaf constructions /
parent constructions / link additions. Records remain **36 bytes** and public
Node remains **104 bytes**. The final checkout also runs L0/R6 and focused
publication/memory checks; its terminal receipt is recorded below.

## Twenty paired samples

Use the repository randomized runner, alternating seed order, twenty separate
processes per role, `GOMAXPROCS=1`, `-count=1`, `750ms` and `-benchmem`. Shared
host lock inodes are held inside 6 GiB / one-CPU Docker isolation. Exact source
pins, complete footers, counter/completion reports and successful non-OOM,
non-timeout exits are retained.

| Selected projection / parse + traversal | Before | After | Change | p |
| --- | ---: | ---: | ---: | ---: |
| Owned YAML R4 time | 364.6 µs | 361.6 µs | −0.83% | .043 |
| 20 KiB Go time | 9.390 ms | 9.195 ms | −2.08% | <.001 |
| 235 KiB Go time | 100.28 ms | 97.21 ms | −3.06% | .001 |
| YAML allocated bytes | 12,948 | 12,824 | −0.96% | <.001 |
| 20 KiB Go allocated bytes | 142,303 | 140,896 | −0.99% | <.001 |
| 235 KiB Go allocated bytes | 3,301,325 | 3,289,805 | −0.35% | <.001 |
| YAML allocations | 103 | 101 | −1.94% | <.001 |
| 20 KiB Go allocations | 209 | 202 | −3.35% | <.001 |
| 235 KiB Go allocations | 2,987 | 2,974 | −0.44% | <.001 |
| 20 KiB Go retained backing | 448,156 B | 300,956 B | −32.85% | <.001 |
| 235 KiB Go retained backing | 4,328,596 B | 2,870,716 B | −33.68% | <.001 |

Exact YAML backing also falls **11,660→9,236 B** on R4 and **46,876→36,996 B**
on 8 KiB (20.79% / 21.08%). These are retained store capacities, not RSS or
whole-runtime retention measurements. Smaller raw scratch is not quantified as
a separate memory win. Strict tree/root/field/mask equality stays mandatory.

## Final safety and limits

All final safety and checkout runs exit zero without OOM or timeout. The
ordinary checkout passes **L0 756/756 root files / 12 tag sets**, **R6 454/101**,
104-byte Node layout, focused memory checks, and the promoted ordinary
cap/cancellation tests. The three measured runtime files match the checkout
byte-for-byte; test promotion removes only its former build constraint.
All 3,477 baseline Go/module inputs remain verified.
CI also caught the moved facade's stale lifecycle read-site line (16→5). The
registry pointer is corrected without changing proof policy; the full lifecycle
suite passes with strict historical proof enabled in a non-shallow Git checkout.
An initial container ownership/preflight failure is excluded from that proof. Earlier source/pair pins
and final counter/completion comparisons remain intact.

| Native safety control | Before | After | Direction | p |
| --- | ---: | ---: | ---: | ---: |
| Go full DFA | 4.361 ms | 4.359 ms | −0.03% | .678 |
| Go single-byte edit DFA | 107.6 µs | 107.7 µs | +0.11% | .718 |
| Go no-edit DFA | 3.387 ns | 3.379 ns | −0.22% | .560 |
| Compact Go R4 | 906.7 µs | 911.6 µs | +0.54% | .242 |
| Legacy Go R4 | 705.4 µs | 706.8 µs | +0.21% | .862 |
| Owned YAML native publication | 416.6 µs | 418.2 µs | +0.38% | .429 |

All six controls are inconclusive; no native-parser speed gain is claimed.
Their byte/allocation medians remain **1,161/6**, **394/5**, **0/0**, **1,696/25**,
**2,712/11** and **12,936/111**, respectively. All timing/allocation medians
stay within the 10% ratchet. Directional increases remain recorded. The
significant full-DFA control increase in the separate root-framing comparison
is not erased or attributed to this one-pass experiment.

No graduation or native-parser gain is inferred from this selected-store
improvement. No included-range, full native parse-state parity, all-grammar
selected-consumer equality or 1 MiB RSS claim is made. Ordinary YAML continues
its existing decline; the compact parser default and allowlist remain unchanged.
The builder promotion does not flip the compact parser default governed by D7.

Evidence: `/home/draco/.local/state/gotreesitter-evidence/20261014-selected-onepass-economics`
(measured mechanism) and `/home/draco/.local/state/gotreesitter-evidence/20261014-selected-onepass-qualification`
(full qualification, physical checkout and native controls). Baseline framing
sources remain frozen in the separate root-framing evidence directory.
