# W5 real-code edit gate

Every code PR runs `editor_latency` in the required CI `build` aggregate.
Trusted repository heads use the configured `GTS_RUNNER_LABELS` pool when
available. Fork heads use disposable GitHub-hosted Linux runners. The job
requires complete paired evidence on the selected runner. Configure a runner
service per VM for quiet measurements; a shared VM's load is recorded in the
receipt and C drift remains visible.

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
use one C grammar lock, compiler, and cache, and receipt content and build
identities must agree. The artifact filename is diagnostic: a cold build loads
`parser.so`, while a warm cache uses a keyed filename. The comparison still
requires the exact artifact SHA-256 and every runtime, grammar, compiler,
linkage, and flag field to match.
Every session step must match fresh Go and fresh C, including the full deep
digest (fields, points, children, flags, errors). Both profiled and unprofiled
Go APIs must pass. Initial fixtures and typing seeds must be clean. Roots must
cover all input, accepted parses must not truncate, `ERROR` roots must report
`HasError`, and a no-edit public reparse must allocate zero objects.

Older baselines can omit the work of a successful fresh-result verification
from their profiles. The campaign restores that already executed attempt with
a narrow profiling adapter before releasing its fresh tree. It also counts
fresh arena clones omitted by the old loop counter and lazy node views created
by the unchanged comparison, once per distinct arena.
The forest adapter counts tokens from successful forest parses and tokens and
nodes from discarded forest attempts before their arenas are released. It
counts the selected verification arena once, through the existing adapter.
The same instrumentation counts every legacy attempt inside the verifier,
including retries whose discarded trees never reached the old profile. It
subtracts the selected arena and attempt already charged by the old adapter.
Counters reset for each profiled operation. Unprofiled parses retain their
original lexer, worklist, selection, and budget checks.
The adapter preserves result selection and the unprofiled timing path. `environment.json`
records the original and instrumented source hashes and the external patch's
hash. Unknown accounting layouts fail closed. The profiled and unprofiled
parity checks still run at every step, so both revisions' counters describe
the complete operation.

Correctness runs in Docker, one language at a time, before any timing. The
paired deterministic ledger then rejects work increases and reuse decreases
above the existing 2% threshold, step by step. It does not refresh any pins.

Fork pull requests run the same campaign on disposable GitHub-hosted Linux
runners against the exact head commit and PR base. Those containers use a 6 GiB
memory limit and a 4 GiB Go memory target. Configured self-hosted runners retain
the 8 GiB container limit and 6 GiB Go target. Both revisions share the same
limits, pinned CPU, fixtures, counters, seeds, and timing threshold. The required
job also runs the Python evidence and failure-path tests before the campaign.

Timing uses `scripts/run_randomized_benchmarks.sh`: twenty shuffle seeds,
one process per revision and seed, `GOMAXPROCS=1`, `GOWORK=off`, `-count=1`,
`-benchtime=750ms`, and `-benchmem`. Base/head order alternates each seed.
Within each shuffled workload the order is Go-C-C-Go. Both revisions run in
the same Docker container for each paired seed, on the same pinned CPU for a
language. Each seed has its own container wall timeout; slow languages can
complete all twenty seeds without sharing one container's time limit. Every
fragment must contain both revisions and all twelve benchmark rows before it
can enter the complete campaign. Campaigns
serialize with a host lock, opened read-only so runner services using different
VM users still contend on the same file. The lock remains in place between
campaigns. The benchmark wrapper also retains its own lock.
After each container, the Docker wrapper returns generated files to the runner
user so the next job can clean its checkout and campaign scratch directory.

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

Baseline profile instrumentation also follows the existing pooled recovery parser. It records those hidden attempts before their arenas are released, restores the parser’s prior profiling scope, and charges the parent operation. This includes recovery probes that are discarded before the selected tree returns.
