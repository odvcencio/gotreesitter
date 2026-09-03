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

1. Freeze a coherent scope. Merge its pull requests and clear the pull-request
   queue; do not release from a feature branch.
2. Move the accumulated changelog entries from `Unreleased` into a dated
   version section. Update the comparison links and the README release status.
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
