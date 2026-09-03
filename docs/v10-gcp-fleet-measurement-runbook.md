---
mdpp: "0.1"
id: runbook.v10-gcp-fleet-measurement
type: runbook
space: hypha://m31labs/gotreesitter
status: active
created: 2026-08-03
tags: [performance, gcp, fleet, bisect, runbook]
related:
  - hypha://m31labs/gotreesitter/object/concept.quiet-host-protocol
  - hypha://m31labs/gotreesitter/object/spec.c4-bytecode-isa.v1
---

# V10 full-fleet measurement on a fresh Google Cloud VM

Use this runbook to measure the v10 parser on a fresh Google Compute Engine
(GCE) virtual machine. Run the complete 206-language fleet at every bisect
boundary. Keep each result with its immutable source and corpus identity.

This runbook extends phase3-admission-timing-runbook.md. The phase-3 runbook
covers the four canonical fixtures. This runbook covers the authenticated full
fleet and the regression ladder.

## Record the measurement identity

Set these values before provisioning the machine:

~~~sh
export GCP_PROJECT=bookt-cc
export GCP_ZONE=us-central1-c
export VM_NAME=gts-v10-$(date -u +%Y%m%d-%H%M%S)
export PERF_HOSTNAME=gts-v10-full-fleet
export CORPUS_LOCK_SHA=41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea
export V10_VM_MAX_RUN_SECONDS=32400
export V10_WALL_TIMEOUT=8h
export V10_GO_TIMEOUT=7h50m
export V10_LANG_TIMEOUT_MS=240000
export V10_MAX_COMPUTE_USD=3.00
~~~

Record the following values in the final receipt:

- repository commit and clean-worktree result;
- machine type, zone, and pinned CPU;
- Google Cloud image name and digest;
- Go and Docker image identities;
- corpus lock path and SHA-256 digest;
- full-fleet language count;
- virtual-machine, campaign, and language time limits;
- the current Spot price and the maximum estimated cost;
- output directory and raw logs.

Do not publish a partial or interrupted scan.

## Provision the VM

Check authentication and the selected project:

~~~sh
gcloud auth list
gcloud config set project "$GCP_PROJECT"
gcloud compute instances list --project "$GCP_PROJECT"
~~~

Use a dedicated eight-vCPU Compute-Optimized VM. Use Spot capacity and delete
the VM after the run. The earlier v9 run found no regional quota for the
preferred N2D shape, so use C2 unless N2D quota is confirmed.

~~~sh
gcloud compute machine-types describe n2d-standard-4 \
  --zone "$GCP_ZONE" >/dev/null 2>&1 || true

gcloud compute instances create "$VM_NAME" \
  --project "$GCP_PROJECT" \
  --zone "$GCP_ZONE" \
  --machine-type c2-standard-8 \
  --provisioning-model SPOT \
  --instance-termination-action DELETE \
  --max-run-duration "${V10_VM_MAX_RUN_SECONDS}s" \
  --image-family ubuntu-2404-lts-amd64 \
  --image-project ubuntu-os-cloud \
  --boot-disk-size 100GB \
  --boot-disk-type pd-balanced \
  --metadata enable-oslogin=TRUE \
  --labels purpose=gotreesitter-v10,max-run=9h
~~~

Use a regular VM when Spot capacity cannot satisfy the measurement window.
Do not run another workload on this VM.

Check the current Spot price before creation. Multiply that price by nine
hours, then add the disk estimate. Do not start above
`V10_MAX_COMPUTE_USD`. Record the estimate in the receipt.

The nine-hour VM limit includes installation and corpus staging. Google Cloud
deletes the VM when this limit expires. Do not extend the limit in place.

## Install the fresh machine

Connect to the VM and install only the required tools:

~~~sh
gcloud compute ssh "$VM_NAME" --project "$GCP_PROJECT" --zone "$GCP_ZONE"
~~~

Run these commands on the VM:

~~~sh
sudo apt-get update
sudo apt-get install -y ca-certificates git curl jq tmux util-linux \
  build-essential docker.io golang-go
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"
newgrp docker
docker version
export PERF_HOSTNAME=gts-v10-full-fleet
export CORPUS_LOCK_SHA=41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea
~~~

Keep the host quiet. Do not install monitoring agents, cron jobs, or package
updates during the timing window. Pin one CPU for the container and reserve
the other CPUs for Docker and the operating system.

## Check out the source

Clone the private repository or the exact repository mirror used for the run.
Then check out the target commit in detached mode:

~~~sh
sudo mkdir -p /srv/gotreesitter-perf
sudo chown "$USER":"$USER" /srv/gotreesitter-perf
git clone https://github.com/odvcencio/gotreesitter.git \
  /srv/gotreesitter-perf/gotreesitter
cd /srv/gotreesitter-perf/gotreesitter
git fetch --tags --prune origin
git checkout --detach "$TARGET_REV"
test -z "$(git status --porcelain)"
git rev-parse HEAD
~~~

