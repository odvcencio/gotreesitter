# Release policy

GoTreeSitter uses continuous merge flow and milestone-based release trains. A
merged pull request does not create a release obligation. One minor train is
active at a time, and it closes only when it tells one coherent user story,
clears its declared gates, and has soaked on `main`.

## Cadence

The default is no more than one planned minor release in a rolling seven-day
period. This is backpressure, not a promise to publish weekly and not an
absolute embargo. A security fix, severe user-visible regression, invalid
certification, announced deadline, or explicit owner ruling may override the
interval. The train receipt must record the reason.

A normal `0.x` minor release contains one substantial user-visible capability
or roughly three to seven grouped, meaningful outcomes supporting the same
narrative. Pull-request count is not a sizing metric: one outcome may take many
PRs, and internal maintenance may accumulate without forcing a tag.

Patch releases are reserved for an urgent user-visible regression, security
issue, silent wrong-tree or corrupt-data risk, install/build break, or evidence
that invalidates a published certification. Documentation, cleanup, ordinary
maintenance, and a single non-urgent fix wait for the next minor train.

## Opening a train

Each train has:

- a version, theme, owner, and GitHub milestone;
- included user outcomes and explicit deferrals;
- correctness and release gates appropriate to its changed surface; and
- a receipt recording the exact commit, checks, exceptions, known residuals,
  and rollback or escape hatch where relevant.

Classify each user-visible PR as current-train, later-train, or no release note.
Re-scope the train if new work no longer supports its one-sentence theme.

## Closing a minor release

A minor release is ready only when:

1. Its promised outcome is complete. Unfinished work is explicitly deferred,
   not hidden in the release notes.
2. Correctness and release gates for the affected surface are green.
   Correctness remains separate from performance. Advisory performance work
   does not block a correct, portable, inspectable release unless the train
   declared a performance promise.
3. `CHANGELOG.md` groups outcomes and explains their operational meaning rather
   than listing commits. README and install examples identify the actual latest
   release.
4. The supported install path and smallest representative parse smoke test pass
   against the proposed tag or release candidate.
5. The candidate has soaked on `main` for at least 24 hours. Use at least 48
   hours for parser routing, grammar blob/table formats, materialization
   ownership, or broad scanner-checkpoint changes.
6. The release receipt is complete.

Prefer a release candidate for high-risk parser routing, grammar artifacts or
table-format changes, broad scanner-state changes, and talk-freeze candidates.
During a declared freeze, the freeze checklist is the stricter authority.

## Measuring the policy

Review this policy after three minor trains. Track days between releases,
grouped outcomes per train, patch reasons, time-to-close, install failures, and
release-announcement engagement. GitHub-star movement is worth observing over
the same window, but it is not a release gate and cannot establish why an
individual user starred or unstarred the repository.

The active train is recorded under [`docs/release-trains`](release-trains/).
