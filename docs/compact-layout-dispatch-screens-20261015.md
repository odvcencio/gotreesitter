# Compact layout and dispatch screens

Evidence date: 2026-10-15 UTC. Each independent comparison starts at
`c498e889ec2d86382a3a0baad58020d7df7145b3`. All eight prototypes remain outside
the checkout. None improves compact throughput enough to promote. All merges
to main remain held.

| Independent experiment | Go query time | Large Go time | Owned YAML R4 time | Owned YAML 8 KiB stream time | Decision |
| --- | ---: | ---: | ---: | ---: | --- |
| Pack two lineage booleans into flags; retain dense records | −0.20%, p=.242 | −0.84%, p=.121 | −0.12%, p=.398 | +0.67%, p=.040 | Reject: no demonstrated compact speed gain |
| Reuse an exactly equal immutable lexer-state wrapper | Not claimed | Not claimed | +0.84%, p=.030 | +0.81%, p=.011 | Reject: actual allocation counts do not fall |
| Select the first pure reduction through compiled cohort rows | +1.67%, p<.001 | +1.67%, p<.001 | +0.92%, p=.063 | +0.37%, p=.547 | Reject: both canonical Go files slow down |
| Inspect built-in rows directly; authenticate the selected reduction normally | −0.16%, p=.583 | −0.05%, p=.620 | −0.32%, p=.425 | −0.29%, p=.301 | Reject: no established throughput gain |
| Check existing policy rejection guards before token projection | −0.22%, p=.461 | −0.99%, p=.076 | +0.24%, p=.547 | −0.05%, p=.640 | Reject: no established compact throughput gain |
| Extract the unchanged existing-boundary condensation body | +0.17%, p=.583 | +0.70%, p=.043 | +0.26%, p=.698 | +0.36%, p=.758 | Reject: large Go slows down |
| Reuse the classified cell only for one unchanged owned header | Not claimed | Not claimed | −0.43%, p=.183 | −0.24%, p=.314 | Reject: no supported gain on the affected YAML path |
| Hint hash-table slots by state, then verify the current complete key | +0.09%, p=.779 | +0.25%, p=.640 | −0.58%, p=.478 | −0.09%, p=.383 | Reject: no supported compact throughput gain |

Negative time means less parse time. Inconclusive directions are retained in
the table, not presented as wins. These percentages have independent paired
baselines and must not be added to earlier improvements.

## What the screens establish

Packing flags reduces each dense lineage record from 32 to 28 bytes without
an interner or loss of metadata. The retained record width falls 12.5%, but
the measured compact benchmarks do not establish a throughput improvement.
All compact allocation-count and byte medians stay equal. Default full and
no-edit controls move −3.57% and −1.06% (both p<.001); the edit control moves
−0.25% (p=.433). Those controls bypass this compact representation, so their
directions are not attributed to the packing change.

Exact wrapper reuse preserves every comparable field and copies on any change.
A focused regression fails on the baseline and proves zero allocations for an
equal publication in the prototype; changed values preserve old aliases and
retained acceptance state. The public YAML allocation medians nevertheless
remain 109 and 336 objects, with the same byte medians. The existing equivalent
state installation already handles the common public path. After completing
all 20 affected YAML pairs, the remaining Go timing was deliberately stopped;
partial Go samples establish nothing.

The compiled cohort reads only ordinary unary shift/reduce rows, selects the
same first reduction as generic dispatch, and rejects recovery, contextual
relex, owned scanning, conflicts and other mixed policies. It authenticates the
selected boundary and uses the existing corridor reducer. The extra preflight
does not pay for itself: query time rises 9.66190→9.82312 ms and large Go rises
102.511→104.226 ms. Go R4 moves +0.45% (p=.142). Object counts stay equal;
query bytes round 138,738→138,739, while large Go stays at 3,251,728 bytes.
The default trio moves +0.11%, +0.04%, −0.48%, all inconclusive; no-edit remains
zero bytes and objects.

The direct-row follow-up avoids compiled sparse dispatch and does not use the
corridor-trusted reducer or caller-supplied classified rows. A current scheduler
owner can borrow the table provider, and only the concrete built-in adapter is
eligible for speculative inspection; arbitrary callbacks stay on the original
path. Ordinary `ClassifyBoundary` authenticates the selected row. This removes
the earlier significant regression but establishes no gain on either canonical
Go fixture. Go R4 moves −0.48% (p=.301); default full/edit/no-edit move
+0.08% / +0.28% / −0.71% (p=.799/.512/.060). Object counts remain equal. The
full-DFA byte median rounds 1,159→1,160; compact byte medians remain equal.

