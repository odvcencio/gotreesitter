# B0 witness restoration receipt

Receipt ID: `b0-witness-restoration-v1`

Authority: Hyphae `spec.campaign.v7`, Revision R1, Phase 1, unit 1.4.

Audit time: `2026-08-11T22:30:10Z`.

Audit tree: `f13d192e7e768639821c93a40a3f83b477ed66de`.

## Verdict

B0 remains partially gated. The audit restores 20 of 98 witnesses.

The audit cannot adjudicate the remaining 78 as duplicate or corrupt.
No source record, mutation seed, mutation recipe, or sweep log exists for them.

Do not claim B0 closure.

## Denominator

| Set | Total | HTML | JavaScript | Swift |
| --- | ---: | ---: | ---: | ---: |
| R1 campaign target | 98 | 88 | 8 | 2 |
| Committed and reproducible | 20 | 10 | 8 | 2 |
| Unresolved gap | 78 | 78 | 0 | 0 |

The denominator is recorded in:

`cgo_harness/testdata/compact_t3_oracle_witnesses_v2.json`

Manifest SHA-256: `e7d51af1f3f3fea5d04437ed4ee84527df102eab05c09b5e569441f516c47b61`.

## Restorable witnesses

These records contain inline source bytes, source SHA-256 values, and locked
C-oracle provenance. The existing gate validates every record.

| Class | Count | Witness identifiers |
| --- | ---: | --- |
| HTML erroneous end tag | 10 | `html_min_a`, `html_min_html`, `html_log_1` through `html_log_8` |
| JavaScript arrow parameter recovery | 8 | `js_log_1` through `js_log_8` |
| Swift shift comparison | 2 | `swift_log_1`, `swift_log_2` |

The 20 identifiers are unique. Each record has a non-empty UTF-8 source and a
valid source digest. No duplicate or corrupt record exists in this set.

## Missing witnesses

The 78 missing records are the HTML remainder of the campaign target.

The repository has no stable identifiers, source bytes, source digests, C
provenance, mutation seeds, mutation recipes, or original sweep log for them.

The following searches found no full artifact or regeneration harness:

- Git history for `compact_route_differential_fuzz`.
- Tracked and working-tree files for the same origin string.
- PR #573 file list and its merged commits.
- Issue #587 and its linked resolution.

The only related committed artifact is the 20-record manifest above. Generic
panic fuzzers do not contain the missing witness set.

## Independent gate

Command:

```text
bash cgo_harness/docker/run_parity_in_docker.sh --no-build \
  --run '^TestCompactT3OracleAdjudication$' -- \
  'cd /workspace/cgo_harness && go test . -tags treesitter_c_parity \
  -run "^TestCompactT3OracleAdjudication$" -count=1 -parallel 1 \
  -timeout 20m -v'
```

Result: `PASS`, 20 of 20 witness subtests, no out-of-memory kill, and no wall
timeout.

Run artifact:

`/tmp/gotreesitter-r1-b0/harness_out/docker/20260811T223010Z/`

The gate also reports the existing structural findings. Production differs
from the C oracle on all 20 records below the root. This is outside B0.

## Required follow-up

Regenerate the missing 78 records from a reviewed, deterministic differential
fuzz harness. Pin every source, seed, oracle revision, and digest.

Keep the compact route unchanged until the restored denominator is available.

This receipt changes no parser code and no benchmark input.