Replace TARGET_REV with the full 40-character commit before running the
commands. Record the printed commit.

## Stage the authenticated corpus

The full-fleet runner uses an external corpus checkout. It does not use the
grammar lock in grammars/languages.lock. Copy the exact external lock from a
trusted measurement host:

~~~sh
# Run this command from the trusted host.
gcloud compute scp \
  /home/draco/work/gotreesitter-corpora/corpus_sources.lock \
  "$VM_NAME:/srv/gotreesitter-perf/corpus_sources.lock" \
  --project "$GCP_PROJECT" --zone "$GCP_ZONE"
~~~

Verify the lock before cloning any source:

~~~sh
cd /srv/gotreesitter-perf/gotreesitter
sha256sum /srv/gotreesitter-perf/corpus_sources.lock
test "$(sha256sum /srv/gotreesitter-perf/corpus_sources.lock | awk '{print $1}')" = \
  "$CORPUS_LOCK_SHA"
~~~

Materialize every locked source with the repository helper:

~~~sh
mkdir -p /srv/gotreesitter-perf/corpus_sources
cd /srv/gotreesitter-perf/gotreesitter/cgo_harness
GOWORK=off go run ./cmd/real_corpus_sources \
  --lock /srv/gotreesitter-perf/corpus_sources.lock \
  --root /srv/gotreesitter-perf/corpus_sources \
  --langs all206 \
  --apply \
  --out-json /srv/gotreesitter-perf/corpus-source-status.json
~~~

Inspect the status file. Require 206 locked languages, matching commits, and
no source errors. The sparse Git clones can use substantial disk space.
Keep at least 40 GB free after materializing the corpus. Increase the boot
disk size when the host has less free space.

Extract the languages whose fixtures are embedded inside another syntax. The
fleet runner reads them from `.gts-extracted/<language>` directories, and
`real_corpus_sources` does not create those directories:

~~~sh
cd /srv/gotreesitter-perf/gotreesitter/cgo_harness
GOWORK=off go run ./cmd/real_corpus_extract \
  --root /srv/gotreesitter-perf/corpus_sources \
  --langs comment,doxygen,gitcommit,jsdoc,markdown_inline
~~~

Confirm that each of the five directories exists before the scan. A missing
directory produces a `no_corpus` coverage finding for that language and
removes its rows from the fleet denominator. The 2026-08-30 run lost 12 rows
this way.

## Run the authoritative full fleet

Build the pinned local Docker image and run the scan from the checked-out
repository:

~~~sh
cd /srv/gotreesitter-perf/gotreesitter
REV=$(git rev-parse HEAD)
OUT="cgo_harness/perf_scan/out/v10-${REV}-$(date -u +%Y%m%dT%H%M%SZ)"

bash cgo_harness/docker/run_parity_in_docker.sh \
  --label "v10-${REV}" \
  --memory 8g \
  --cpus 1 \
  --cpuset-cpus 4 \
  --pids 4096 \
  --gomemlimit 6GiB \
  --timeout "$V10_GO_TIMEOUT" \
  --wall-timeout "$V10_WALL_TIMEOUT" \
  --hostname "$PERF_HOSTNAME" \
  --mount /srv/gotreesitter-perf:/corpus:ro \
  -- \
  "cd /workspace/cgo_harness && \
   GOWORK=off \
   GTS_PARITY_ALLOW_HOST=1 \
   GTS_PERF_SCAN=1 \
   GTS_PERF_SCAN_HARD_GATE=1 \
   GTS_PERF_SCAN_REQUIRE_FLEET=1 \
   GTS_PERF_SCAN_GIT_REVISION=$REV \
   GTS_PERF_SCAN_GIT_CLEAN=1 \
   GTS_PERF_SCAN_CORPUS_ROOT=/corpus/corpus_sources \
   GTS_REAL_CORPUS_BENCH_LOCK=/corpus/corpus_sources.lock \
   GTS_PERF_SCAN_MAX_FILES=8 \
   GTS_PERF_SCAN_ORDER=largest \
   GTS_PERF_SCAN_REPS=5 \
   GTS_PERF_SCAN_FILE_BUDGET_MS=10000 \
   GTS_PERF_SCAN_LANG_TIMEOUT_MS=$V10_LANG_TIMEOUT_MS \
   GTS_PERF_SCAN_CHILD_RSS_LIMIT_MB=6144 \
   GTS_PERF_SCAN_OUT=/workspace/$OUT \
   go test -tags 'treesitter_c_parity treesitter_c_perfscan' \
     -run '^TestPerfScanSweep$' -v -count=1 -timeout $V10_GO_TIMEOUT ."
~~~

The wrapper returns exit code 124 when the wall limit expires. It stops the
container and writes `inspect.json` and `metadata.txt`. Treat that run as
incomplete.

## Monitor campaign checkpoints