The policy follow-up moves the callee's existing nil-language, empty-policy and
unary-row rejection guards before token construction in shared and owned
dispatch. It adds no cache, capability or state layout. All 414 rows remain exact,
but the completed screen establishes no compact speed gain. Go R4 moves −0.16%
(p=.698). Default full/edit/no-edit controls move +0.12% / +0.96% / −1.79%
(p=.529/.001/<.001). The controls bypass this policy path; their directions are
recorded without attributing them to the guard. Object medians stay equal;
full-DFA bytes round 1,159→1,159.5, and all compact byte medians stay equal.
The source-only Buckley review finds no concrete defect in the supplied diff
and cannot independently verify the calibration receipts or qualification.
Its lack of findings does not override the performance result.

The condensation screen moves 326 lines of existing-boundary fold handling
into an internal helper. It preserves the frontier, predecessor, payload,
stored-cost and independent-owner checks before the same direct-append path;
the extracted body is byte-for-byte equal. This tests the large condensation
frame and instruction layout without changing a cache, proof or algorithm.
The test-only baseline head is `7b016214d`; production source still equals
`c498e889e`. All 414 rows remain exact. Twenty pairs regress large Go 0.70%
(p=.043), while query Go, Go R4 (+0.09%, p=.738) and both YAML times are
inconclusive. All byte and object medians remain equal. Default full/edit/no-edit
controls move +0.003% / +0.40% / −1.69% (p=.383/.007/<.001); they bypass compact
condensation and receive no attribution. No-edit stays at zero bytes and objects.
The split is rejected and receives no broad qualification.

The owned single-header screen reuses the first classified cell only while one
header and one candidate remain, with unchanged head and request references.
Authentication before apply stays intact; multiple headers keep their second
classification. Full focused correctness and all 414 rows pass. Twenty YAML
pairs establish no gain: R4 353.451→351.939 µs (p=.183), stream
1,380.119→1,376.745 µs (p=.314). Bytes remain 12,872 / 44,313 and objects
109 / 336. The experiment is rejected after completing YAML; unfinished Go
samples are deliberately stopped and excluded. The owned container exits 143
because of that stop, not a crash or timeout. Buckley's supplied-source review
finds no concrete defect but cannot inspect missing helper context; it does not
validate execution evidence. No broader qualification follows.

Blindly extending reuse to earlier cells on a multi-header frontier is unsafe:
binding another request can advance the classification phase. Head/token
equality does not reauthenticate the old capability. Existing fresh
classification remains across that boundary; no broader reuse is promoted.

The slot-hint screen retains the complete boundary hash table. Sixty-four hints
store positions only; a hit checks the current slot's bounds, generation and
complete state/byte/checkpoint/flag key before reading its canonical node ID.
The final buffer is lazy, has explicit capacity accounting, restores its slice
header on rollback, and is dropped and disabled in recovery clones. No proof,
node identity or scanner equality is cached. Found publications do not grow;
cold re-probes recompute the complete key hash.

The initial inline attempt omits the new fixed storage from the footprint
gauge; its partial timing is excluded. Charging it unconditionally then fails
the idle/reservation contracts, so that attempt is rejected before timing.
A slice-declaration build error is separately archived. The final lazy source
passes the unchanged reservation/cap tests, focused core/root/C/edit checks,
all 414 rows and six one-process memory-budget trials. Peaks are
1,073,116 / 13,316 bytes, each 256 bytes above production and within the existing
bounds. The final source-only Buckley review establishes no defect, while
leaving broader snapshot-held backing accounting outside its supplied scope.

Twenty final-source pairs still establish no compact speed gain. Go R4 moves
+0.36% (p=.157); query/large Go and both YAML directions are in the table.
All affected byte and object medians remain equal. Default full/edit/no-edit
controls move −3.67% / −0.44% / −1.99% (p<.001/.035/<.001), bypassing the
changed index; their improvements are not attributed to the hints. Full-DFA
bytes round 1,159.5→1,159, while no-edit stays at zero bytes and objects. The
runtime is rejected. No full fleet, race or RSS certificate follows.

## Verification and limits

