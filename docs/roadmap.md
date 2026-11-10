# Roadmap

The [v1 design](v1-design.md) is the current roadmap.
It defines milestones M0–M4 and governs engine work.
It replaces the earlier cleanup roadmap and its deferred-architecture restrictions.

The full design is `hypha://m31labs/gotreesitter/specs/spec.gotreesitter-v1-design`, dated 2026-09-25.
It supersedes Campaign v7 where they conflict and fully supersedes the compact graduation rider.

## Now

On 2026-11-10 the owner decided that a C-faithful core replaces the compact scheduler, which is frozen. See the [decision](v1-design.md#decision-of-2026-11-10-a-c-faithful-core).

- `internal/ccore` is the C-faithful core. It has been in the tree since v0.58.0.
- The required CI job `ccore-equality-cgo` compares it with pinned C 0.27.1 for json, python, c, and typescript on every pull request that changes code.
- No public API reaches it. `Parse` and incremental parsing still run on the existing engines.

The aim is exact tree-sitter behaviour in pure Go, on every target Go builds for, with less work per grammar than the compact scheduler needed.

## Next

This is the plan, without dates:

1. Free memory during long edit sessions: a subtree store with reference counts.
2. A route that lets `Parse` and incremental parsing use the core for certified grammars.
3. Included ranges and exact lexer tables.
4. `grammargen` enhancements, so that the grammars it builds run on the core, and grammar format v1 (D12).
5. Graduation of grammars one cohort at a time, each behind a per-grammar certificate.
6. A release candidate, `v1.0.0-rc.1`, that flips the global default (D7).
7. Deletion of the legacy engine and the compact scheduler, then the root cleanup.

## References

Use these references:

- [v1 design](v1-design.md) for decisions, gates, lanes, owner decisions, and graduation cohorts.
- [Repository map](repository-map.md) for current ownership and planned layout phases.
- [AGENTS.md](../AGENTS.md) for the required workflow and prose rules.
- [Release process](releasing.md) for publication procedures.
- [CHANGELOG.md](../CHANGELOG.md) and its [archive](changelog/) for shipped evidence.

Keep status and evidence in the v1 milestone issues and their linked work items.
Attach reproducible evidence to PRs or release assets.
The owner decided O-Q1 to O-Q7 on 2026-09-26. The [v1 design](v1-design.md#owner-decisions) lists the decisions.
Write plainly: lead with the point, use common words and the active voice, keep each term consistent, back claims with evidence (numbers, links, test output), and say what you did not verify.

R3 keeps the counter-ledger 2% ratchet strict. A PR may reset rows or move a guarded counter only when its body lists every changed row, the old and new values, and the reason, and the owner explicitly approves that PR. Agents never approve ledger exceptions. See the [R3 counter-ledger rule](v1-design.md#r3-counter-ledger-changes).