Open a second shell on the VM. Count completed language fragments every five
minutes:

~~~sh
cd /srv/gotreesitter-perf/gotreesitter
export OUT='cgo_harness/perf_scan/out/<exact-v10-output-directory>'
find "$OUT/langs" -maxdepth 1 -type f -name '*.json' | wc -l
~~~

Record elapsed time at 52, 103, and 155 completed languages. Project the final
duration after each checkpoint. Stop the run when the projection exceeds the
eight-hour wall limit.

Each language writes its fragment and log before the next language starts.
Copy these files after an interrupted run. Never promote them to an accepted
scoreboard.

The explicit revision and clean-source values authenticate the Docker root
when Git VCS metadata is unavailable inside the container. Use them only after
the host checkout passes the clean-worktree check.

Require one authenticated scoreboard row for every locked language. Preserve
the container log, metadata file, scoreboard, and Markdown report.

## Run the bisect ladder

Run the same full-fleet command at each immutable boundary. Keep one output
directory per revision. Do not change the machine, CPU pin, corpus lock, Go
image, Docker image, file budget, or repetition count between revisions.

Use this ladder from the sealed v8 baseline through the v9 boundary:

| label | commit | known change |
|---|---|---|
| v8 | 3325e0b107cdc23a1092714e311dbe426f59bb70 | sealed baseline |
| pr605 | 8f93cecf | leaf tiling |
| pr614 | df619d1c | de-indirect dispatch and shadow census; confounded step |
| pr616 | 1deca56a | compact struct widening; largest measured step |
| pr619 | 193c44b3 | recording-on and rank-walk retirement |
| pr622 | 492cd600 | L1 fast paths |
| v10 | <target revision> | current target |

For every row, run:

~~~sh
git checkout --detach "$REV"
test -z "$(git status --porcelain)"
# Re-run the authoritative command above with this REV and a unique OUT path.
~~~

Treat the first statistically significant step as the regression point. Use
the four canonical fixtures for mechanism attribution after the fleet scan.
Use ten or more stable samples for each comparison. Report the geomean and
each language result separately.

The prior quiet GCP ladder provides a hypothesis for the v10 receipt:

- v8 to PR605: about +0.83% geomean; two of four fixtures were significant;
- PR605 to PR614: about +2.08% geomean; all four fixtures were significant;
- PR614 to PR616: about +2.80% geomean; all four fixtures were significant;
- PR616 to PR619: about +0.41% geomean; one of four fixtures was significant;
- PR619 to PR622: about -0.25% geomean; no fixture was significant.

The PR614 step contains two changes. The PR616 struct-widening step is the
largest isolated regression in that ladder. Confirm both statements with the
v10 run before treating them as final fleet evidence.

## Keep production and candidate tags correct

The production full-parse driver must use gts_no_parsercorephase0. The
candidate driver must use gts_parsercorephase0. Do not use the candidate tag
for a production control. A previous GCP run silently measured the compact
route as production after using the wrong tag.

Use the canonical four-fixture driver when you need mechanism-level timing:

~~~sh
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh \
  --go-backend production --core 4 --out "$OUT/production"
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh \
  --go-backend candidate --core 4 --out "$OUT/candidate"
~~~

Run the driver on the same clean revision and pinned core. Read
phase3-admission-timing-runbook.md before using its publication thresholds.

## Bytecode stream specification

The canonical bytecode stream specification is already active in Hyphae:

hypha://m31labs/gotreesitter/object/spec.c4-bytecode-isa.v1

It defines the instruction set, deterministic corridor, generalized-LR (GLR)
fallback, stream layout, decode-back proof, and staged measurement gates. Keep
that Hyphae object as the source of truth. This runbook records measurement
procedure only.

Verify the object before a campaign handoff:

~~~sh
hypha show hypha://m31labs/gotreesitter/object/spec.c4-bytecode-isa.v1 --body
~~~

Do not claim that the bytecode stream is implemented. The specification is a
design brief with explicit correctness and performance gates.

## Preserve and clean up

Copy the complete output directory off the VM before deletion. Store the
receipt with the commit, lock digest, host identity, and regression summary.

~~~sh
gcloud compute scp --recurse \
  "$VM_NAME:/srv/gotreesitter-perf/gotreesitter/cgo_harness/perf_scan/out" \
  ./v10-receipts/ \
  --project "$GCP_PROJECT" --zone "$GCP_ZONE"

gcloud compute instances delete "$VM_NAME" \
  --project "$GCP_PROJECT" --zone "$GCP_ZONE" --quiet
~~~

If the VM terminates before the output is copied, mark the run incomplete.
Do not merge incomplete artifacts into the authoritative fleet scoreboard.

Delete the instance in the same session as the harvest. A stopped instance
still bills its boot disk, and a running one bills the machine until spot
preemption. Confirm with `gcloud compute instances list` that no `gts-v10-*`
instance remains.
