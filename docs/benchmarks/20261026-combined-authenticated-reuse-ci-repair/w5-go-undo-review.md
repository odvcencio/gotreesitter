# W5 Go undo reuse counter: owner review pending

**The strict gate failed and remains failed. No exception has been approved or applied.** The affected row is Go / `one_byte` / undo step 1 / `reused_subtrees`: **39 → 35 (-10.26%)**, exceeding the 2% threshold. This request is limited to this fixture, operation, step, counter and revisions; it does not propose changing the global gate, baseline or counter.

The CI comparison used base `e079b642fa1c11a3c9bd939d367e05598fc8162f` and production head `462341df680f55944e1a747e4ea36014f063ae99`. The real Go fixture is `src/debug/elf/elf.go` at upstream `d00c67f297ef6f2cb2cd0e9aae59fa3936bb7eca`, 140,026 bytes, SHA-256 `ff2f811d29a56303bad96c3c7cff1a83814a974503a51cee918a314a96eb476e`. The operation changes byte 2959 from E to Z and then back. The [original CI artifact](https://github.com/odvcencio/gotreesitter/actions/runs/40442537049/artifacts/12706678126) has SHA-256 `98020ed133aabab1369edb4df4f69719f2adbcfbfef4a2a09e1c08ec59ce914f` and is also retained in [the evidence archive](w5-go-undo-evidence.tar.gz).

| Step | Counter | Baseline | Head |
| --- | --- | ---: | ---: |
| 0 | nodes | 108999 | 98946 |
| 0 | max stacks | 18 | 9 |
| 1 | tokens | 22393 | 22385 |
| 1 | nodes | 109018 | 98946 |
| 1 | max stacks | 18 | 9 |
| 1 | reused subtrees | 39 | 35 |
| 1 | reused bytes | 2816 | 2853 |

The scratch trace reproduces the original counters. On undo, 34 selections match. Baseline selects five leaves at `[3168,3169)`, `[3171,3178)`, `[3192,3193)`, `[3195,3205)` and `[3216,3217)`, totaling 20 bytes; head selects their enclosing five-child subtree `[3162,3219)`, totaling 57 bytes. This accounts for four fewer selections and 37 additional reused bytes. Both arms count successful reuse operations, so this explanation does not make the failed row pass.

The original C 0.25.1 harness passed public fresh/incremental tree comparisons for Go on plain and profiled paths. A separate scratch replay checked both steps and APIs against fresh Go for structure plus every node's parse state, pre-goto state and dynamic precedence; it passed. Baseline invoked the verifier and retained its incremental tree; head used authenticated bypass. The scratch replay does not replace the C oracle or broader gates.

Owner review and a separately reviewed mechanism would be required for any exception; the W5 script currently has no exception input. No threshold or result is changed here. Validate the retained artifact and diagnostics with `python3 docs/benchmarks/20261026-combined-authenticated-reuse-ci-repair/validate-w5-review.py`.