Every experiment passes full core and phase-zero-A tests, focused Go/YAML root
checks, Go/YAML/CMake/HTML/Python edit invariants, the diagnostic-owned YAML
invariant, and focused locked-C comparisons before timing. All **414 ledger
rows** are exactly equal: 412 ordinary and two diagnostic-owned YAML rows.
Default and opt-in C4 checks pass. The layout screen rechecks the 206 static
table shapes; the cohort screen reuses the unchanged compiler's pinned static
receipt rather than claiming a fresh fleet shape run.

Comparisons use the repository randomized runner, 20 alternating shuffled pairs,
one process per seed, `GOMAXPROCS=1`, `-count=1`, 750 ms and `-benchmem`, serialized
under both locks in 6 GiB Docker on CPU 6. Source pins and timed route/node
metrics pass the completed layout and cohort audits. Wrapper reuse has 20
completed YAML pairs and a separate pinned rejection receipt, not a completed
Go audit. Local evidence directories are `20261015-packed-lineage-flags`,
`20261015-version-state-reuse`, and `20261015-pure-reduction-cohort` under
`gotreesitter-evidence`; the direct follow-up is `20261015-direct-reduction-cohort`.
The policy follow-up is `20261015-conflict-policy-preflight`; both roles share
the identical test-only diagnostic receipt refresh. The runtime guard is not
promoted, and the test repair receives no performance claim.
The condensation screen is `20261015-condense-hot-frame`; its complete source,
sample, work/route and unchanged-body audits pass.
The owned single-header screen is `20261015-owned-single-classification`, with
separate completed-YAML and partial-Go provenance.
The slot-hint screen is `20261015-boundary-slot-hints`, with all earlier attempts
excluded from the final source, sample and work/route audit.

The cohort run was interrupted for disk pressure after completing YAML. Its
partial Go samples were discarded and all 20 Go pairs restarted from seed 1
after serialized cleanup of old rebuildable Docker cache. Completed source,
fixtures, raw evidence and CPU profiles were preserved. This interruption is
not a parser failure or an out-of-memory result.

No full 206-session invariant fleet, 618-fixture C fleet, strict census, race or
large-file RSS qualification transfers to these rejected prototypes. The six
focused budget trials belong only to the final slot-hint source above. Runtime,
admission allowlists and global defaults remain unchanged.

## Censuses that rule out broad caches

A separate read-only census compares complete parent records and ordered child,
field and alias arrays with the existing reduction-identity predicate. It finds
only 749 exact duplicate parents among 129,665 large-Go compact subtrees
(0.58%; 0.99% of parents), and none among the 187 / 751 parents in the ordinary
YAML R4 / four-document prototype. Go query has 21 duplicates among 12,961
subtrees. This gives no ownership, replay or reuse certificate and does not
justify a global subtree interner. All four Go deep digests and both YAML
public differentials pass, with unchanged observed work and frozen source.

A boundary-index census excludes internal rehash lookups. Large Go makes
287,787 logical probes; only 31,715 (11.02%) hit the first published key of a
frontier. YAML makes 337 / 1,341 probes, with 38 / 149 first-key hits (11.28% /
11.11%). The 50,609 empty Go probes equal its first publications, so making
only empty lookups lazy would still need the insertion hash. These counts do
not establish a worthwhile first-entry cache. The initial observer incorrectly
asserted table completeness during partial rehash and is archived as invalid;
the corrected run passes all named digest/work checks and source pins.

These diagnostics live in `20261015-exact-subtree-duplicate-census` and
`20261015-boundary-first-entry-census`. Neither changes the runtime or supplies
a timing, C-fleet or graduation certificate.

A new state/frontier census compares separately executed observed and
unobserved parses on all four canonical Go files. Both work vectors and pinned
deep digests remain exact. Each successful probe reaches that state's first
published key in its frontier; no second byte/checkpoint/flag key for the same
state appears in these witnesses. Large Go has 129,327 successful probes of
287,787 (44.94%), with 479 distinct states across the parse and at most 14
published primary states in one frontier. The 158,460 missing-key probes are
different from the earlier census's probes against an entirely empty table.

This supports testing state-indexed slot hints; it does not certify an index
that omits full-key or generation checks. The diagnostic is pinned in
`20261015-state-frontier-index-census`, with zero lost model keys, source audit
and terminal exit zero. No throughput or graduation claim follows from counts.
