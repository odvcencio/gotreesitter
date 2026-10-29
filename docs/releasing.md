# Releasing gotreesitter

## Eligibility

Publish a standard minor release on any day after the exact commit on `main`
passes the full hosted continuous integration (CI) workflow. Require complete
correctness and performance evidence. Do not use a calendar delay as a
replacement for release evidence.

Use the urgent patch route only for urgent correctness, security, or packaging
regressions. It still requires an incident and a reason that delay is worse.

Keep release planning and campaign notes in the private
`hypha://m31labs/gotreesitter` space.

## Release checklist

1. Freeze a coherent scope. Merge the pull requests in that release scope.
   Unrelated open pull requests do not block publication. Do not release from
   a feature branch.
2. Move the accumulated changelog entries from `Unreleased` into a dated
   version section. Update the comparison links and the README release status. Give the
   section the shape in "Release section template" below.
3. Dispatch the full hosted `ci.yml` workflow for the exact commit on `main`.
   Require its successful completion. Keep correctness, parity, race, and
   performance evidence distinct.
4. Complete the selected performance validation. Record its environment and
   randomized evidence with the release receipt.
5. Dispatch the manual `release.yml` workflow from `main`. Supply the version,
   candidate commit hash, release route, and signed Hyphae receipt ID.
6. For an urgent patch, also supply the incident and explain why waiting is
   worse. Do not use this route for ordinary maintenance.
7. Review the verification evidence. Approve the protected `release`
   environment only when Arbiter selects `Allow`.
8. Let the workflow repeat the mutable checks. Arbiter reevaluates the
   resulting facts before the workflow creates the tag and GitHub release.
9. Verify that the module proxy can fetch the new version.
10. Checkpoint the version, commit hash, gate results, and any intentionally
   deferred work in Hyphae. Close campaign issues only when the release
   contains their documented acceptance evidence.
11. Bump the documentation site. In the `gotreesitter-docs` repository, pin the
   new module version, refresh the embedded release catalog, and deploy the
   new image. The site skips a release when this step is skipped.

Tags are immutable. If a release is wrong, preserve its tag and publish a
follow-up version.

Keep the `receipt_uri` input name for workflow dispatch compatibility. Supply a
signed Hyphae receipt ID, not a `hypha://` URI. Use
`hypha-receipt:YYYY-MM-DD:<short-id>`.

The workflow has no schedule. The Go evidence collector writes deterministic
facts to `policy-facts.json`. It does not choose the release route.

The `policy/release.arb` strategy is the authoritative publication decision.
It selects exactly one `Allow` or `Deny` outcome. The workflow installs Arbiter
v1.9.0 from its digest-pinned release archive.

The strategy denies a release when a fact does not satisfy the release receipt.
These facts include:

- the current `main` commit;
- the exact manual CI run and result;
- the document state;
- the tag and release state;
- the GitHub service result.

The evidence artifacts record the CI run ID, URL, event, commit, conclusion,
and completion time.

## Release section template

Write each dated version section in the changelog in this order. Drop an empty
heading.

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Highlights
- Three bullets at most, in user words, with numbers and units
  (for example "YAML parse time -5.39% on the legacy corpus").

### Upgrade notes
- One bullet per break: what changed, who it affects, and how to fix it.

### Added
### Changed
### Fixed

### Evidence
- Links to the CI run, benchmark receipt, and pull requests.
```

## External activation gates

Keep the release workflow inactive until repository owners confirm these
controls:

- Create the protected `release` environment.
- Require one owner reviewer for that environment.
- Restrict that environment to `main`.
- Set the default workflow permission to read-only.
- Add a `v*` tag ruleset before publication.
- Block tag updates and deletions.
- Restrict tag creation to the governed release workflow when GitHub supports
  that actor rule.

Treat an unsupported tag actor rule as an external blocker. Do not weaken the
local workflow to compensate for it.

### Tag-creation rule gap

The `Protect version tags` ruleset blocks updates and deletions. It has no rule
that restricts tag creation to the release workflow, because GitHub returned
HTTP status `422` for the Actions integration `15368`. Five of the six
releases since v0.52.0 ran without that actor restriction, and the owner
authorized each one separately. v0.54.0 has no recorded exception; this file
does not confirm whether the ruleset lacked the rule at that release. The five
exceptions are:

| Version | Owner authorization date |
| --- | --- |
| v0.52.0 | 2026-09-05 |
| v0.53.0 | 2026-09-19 |
| v0.55.0 | 2026-09-24 |
| v0.55.1 | 2026-09-26 |
| v0.56.0 | 2026-10-04 |

Five exceptions in 30 days is a standing gap, not a one-time event. Either fix
the ruleset or record one standing owner decision. Until then, each release
needs its own owner authorization, which grants no authority for another
version. An exception permits publication without the workflow-only
tag-creation actor restriction. It does not permit manual publication or bypass
another gate. Keep these requirements mandatory:

- Publish from the exact candidate commit on protected `main`.
- Require successful full hosted CI for that exact commit.
- Require the protected release environment and owner approval.
- Require the signed release receipt and governed workflow decision.
- Preserve immutable tags, including rules that block updates and deletions.
- Complete and record the required correctness and performance evidence.

An exception changes neither the validation code nor the release policy.
