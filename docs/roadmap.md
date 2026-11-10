# Roadmap

The [v1 design](v1-design.md) is the current roadmap.
It defines milestones M0–M4 and governs engine work.
It replaces the earlier cleanup roadmap and its deferred-architecture restrictions.

The full design is `hypha://m31labs/gotreesitter/specs/spec.gotreesitter-v1-design`, dated 2026-09-25.
It supersedes Campaign v7 where they conflict and fully supersedes the compact graduation rider.

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
