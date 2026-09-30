# W5 real-code edit gate

Every code PR runs `editor_latency` in the required CI `build` aggregate.
The configured `GTS_RUNNER_LABELS` pool must select `self-hosted,gts-vm`.
The job fails if it cannot produce complete evidence on that pool. Configure
a runner service per VM for quiet measurements; a shared VM's load is recorded
in the receipt and C drift remains visible.

Reproduce from a clean checkout:

```sh
GOWORK=off bash scripts/run_editor_latency_gate.sh \
  --base <full-PR-base-commit> --output /tmp/w5-new-campaign
```

The corpus lock URL comes from `GTS_CORPUS_LOCK_URL`. The runner authenticates
it against the committed SHA-256, checks each fixture's repository and commit,
fetches immutable whole-file bytes, and verifies their hashes and sizes.
The lock is deleted from the external output directory immediately after
authentication. It must never enter the repository or uploaded evidence.

`fixtures.json` fixes the first eight languages in
`grammars/update_tier1_top50.txt`, plus Go and PowerShell. It contains every
source, edit position, replacement byte, comment, and typing seed. Runtime
fixture selection, known-difference skips, and timing eligibility filters are
absent. An engine change cannot silently drop a failing file or cell.

Each language has three cells:

- `one_byte`: change one byte of an identifier or string, then undo it.
- `hundred_byte`: insert exactly 100 comment bytes at a fixed middle line,
  then delete them.
- `typing`: append 17 bytes, one keystroke at a time, at EOF. The seed has a
  complete line-comment prefix; PowerShell has a variable prefix. This receipt
  covers those valid typing states, not malformed comment-prefix recovery.

An operation completes the entire session; calibration cannot omit later
keystrokes. Results divide session time and allocations by `edits/op`.
Go's fresh typing-session resets and fixture preparation are untimed. C
restores an immutable seed tree with `Clone`; admission proves both that the
seed stays immutable and that replay with the same parser matches fresh C.
C typing uses a clock around each whole session so repeated timer memory
snapshots cannot dominate its actual edit work. Its clone is excluded from
ns/op and included in its small Go binding allocation metrics. The timer
includes `Tree.Edit`, the public reparse route, and previous-tree release.
It includes registry token-source construction where that route requires it.
Go and C use identical source bytes and edits. C uses the locked in-process
v0.25.1 runtime and grammar loader, including the Go binding's overhead.
Go allocations measure parser work; C allocation metrics measure the binding,
not native malloc. No absolute Go/C target is introduced by this job.

The exact same driver and C harness compile against the PR base and head in
temporary modules. Base source stays in a detached worktree. Both binaries
use one C grammar lock, compiler, and cache, and receipt identities must agree.
Every session step must match fresh Go and fresh C, including the full deep
digest (fields, points, children, flags, errors). Both profiled and unprofiled
Go APIs must pass. Initial fixtures and typing seeds must be clean. Roots must
cover all input, accepted parses must not truncate, `ERROR` roots must report
`HasError`, and a no-edit public reparse must allocate zero objects.

Correctness runs in Docker, one language at a time, before any timing. The
paired deterministic ledger then rejects work increases and reuse decreases
above the existing 2% threshold, step by step. It does not refresh any pins.

Timing uses `scripts/run_randomized_benchmarks.sh`: twenty shuffle seeds,
one process per revision and seed, `GOMAXPROCS=1`, `GOWORK=off`, `-count=1`,
`-benchtime=750ms`, and `-benchmem`. Base/head order alternates each seed.
Within each shuffled workload the order is Go-C-C-Go. Both revisions run in
the same Docker container and on the same pinned CPU for a language. Campaigns
serialize with a host lock. The benchmark wrapper also retains its own lock.

For each seed, average the two Go samples and the two C samples separately.
Fail a cell when the median of its twenty paired `head Go / base Go` ratios
exceeds **1.05**. A slowdown in C cannot hide a Go regression. Report Go/C
before and after, paired sample range, C drift, allocation medians, and counters.
The pinned `benchstat` tool also compares normalized metrics per edit.
Missing, duplicate, nonfinite, partial-session, mismatched-oracle, unpaired,
or unfinished evidence fails closed.

`receipt.json`, `counters.json`, admissions, raw randomized output, normalized
output, benchstat tables, and Docker logs form the evidence. `METRIC:` lines
appear in the job log; `summary.md` becomes the GitHub job summary. CI retains
the artifact for 90 days. A failed timing gate still writes the complete
receipt and summary, so its cause remains reviewable. A correctness or resource
failure leaves its logs and blocks the job before timing.

The generated-fixture `TestW5EditorLatencyGate` and manual slow-tier job remain
independent gates. This timing gate does not change their ceilings, the engine
allowlists, the global default, or the existing fleet performance gates.
