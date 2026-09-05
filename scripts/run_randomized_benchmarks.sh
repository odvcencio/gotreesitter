#!/usr/bin/env bash

set -euo pipefail

usage() {
	cat >&2 <<'EOF'
Usage: scripts/run_randomized_benchmarks.sh --output PATH [options]

Run the combined performance set once for each shuffle seed.
Pair a baseline with the current directory to alternate execution order.

Options:
  --output PATH       Write raw output for the current directory to PATH.
  --baseline-root DIR Run the same seeds in DIR. Requires --baseline-output.
  --baseline-output PATH
                      Write baseline output to PATH. Requires --baseline-root.
  --lock-path PATH    Host lock path. Default: /tmp/gotreesitter-run-randomized-benchmarks.lock.
  --runs N            Number of seeds to run. Default: 20.
  --seed-start N      First shuffle seed. Default: 1.
  --benchtime D       Benchmark duration. Default: 750ms.
  --tags TAGS         Go build tags. Default: gts_parsercorephase0.
  --bench-regex REGEX Benchmark selection regex. Default: combined set.
  --require-benchmarks CSV
                      Require each exact benchmark name once per process, with all three standard metrics.
  --package PATH      Go package path. Default: .
  --help              Show this help.
EOF
}

output_path=""
baseline_root=""
baseline_output=""
baseline_root_set=0
baseline_output_set=0
lock_path="/tmp/gotreesitter-run-randomized-benchmarks.lock"
runs=20
seed_start=1
benchtime=750ms
build_tags=gts_parsercorephase0
package_path=.
required_benchmarks=""
benchmark_re='^(BenchmarkGoParse(FullDFA|CoreDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA|IncrementalRandomSingleByteEdit)|Benchmark(KDLRecoveryGarbageSuffix|RecoveryCorpusFile)|BenchmarkExpectedRootCanFrameLongRepeat|BenchmarkDiagnosticParserCore(CorridorSchedulerOnly|WarmSchedulerOnlyQueryCompile|WarmMaterializationOnlyQueryCompile)|BenchmarkParserCoreFreshFull(Canonical|SelectedStoreCanonical)|Benchmark(TaggerTag(Tree)?Go|ExtractCodeUnderstanding(Tree)?Go|ExtractAllFactsTreeGo|FactProgram(All)?(Tree)?Go))$'

