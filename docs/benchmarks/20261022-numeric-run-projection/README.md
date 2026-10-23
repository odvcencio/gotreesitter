# Numeric-run projection: large edit win, unpromoted

A private prototype reuses the complete edited tree when a decimal digit-run edit preserves every relevant lexical decision. On a generated 137 KiB Go variable-declaration witness, twenty paired native ARM64 runs reduce edit time by about 98% on both configured routes. It remains unpromoted: the primary single-byte benchmark regresses 9.67%, two targeted block-splice counters fall, and PR #1509's absolute RSS hold remains. No route, allowlist, ledger baseline or shipped runtime changes.

## Mechanism and scope

The new Go scanner certificate covers decimal digit runs only. It does not certify identifiers, punctuation, line changes or arbitrary external-scanner state. The existing bounded DFA/keyword proof still authenticates token identity and scan frontiers; the source-sensitive Go proof still requires a clean numeric leaf. Unknown or exhausted evidence declines to the established reparse and verifier.

Compact's ordinary runtime does not publish the single-stack statistic used by this existing width-changing fast path. A completed dependency-producer trace supplies a separate private certificate: active matching read history, no invalidated multi-header/blended/recovery frontier, one final exact path, complete subtree receipt coverage and a frontier beyond EOF. A final sole head alone cannot certify the parse. Small or unsupported requests that lack this evidence continue to reparse. The flag occupies existing Tree padding; measured Node and Tree sizes remain 104 and 208 bytes on native amd64/arm64. It is cleared when source coverage is replaced or a Tree is released.

The seven-file [prototype patch](prototype.patch) applies to runtime `4de229c4b1718989917c6212f92cf74df28cf229` (docs-only head `9aa6778`). It is an experiment in the evidence archive, not part of this PR's runtime. Source hashes in [qualification metadata](qualification.json) bind the actual C and benchmark candidates. Both benchmark roots share the exact same measurement wrapper.

The 140,292-byte witness has SHA-256 `469b85fa63a73a83eaa4e16406e2823132e0a3652a95d7aa9a966a0c3db5729d`. Each session inserts and removes one interior digit near the start of a generated file of variable declarations. This is distinct from Cognociente's 140,368-byte function fixture and does not establish a general Go edit speedup or compact graduation.

## Correctness before timing

The final candidate passes 2,836 exact locked-C comparisons, including 28 numeric sessions of 33 consecutive edits each (924 edit steps) across both routes and both incremental APIs. These sessions include integer, float, exponent, separated-digit, comment-boundary, function-body and large-file cases. Independent fresh Go and both fresh/incremental C trees must agree on public symbols, fields, spans, points, flags and deep digests. Existing numeric repair/reuse C regressions remain green.

Four Go/YAML R4 sessions pass all 288 steps with zero no-edit allocations. Go/YAML deterministic ledger exports are byte-identical. Twenty memory contracts, provenance reset tests, reservation/density tests and scanner-mask certificate checks pass. The Mac repeats the native gates before measurement and passes four focused root race cases. One earlier harness run stops before timing because it inherited CGO_ENABLED=0 for its race command; the retained failed audit is separate from the successful rerun, which enables CGO for the race command only.

The exact latest [Cognociente report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850) is replayed separately against this candidate: eight clean insert/replace C cases pass, all twelve incremental/fresh Go cases agree, and all twelve incremental/fresh C cases agree. Four malformed-delete cross-engine comparisons still fail at an ERROR with one Go child versus two C children. They remain failures. The Windows ARM64/x64 matrix is not rerun.

## Paired native measurements

M1 Max, native Linux arm64, Go 1.25.14, GOMAXPROCS=1, one process per explicit shuffle seed, count=1, benchtime=750ms and benchmem. The repository randomized runner alternates baseline/candidate order. Numeric seeds are 158001–158020; primary seeds are 159001–159020. All 140 benchmark rows per role are complete; source manifests match before/after and the pinned sources, and the container reports no OOM. [Full numeric statistics](benchstat-numeric.txt) and [primary statistics](benchstat-primary.txt) retain all metrics.

| Numeric session | Before | Prototype | Change |
| --- | ---: | ---: | ---: |
| Default, plain | 54.534 ms | 1.134 ms | −97.92% |
| Default, profiled | 63.404 ms | 1.219 ms | −98.08% |
| Compact, plain | 107.552 ms | 1.985 ms | −98.15% |
| Compact, profiled | 108.171 ms | 1.928 ms | −98.22% |

All four timing differences have p<0.001, n=20. Plain allocated bytes fall from about 4.006 MB to 480 bytes on default and from 13.08 MiB to 32.21 KiB on compact. Plain allocations fall 47→8 and 163→52 respectively. These are the complete measured calls, including Tree.Edit, reuse/reparse, result checks and old-tree release.

Phase attribution identifies Tree.Edit as the next large cost: profiled medians are 1.080 ms on default and 1.012 ms on compact, with reuse at 131.7 and 807.1 µs. Selected reparse time becomes zero on default and about 1.8 µs on compact. Selected profile fields are not total hidden-verifier work; complete wall time above measures the operation.

| Primary regression check | Before | Prototype | Result |
| --- | ---: | ---: | --- |
| Full parse | 5.864 ms | 5.828 ms | Inconclusive, p=0.183 |
| Single-byte edit | 130.2 µs | 142.8 µs | +9.67%, p<0.001 |
| No edit | 6.123 ns | 6.104 ns | Inconclusive, p=0.298 |

Primary allocation counts remain 6/5/0 and no-edit allocated bytes remain zero. Restoring the original same-width digit check does not remove the measured single-byte regression. Keep it as an adverse result; profile it before promotion.

## Counter trade and holds

Existing Go/YAML ledger rows are unchanged, but the new targeted session has two raw reuse-counter decreases. Whole-root sharing replaces partial top-level splices. Do not inflate that physical operation count to imitate the old path.

| Selected profiled work per operation | Default before→after | Compact before→after |
| --- | ---: | ---: |
| New nodes | 23,572→0 | 23,572→0 |
| Tokens | 7,859→0 | 7,864.5→43 |
| Reused bytes | 132,425→140,292.5 | 132,425→140,292.5 |
| Block splices | 7,857→0 | 7,857→0 |

The counter trade is unapproved under the [v1 D10/R3 direction and owner rule](../../v1-design.md#r3-counter-ledger-changes). No exemption or ledger reset is applied. These two decreases, the primary edit regression, broader qualification and the existing absolute RSS failure keep the prototype unpromoted. Ordinary YAML ownership and compact graduation remain open.

The [raw archive](receipts.tar.gz) includes controls, frozen runtime sources, independent C logs, the retained report failures, native tests/race logs, complete randomized samples, before/after source manifests and the failed pre-timing harness receipt. [Metadata](qualification.json) authenticates every member. The first, smaller prototype's negative primary result is retained as well.
