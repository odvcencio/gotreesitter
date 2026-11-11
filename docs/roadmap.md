# Roadmap

The [v1 design](v1-design.md) defines engine requirements and validation gates.

Use these references:

- [v1 design](v1-design.md) for engine requirements and validation gates.
- [Repository map](repository-map.md) for file locations and ownership.
- [AGENTS.md](../AGENTS.md) for the required workflow and prose rules.
- [Release process](releasing.md) for publication procedures.
- [CHANGELOG.md](../CHANGELOG.md) and its [archive](changelog/) for shipped evidence.

Keep status in pull requests and their linked issues.
Attach reproducible evidence to PRs or release assets.
Write plainly: lead with the point, use common words and the active voice, keep each term consistent, back claims with evidence (numbers, links, test output), and say what you did not verify.

R3 keeps the counter-ledger 2% ratchet strict. A PR may reset rows or move a guarded counter only when its body lists every changed row, the old and new values, and the reason, and the owner explicitly approves that PR. Agents never approve ledger exceptions. See the [R3 counter-ledger rule](v1-design.md#r3-counter-ledger-changes).