while (($# > 0)); do
	case "$1" in
	--output)
		if (($# < 2)); then
			printf '%s\n' "--output requires a path" >&2
			exit 2
		fi
		output_path=$2
		shift 2
		;;
	--baseline-root)
		if (($# < 2)) || [[ -z "$2" ]]; then
			printf '%s\n' "--baseline-root requires a directory" >&2
			exit 2
		fi
		baseline_root=$2
		baseline_root_set=1
		shift 2
		;;
	--baseline-output)
		if (($# < 2)) || [[ -z "$2" ]]; then
			printf '%s\n' "--baseline-output requires a path" >&2
			exit 2
		fi
		baseline_output=$2
		baseline_output_set=1
		shift 2
		;;
	--lock-path)
		if (($# < 2)); then
			printf '%s\n' "--lock-path requires a path" >&2
			exit 2
		fi
		lock_path=$2
		shift 2
		;;
	--runs)
		if (($# < 2)); then
			printf '%s\n' "--runs requires a positive integer" >&2
			exit 2
		fi
		runs=$2
		shift 2
		;;
	--seed-start)
		if (($# < 2)); then
			printf '%s\n' "--seed-start requires a non-negative integer" >&2
			exit 2
		fi
		seed_start=$2
		shift 2
		;;
	--benchtime)
		if (($# < 2)); then
			printf '%s\n' "--benchtime requires a duration" >&2
			exit 2
		fi
		benchtime=$2
		shift 2
		;;
	--tags)
		if (($# < 2)); then
			printf '%s\n' "--tags requires a comma-separated tag list" >&2
			exit 2
		fi
		build_tags=$2
		shift 2
		;;
	--bench-regex)
		if (($# < 2)); then
			printf '%s\n' "--bench-regex requires a regex" >&2
			exit 2
		fi
		benchmark_re=$2
		shift 2
		;;
	--require-benchmarks)
		if (($# < 2)) || [[ -z "$2" ]]; then
			printf '%s\n' '--require-benchmarks requires a comma-separated name list' >&2
			exit 2
		fi
		required_benchmarks=$2
		shift 2
		;;
	--package)
		if (($# < 2)); then
			printf '%s\n' "--package requires a path" >&2
			exit 2
		fi
		package_path=$2
		shift 2
		;;
	--help)
		usage
		exit 0
		;;
	*)
		printf 'unknown option: %s\n' "$1" >&2
		usage
		exit 2
		;;
	esac
done

if [[ -z "$output_path" ]]; then
	printf '%s\n' "--output is required" >&2
	usage
	exit 2
fi

if [[ ! "$runs" =~ ^[1-9][0-9]*$ ]]; then
	printf 'invalid --runs value: %s\n' "$runs" >&2
	exit 2
fi

if [[ ! "$seed_start" =~ ^[0-9]+$ ]]; then
	printf 'invalid --seed-start value: %s\n' "$seed_start" >&2
	exit 2
fi

if [[ -z "$lock_path" ]]; then
	printf '%s\n' "--lock-path requires a path" >&2
	exit 2
fi

if [[ -n "$required_benchmarks" ]]; then
	if [[ "$required_benchmarks" == ,* || "$required_benchmarks" == *, || "$required_benchmarks" == *,,* ]]; then
		printf '%s\n' '--require-benchmarks contains an empty name' >&2
		exit 2
	fi
	declare -A required_seen=()
	IFS=',' read -r -a required_names <<<"$required_benchmarks"
	for name in "${required_names[@]}"; do
		if [[ ! "$name" =~ ^Benchmark[^[:space:],]+$ || -n "${required_seen[$name]:-}" ]]; then
			printf 'invalid or duplicate required benchmark name: %s\n' "$name" >&2
			exit 2
		fi
		required_seen[$name]=1
	done
fi

if ((baseline_root_set != baseline_output_set)); then
	printf '%s\n' '--baseline-root and --baseline-output must be supplied together' >&2
	exit 2
fi

head_root=$(pwd -P)
protocol=single-seed-processes
if ((baseline_root_set)); then
	if [[ ! -d "$baseline_root" ]]; then
		printf 'baseline directory does not exist: %s\n' "$baseline_root" >&2
		exit 2
	fi
	baseline_root=$(cd -- "$baseline_root" && pwd -P)
	protocol=paired-alternating-seeds
fi

if ! command -v realpath >/dev/null 2>&1; then
	printf '%s\n' 'realpath is required to validate output paths' >&2
	exit 2
fi

requested_outputs=("$output_path")
if ((baseline_root_set)); then
	requested_outputs+=("$baseline_output")
fi
for path in "${requested_outputs[@]}"; do
	if [[ -e "$path" || -L "$path" ]]; then
		printf 'output already exists: %s\n' "$path" >&2
		exit 2
	fi
done
output_path=$(realpath -m -- "$output_path")
canonical_lock=$(realpath -m -- "$lock_path")
if ((baseline_root_set)); then
	baseline_output=$(realpath -m -- "$baseline_output")
	if [[ "$output_path" == "$baseline_output" ]]; then
		printf '%s\n' 'baseline and current output paths must be distinct' >&2
		exit 2
	fi
fi
if [[ "$output_path" == "$canonical_lock" || "$baseline_output" == "$canonical_lock" ]]; then
	printf '%s\n' 'output and lock paths must be distinct' >&2
	exit 2
fi

if ! command -v flock >/dev/null 2>&1; then
	printf '%s\n' 'flock is required to serialize benchmark campaigns' >&2
	exit 2
fi

lock_dir=$(dirname -- "$lock_path")
mkdir -p -- "$lock_dir"
lock_fd=""
lock_held=0
created_outputs=()

mark_incomplete() {
	local path
	for path in "${created_outputs[@]}"; do
		printf '# status: incomplete\n' >>"$path" || true
	done
}

release_lock() {
	if ((lock_held == 0)); then
		return 0
	fi
	: >"$lock_path" || true
	flock -u "$lock_fd" || true
	lock_held=0
}

exit_with_lock_status() {
	local status=$?
	if ((status != 0)); then
		mark_incomplete
	fi
	release_lock
	exit "$status"
}

exit_after_signal() {
	local signal=$1
	mark_incomplete
	release_lock
	trap - "$signal"
	kill -s "$signal" "$$"
}

trap exit_with_lock_status EXIT
trap 'exit_after_signal HUP' HUP
trap 'exit_after_signal INT' INT
trap 'exit_after_signal TERM' TERM

if ! exec {lock_fd}>>"$lock_path"; then
	printf 'cannot open benchmark lock: %s\n' "$lock_path" >&2
	exit 2
fi

if ! flock -n "$lock_fd"; then
	printf 'benchmark lock is busy: %s\n' "$lock_path" >&2
	if [[ -s "$lock_path" ]]; then
		sed 's/^/owner: /' <"$lock_path" >&2
	else
		printf '%s\n' 'owner: metadata unavailable' >&2
	fi
	exit 75
fi

lock_held=1
lock_started=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
lock_cwd=$(pwd -P)
{
	printf 'pid=%s\n' "$$"
	printf 'started=%s\n' "$lock_started"
	printf 'cwd=%s\n' "$lock_cwd"
	printf 'output=%s\n' "$output_path"
	if ((baseline_root_set)); then
		printf 'baseline_root=%s\n' "$baseline_root"
		printf 'baseline_output=%s\n' "$baseline_output"
	fi
} >"$lock_path"

source_metadata() {
	local root=$1 suffix=$2 repository revision dirty state
	repository=$(git --no-optional-locks -C "$root" rev-parse --show-toplevel 2>/dev/null) || repository=unavailable
	revision=$(git --no-optional-locks -C "$root" rev-parse --verify HEAD 2>/dev/null) || revision=unavailable
	dirty=unknown
	if state=$(git --no-optional-locks -C "$root" status --porcelain=v1 --untracked-files=normal 2>/dev/null); then
		dirty=false
		[[ -z "$state" ]] || dirty=true
	fi
	printf '# repository root%s: %s\n' "$suffix" "$repository"
	printf '# revision%s: %s\n' "$suffix" "$revision"
	printf '# dirty%s: %s\n' "$suffix" "$dirty"
}

initialize_output() {
	local root=$1 role=$2 path=$3 identity
	identity=$(source_metadata "$root" '')
	mkdir -p -- "$(dirname -- "$path")"
	if ! (set -o noclobber; : >"$path"); then
		printf 'cannot create output without overwriting it: %s\n' "$path" >&2
		return 2
	fi
	created_outputs+=("$path")
	{
		printf '# status: incomplete\n'
		printf '# protocol: %s\n' "$protocol"
		printf '# role: %s\n' "$role"
		printf '# working directory: %s\n' "$root"
		printf '%s\n' "$identity"
		printf '# source metadata: informational; no source snapshot authentication\n'
		printf '# started: %s\n' "$lock_started"
		printf '# package: %s\n' "$package_path"
		printf '# bench regex: %s\n' "$benchmark_re"
		printf '# required benchmarks: %s\n' "${required_benchmarks:-none}"
		printf '# build tags: %s\n' "$build_tags"
		printf '# seeds: %s..%s\n' "$seed_start" "$((seed_start + runs - 1))"
		printf '# benchtime: %s\n' "$benchtime"
		printf '# GOMAXPROCS: 1\n'
		printf '# count per process: 1\n'
	} >>"$path"
}

run_seed() {
	local root=$1 role=$2 path=$3 seed=$4 position=$5 sample_start
	printf 'running %s shuffle seed %d\n' "$role" "$seed" >&2
	printf '# seed: %d; position: %d\n' "$seed" "$position" >>"$path"
	sample_start=$(wc -c <"$path")
	(
		cd -- "$root"
		GOMAXPROCS=1 go test -tags "$build_tags" "$package_path" \
			-run '^$' \
			-bench "$benchmark_re" \
			-benchmem \
			-count=1 \
			-benchtime="$benchtime" \
			-shuffle="$seed"
	) >>"$path"
	if ! tail -c "+$((sample_start + 1))" "$path" | GTS_BENCH_REQUIRED_NAMES="$required_benchmarks" awk '
		BEGIN {
			required = ENVIRON["GTS_BENCH_REQUIRED_NAMES"]
			if (required != "") {
				n = split(required, names, ",")
				for (i = 1; i <= n; i++) wanted[names[i]] = 1
			}
		}
		/^Benchmark[^[:space:]]+[[:space:]]/ && $2 ~ /^[1-9][0-9]*$/ {
			name = $1
			if (!(name in wanted)) sub(/-[0-9]+$/, "", name)
			ns = bytes = allocs = 0
			for (i = 3; i < NF; i += 2) {
				if ($i ~ /^([0-9]+([.][0-9]*)?|[.][0-9]+)([eE][+-]?[0-9]+)?$/) {
					if ($(i + 1) == "ns/op") ns = 1
					if ($(i + 1) == "B/op") bytes = 1
					if ($(i + 1) == "allocs/op") allocs = 1
				}
			}
			if (ns) found = 1
			if (name in wanted) {
				counts[name]++
				if (!ns || !bytes || !allocs) invalid[name] = 1
			}
		}
		END {
			bad = !found
			for (name in wanted) {
				if (counts[name] != 1 || invalid[name]) {
					printf "required benchmark %s needs one valid ns/op, B/op, and allocs/op row; rows=%d\n", name, counts[name] > "/dev/stderr"
					bad = 1
				}
			}
			exit bad
		}
	'; then
		printf '%s seed %d produced no valid benchmark timing row or an incomplete required set\n' "$role" "$seed" >&2
		return 1
	fi
}

printf 'randomized benchmark output: %s\n' "$output_path" >&2
printf 'seeds: %s..%s\n' "$seed_start" "$((seed_start + runs - 1))" >&2
printf 'benchtime: %s\n' "$benchtime" >&2
initialize_output "$head_root" head "$output_path"
if ((baseline_root_set)); then
	printf 'randomized baseline output: %s\n' "$baseline_output" >&2
	initialize_output "$baseline_root" baseline "$baseline_output"
fi

for ((offset = 0; offset < runs; offset++)); do
	seed=$((seed_start + offset))
	if ((baseline_root_set == 0)); then
		run_seed "$head_root" head "$output_path" "$seed" 1
	elif ((offset % 2 == 0)); then
		run_seed "$baseline_root" baseline "$baseline_output" "$seed" 1
		run_seed "$head_root" head "$output_path" "$seed" 2
	else
		run_seed "$head_root" head "$output_path" "$seed" 1
		run_seed "$baseline_root" baseline "$baseline_output" "$seed" 2
	fi
	seed_completed=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
	for path in "${created_outputs[@]}"; do
		printf '# completed seed: %d; at: %s\n' "$seed" "$seed_completed" >>"$path"
	done
done

source_metadata "$head_root" ' at end' >>"$output_path"
if ((baseline_root_set)); then
	source_metadata "$baseline_root" ' at end' >>"$baseline_output"
fi
for path in "${created_outputs[@]}"; do
	printf '# completed runs: %d\n# status: complete\n' "$runs" >>"$path"
done
printf 'completed %d randomized runs\n' "$runs" >&2
