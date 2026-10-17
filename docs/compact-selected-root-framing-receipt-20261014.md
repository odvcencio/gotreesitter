# Selected-store result-root framing qualification

Selected projection now matches native publication's root metadata for the
owned YAML R4 sample and its 8 KiB EOF witness, including leading-padding
variants. This removes a correctness prerequisite for direct publication
projection. It enables no parser route or graduation. The independent
selected-supertype fix remains in the same output-lane PR, #1484.

## Before and after

The baseline's admitted R4 selected `stream` preserves reduced production ID
**1**, while the native synthetic `stream` publishes **0**. When extra roots
surround an unfielded grammar root, the shared selected-store framing now
clears its reduction production, own dynamic precedence and collapsed root
supertype ancestry. Child precedence, error flags and ancestry remain intact.
Fielded roots and single roots keep their reduction provenance. Extra children
extend root bounds; retained leading children can own padding. Otherwise the
root starts at the first source byte after UTF-8 BOM / ASCII padding, matching
native framing. A source gap whose skipped-prefix ownership is not represented
fails closed instead of guessing.

The regression records the original R4 production mismatch. The first range
unit test caught a missing extra-bound extension, which was fixed without
relaxing the assertion. Inspection then found the forced-zero leading start;
new space/newline witnesses and locked-C checks cover the corrected rule.
A superseded partial timing launch and a setup-path failure are excluded.

## Final correctness and deterministic evidence

Baseline: `78f24bcf06e71f758d0e7bf605a1669c50510a2d` (the pending supertype PR).
Nine pinned source files contain the root changes and regression coverage.
Both staged and one-pass builders pass fielded/single/synthetic provenance,
retained-prefix ownership, unrepresented-prefix decline/lifecycle, masks,
fields, alias, cancellation and retained-cap tests. Strict selected/native
metadata comparisons pass all four canonical Go fixtures and these YAML cases:

| YAML source | Bytes | Selected nodes |
| --- | ---: | ---: |
| Pinned R4 | 1,998 | 231 |
| Two leading spaces + R4 | 2,000 | 231 |
| Leading newline and spaces + R4 | 2,001 | 231 |
| Four-document EOF witness | 8,012 | 925 |

The added C tests require actual owned compact acceptance and exact structural
agreement, including points/fields/flags, on both padding variants. Go
supertype query and occurrence-mask witnesses remain green. Core/root races,
full core tests, **206 invariant sessions**, **618 C fixture checks**, the
owned-YAML document/recover-EOF checks and all **412 ordinary work/reuse rows**
pass. No work or reuse baseline is reset and no counter exception is needed.
The invariant sessions retain zero-allocation no-edit reparses.

The selected Go work, source identities, node counts and canonical digests stay
pinned. The staged retained stores remain **448,156 / 4,328,596 bytes** for
20 KiB / 235 KiB Go and **11,660 / 46,876 bytes** for R4 / 8 KiB YAML. Records
remain **36 bytes**. The diagnostic one-pass YAML backing is independently
**9,236 / 36,996 bytes**; this does not establish a timing win or graduate that
builder.

## Timing and final checkout

All four final timing families complete twenty alternating shuffled paired
processes at `GOMAXPROCS=1`, `750ms`, `-count=1` and `-benchmem`, under the two
shared host locks inside 6 GiB / one-CPU Docker isolation. They exit zero
without OOM or timeout. All 3,477 baseline inputs and nine measured candidate
files are verified after timing; runtime hashes match the physical checkout.
The unchanged YAML test/benchmark bodies were moved into the existing
selected-store test file after L0 rejected a 757th root file. The final
physical checks pass **756/756 files / 12 tags**, R6 **454/101**, focused
publication/memory tests and the unchanged **104-byte Node** layout. This
placement correction changes no runtime code or test/benchmark body.

| Benchmark | Before | After | Change | p |
| --- | ---: | ---: | ---: | ---: |
| Owned YAML selected projection | 361.9 µs | 361.8 µs | −0.01% | .758 |
| Selected 20 KiB Go | 9.409 ms | 9.423 ms | +0.14% | .989 |
| Selected 235 KiB Go | 100.47 ms | 99.70 ms | −0.76% | .009 |
| Compact Go R4 | 911.7 µs | 908.8 µs | −0.31% | .253 |
| Owned YAML native publication | 417.6 µs | 417.4 µs | −0.06% | .738 |
| Legacy Go R4 control | 691.0 µs | 711.1 µs | +2.92% | .004 |
| Go full DFA control | 4.188 ms | 4.376 ms | +4.49% | <.001 |
| Go one-byte edit DFA control | 107.9 µs | 107.4 µs | −0.39% | .620 |
| Go no-edit DFA control | 3.364 ns | 3.412 ns | +1.41% | .001 |

The small supported selected-Go decrease does not close the throughput gap.
The control regressions remain recorded, including the significant 4.49%
full-DFA increase; all timing medians remain within the 10% ratchet. This
bucket is justified by correctness and makes no overall parser speed claim.
No edit still uses **0 B/op / 0 allocations**. Selected YAML allocations are
**103→103** and byte medians **12,946→12,945.5**; selected Go allocations are
**209/2,987→209/2,987**, with byte medians **142,303/3,301,325** unchanged.
Compact Go remains **1,696 B/op / 25 allocations**, native owned YAML
**12,936 / 111**, and legacy Go **2,712 / 11**. Full/edit control byte medians
are **1,160→1,161 / 394→394**, with **6/5** allocations unchanged. Fractional
byte medians and tiny variations are not claimed as memory wins.

Evidence: `/home/draco/.local/state/gotreesitter-evidence/20261014-selected-root-framing`.
Earlier failed and superseded attempts remain in their separately named
external directories. This receipt makes no all-grammar selected-projection,
parse-state-parity, included-range, ordinary-YAML-admission, 1 MiB RSS or
compact-graduation claim. Native full-parse invariants and C fixtures do not
by themselves certify every selected-store consumer.
