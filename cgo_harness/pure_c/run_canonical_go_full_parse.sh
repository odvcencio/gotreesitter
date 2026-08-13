#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

MODE="publication"
GO_BACKEND="production"
OUT_DIR=""
CORE=""
CYCLES=5
BENCHTIME="750ms"
SKIP_CGO_ADMISSION=0
SKIP_QUIET_ADMISSION=0
ALLOW_DIRTY=0
ALLOW_GOT_ENV=0
QUIET_MAX_ATTEMPTS=12
QUIET_INTERVAL_SECONDS=10
QUIET_REQUIRED_CONSECUTIVE=3
readonly QUIET_MAX_LOAD1="1.25"
readonly QUIET_MAX_IO_AVG10="0.05"

usage() {
  cat <<'EOF'
Usage: run_canonical_go_full_parse.sh [options]

Produce the canonical authenticated Go-vs-static-C full-parse receipt.
Publication defaults are strict: four frozen fixtures, five Go-C-C-Go cycles
(10 samples per backend/fixture), >=750 ms samples, one pinned CPU, cgo/static
deep admission, bounded quiet-host admission, no GOT_* overrides, and a clean
benchmark worktree.

Options:
  --out <dir>                 New private receipt directory.
  --go-backend <backend>      Go backend: production (default), candidate, or
                              selected-store. Candidate materializes public
                              nodes; selected-store consumes the authenticated
                              compact snapshot directly. Neither falls back.
  --core <cpu>                Pin all calibration and samples to this CPU.
                              Default: least-busy allowed CPU from a 1s probe.
  --quiet-max-attempts <n>    Bounded quiet checks (default: 12).
  --quiet-interval <seconds>  Delay between quiet checks (default: 10).

Diagnostic-only options (receipt is labeled NONPUBLICATION_DIAGNOSTIC):
  --diagnostic                Enable diagnostic mode.
  --cycles <n>                Override five ABBA cycles.
  --benchtime <duration>      Override 750ms (ns, us, ms, or s).
  --skip-cgo-admission        Skip Docker cgo/static deep admission.
  --skip-quiet-admission      Skip bounded quiet-host admission.
  --allow-dirty               Permit a dirty benchmark worktree.
  --allow-got-env             Permit inherited GOT_* overrides.
  -h, --help                  Show this help.
EOF
}

die() {
  echo "canonical receipt: $*" >&2
  exit 2
}

require_uint() {
  local label="$1" value="$2"
  [[ "$value" =~ ^[0-9]+$ ]] || die "$label must be a non-negative integer, got $value"
}

while (($# > 0)); do
  case "$1" in
    --out)
      (($# >= 2)) || die "--out requires a value"
      OUT_DIR="$2"
      shift 2
      ;;
    --core)
      (($# >= 2)) || die "--core requires a value"
      CORE="$2"
      shift 2
      ;;
    --go-backend)
      (($# >= 2)) || die "--go-backend requires a value"
      GO_BACKEND="$2"
      shift 2
      ;;
    --cycles)
      (($# >= 2)) || die "--cycles requires a value"
      CYCLES="$2"
      shift 2
      ;;
    --benchtime)
      (($# >= 2)) || die "--benchtime requires a value"
      BENCHTIME="$2"
      shift 2
      ;;
    --quiet-max-attempts)
      (($# >= 2)) || die "--quiet-max-attempts requires a value"
      QUIET_MAX_ATTEMPTS="$2"
      shift 2
      ;;
    --quiet-interval)
      (($# >= 2)) || die "--quiet-interval requires a value"
      QUIET_INTERVAL_SECONDS="$2"
      shift 2
      ;;
    --diagnostic)
      MODE="diagnostic"
      shift
      ;;
    --skip-cgo-admission)
      SKIP_CGO_ADMISSION=1
      shift
      ;;
    --skip-quiet-admission)
      SKIP_QUIET_ADMISSION=1
      shift
      ;;
    --allow-dirty)
      ALLOW_DIRTY=1
      shift
      ;;
    --allow-got-env)
      ALLOW_GOT_ENV=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

case "$GO_BACKEND" in
  production|candidate|selected-store) ;;
  *) die "go backend must be production, candidate, or selected-store, got $GO_BACKEND" ;;
esac

require_uint "cycles" "$CYCLES"
require_uint "quiet max attempts" "$QUIET_MAX_ATTEMPTS"
require_uint "quiet interval" "$QUIET_INTERVAL_SECONDS"
((CYCLES >= 1)) || die "cycles must be at least 1"
((QUIET_MAX_ATTEMPTS >= QUIET_REQUIRED_CONSECUTIVE)) || die "quiet max attempts must be at least $QUIET_REQUIRED_CONSECUTIVE"

duration_ns() {
  awk -v value="$1" '
    BEGIN {
      if (value ~ /^[0-9]+([.][0-9]+)?ns$/) { sub(/ns$/, "", value); printf "%.0f\n", value; exit }
      if (value ~ /^[0-9]+([.][0-9]+)?us$/) { sub(/us$/, "", value); printf "%.0f\n", value * 1000; exit }
      if (value ~ /^[0-9]+([.][0-9]+)?ms$/) { sub(/ms$/, "", value); printf "%.0f\n", value * 1000000; exit }
      if (value ~ /^[0-9]+([.][0-9]+)?s$/)  { sub(/s$/,  "", value); printf "%.0f\n", value * 1000000000; exit }
      exit 2
    }
  '
}

BENCHTIME_NS="$(duration_ns "$BENCHTIME")" || die "unsupported benchtime $BENCHTIME (use ns, us, ms, or s)"
((BENCHTIME_NS > 0)) || die "benchtime must be positive"

if [[ "$MODE" == "publication" ]]; then
  [[ "$CYCLES" == "5" ]] || die "publication mode requires exactly 5 cycles; use --diagnostic for overrides"
  [[ "$BENCHTIME" == "750ms" ]] || die "publication mode requires benchtime 750ms; use --diagnostic for overrides"
  ((SKIP_CGO_ADMISSION == 0)) || die "publication mode cannot skip cgo/static admission"
  ((SKIP_QUIET_ADMISSION == 0)) || die "publication mode cannot skip quiet admission"
  ((ALLOW_DIRTY == 0)) || die "publication mode cannot allow a dirty worktree"
  ((ALLOW_GOT_ENV == 0)) || die "publication mode cannot allow GOT_* overrides"
fi

for command_name in awk git go gzip lscpu realpath sha256sum stat taskset sort tar /usr/bin/time; do
  command -v "$command_name" >/dev/null 2>&1 || die "required command not found: $command_name"
done

GOT_ENV="$(env | LC_ALL=C sort | awk -F= '$1 ~ /^GOT_/ { print }')"
if [[ -n "$GOT_ENV" && "$ALLOW_GOT_ENV" != "1" ]]; then
  printf 'inherited parser overrides are forbidden:\n%s\n' "$GOT_ENV" >&2
  die "clear GOT_* variables, or use --diagnostic --allow-got-env"
fi

GO_OVERRIDE_ENV="$(
  for name in GOGC GOMEMLIMIT GODEBUG GOEXPERIMENT GOFLAGS GOMAXPROCS; do
    if [[ -v "$name" ]]; then
      printf '%s=%s\n' "$name" "${!name}"
    fi
  done
)"
if [[ "$MODE" == "publication" && -n "$GO_OVERRIDE_ENV" ]]; then
  printf 'inherited Go runtime/build overrides are forbidden:\n%s\n' "$GO_OVERRIDE_ENV" >&2
  die "clear Go performance overrides; the driver sets and records GOMAXPROCS=1"
fi

GIT_HEAD="$(git -C "$REPO_ROOT" rev-parse HEAD)"
GIT_STATUS="$(git -C "$REPO_ROOT" status --porcelain=v1 --untracked-files=all)"
if [[ -n "$GIT_STATUS" && "$ALLOW_DIRTY" != "1" ]]; then
  printf 'benchmark worktree is dirty:\n%s\n' "$GIT_STATUS" >&2
  die "publication requires a clean worktree; diagnostic runs require --diagnostic --allow-dirty"
fi

if [[ -z "$OUT_DIR" ]]; then
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  OUT_DIR="$REPO_ROOT/harness_out/canonical_go_full_parse/$stamp"
fi
OUT_DIR="$(realpath -m "$OUT_DIR")"
[[ ! -e "$OUT_DIR" ]] || die "output path already exists: $OUT_DIR"
umask 077
mkdir -m 0700 -p "$OUT_DIR"/{bin,calibration,fixtures,preflight,raw,time}
printf '%s\n' "$GOT_ENV" > "$OUT_DIR/preflight/inherited_got_overrides.txt"
printf '%s\n' "$GO_OVERRIDE_ENV" > "$OUT_DIR/preflight/inherited_go_overrides.txt"

RUN_COMPLETE=0
ORACLE_TEMP_ROOT=""
on_exit() {
  local code=$?
  if [[ -n "$ORACLE_TEMP_ROOT" ]]; then
    rm -rf -- "$ORACLE_TEMP_ROOT"
  fi
  if ((RUN_COMPLETE == 0)); then
    printf 'receipt_status=FAILED\nexit_code=%d\n' "$code" > "$OUT_DIR/run.status"
  fi
}
trap on_exit EXIT

if [[ "$MODE" == "diagnostic" ]]; then
  RECEIPT_CLASS="NONPUBLICATION_DIAGNOSTIC"
elif [[ "$GO_BACKEND" != "production" ]]; then
  RECEIPT_CLASS="AUTHENTICATED_CANDIDATE"
else
  RECEIPT_CLASS="PUBLICATION"
fi
echo "receipt_class=$RECEIPT_CLASS"
echo "output_dir=$OUT_DIR"

readonly FIXTURE_REQUIRED_NODE_KINDS="import_declaration,comment,method_declaration,selector_expression,parameter_declaration,composite_literal,index_expression,call_expression"
readonly SUITE_REQUIRED_NODE_KINDS="type_arguments,generic_type,qualified_type"
readonly SUITE_REQUIRED_ANY_NODE_KINDS="expression_switch_statement,select_statement"
readonly FIXTURE_SPECS=(
  "query_compile|query_compile.go.gz|20168|b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894|8811a477187606ab2a0be003924e2750c8d10cb5e6ef29e8f0f673c5bd5ccfa4|ecc090a83a4343a1c7c2afbad63277f5b4d60c42d8d94a2af2a9b16e46f2ccb5|8|500|400"
  "rewrite|rewrite.go.gz|5116|74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097|e8c92713adda73b661c1e41dae09d4f76f1447b3f8ddd5b3348b67ce655ee243|b3f9814b65763642d4eac58b9065018048ea13e6f10d56afb28a0479bf5a68a1|8|500|400"
  "language|language.go.gz|41387|009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a|a064d45748fd555209971728d9740510c2ca7449870b54acb861909afea2c8c9|583df223904fe414c33bba3b474c6557ecdb20e7f47e304b9a09bfcc2da44539|8|500|400"
  "grammargen_lr|grammargen_lr.go.gz|235626|a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2|7d64368d4dbffca1b3cc472ff3397871c23e082efb1a2bdba5f9670564cc7b22|1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2|8|500|400"
)
readonly CANDIDATE_WORK_SPECS=(
  # Keep this lock equal to parsercore_phase0_canonical_admission_internal_test.go.
  "query_compile|6685|7509|7509|8108|14730|14789|5546|7542"
  "rewrite|1348|1504|1504|1646|2993|3006|1109|1515"
  "language|6512|7564|7564|8377|15816|15124|5375|7674"
  "grammargen_lr|66115|76310|76310|83658|151917|149768|53896|77168"
)

declare -a FIXTURE_IDS=()
declare -A FIXTURE_SOURCE_SHA=()
declare -A FIXTURE_DEEP_SHA=()
declare -A FIXTURE_MIN_MAX_STACKS=()
declare -A FIXTURE_MIN_MULTI_ITERS=()
declare -A FIXTURE_MIN_MULTI_TOKENS=()
declare -A CANDIDATE_EXPECTED_SHIFTS=()
declare -A CANDIDATE_EXPECTED_REDUCTIONS=()
declare -A CANDIDATE_EXPECTED_POP_REQUESTS=()
declare -A CANDIDATE_EXPECTED_POP_PATHS=()
declare -A CANDIDATE_EXPECTED_POP_PAYLOADS=()
declare -A CANDIDATE_EXPECTED_LINK_ADDITIONS=()
declare -A CANDIDATE_EXPECTED_LEAF_CONSTRUCTIONS=()
declare -A CANDIDATE_EXPECTED_PARENT_CONSTRUCTIONS=()
printf 'fixture\tsource_path\tbytes\tsource_sha256\tcompressed_sha256\tdeep_sha256\tmin_max_stacks\tmin_multi_iters\tmin_multi_tokens\trequired_node_kinds\tsuite_required_node_kinds\tsuite_required_any_node_kinds\n' > "$OUT_DIR/fixture_manifest.tsv"
for spec in "${FIXTURE_SPECS[@]}"; do
  IFS='|' read -r fixture asset_name expected_bytes source_sha compressed_sha deep_sha min_max_stacks min_multi_iters min_multi_tokens <<< "$spec"
  require_uint "$fixture min max stacks" "$min_max_stacks"
  require_uint "$fixture min multi-stack iterations" "$min_multi_iters"
  require_uint "$fixture min multi-stack tokens" "$min_multi_tokens"
  asset_path="$REPO_ROOT/internal/benchfixtures/testdata/$asset_name"
  source_path="$OUT_DIR/fixtures/$fixture.go"
  tmp_path="$source_path.tmp"
  [[ -f "$asset_path" ]] || die "fixture asset missing: $asset_path"
  got_compressed_sha="$(sha256sum "$asset_path" | awk '{print $1}')"
  [[ "$got_compressed_sha" == "$compressed_sha" ]] || die "$fixture compressed sha256=$got_compressed_sha want=$compressed_sha"
  gzip -cd -- "$asset_path" > "$tmp_path"
  got_bytes="$(stat -c '%s' "$tmp_path")"
  got_source_sha="$(sha256sum "$tmp_path" | awk '{print $1}')"
  [[ "$got_bytes" == "$expected_bytes" ]] || die "$fixture bytes=$got_bytes want=$expected_bytes"
  [[ "$got_source_sha" == "$source_sha" ]] || die "$fixture source sha256=$got_source_sha want=$source_sha"
  chmod 0400 "$tmp_path"
  mv "$tmp_path" "$source_path"
  FIXTURE_IDS+=("$fixture")
  FIXTURE_SOURCE_SHA["$fixture"]="$source_sha"
  FIXTURE_DEEP_SHA["$fixture"]="$deep_sha"
  FIXTURE_MIN_MAX_STACKS["$fixture"]="$min_max_stacks"
  FIXTURE_MIN_MULTI_ITERS["$fixture"]="$min_multi_iters"
  FIXTURE_MIN_MULTI_TOKENS["$fixture"]="$min_multi_tokens"
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$fixture" "$source_path" "$expected_bytes" "$source_sha" "$compressed_sha" "$deep_sha" \
    "$min_max_stacks" "$min_multi_iters" "$min_multi_tokens" "$FIXTURE_REQUIRED_NODE_KINDS" \
    "$SUITE_REQUIRED_NODE_KINDS" "$SUITE_REQUIRED_ANY_NODE_KINDS" >> "$OUT_DIR/fixture_manifest.tsv"
done
for spec in "${CANDIDATE_WORK_SPECS[@]}"; do
  IFS='|' read -r fixture shifts reductions pop_requests pop_paths pop_payloads link_additions leaf_constructions parent_constructions <<< "$spec"
  CANDIDATE_EXPECTED_SHIFTS["$fixture"]="$shifts"
  CANDIDATE_EXPECTED_REDUCTIONS["$fixture"]="$reductions"
  CANDIDATE_EXPECTED_POP_REQUESTS["$fixture"]="$pop_requests"
  CANDIDATE_EXPECTED_POP_PATHS["$fixture"]="$pop_paths"
  CANDIDATE_EXPECTED_POP_PAYLOADS["$fixture"]="$pop_payloads"
  CANDIDATE_EXPECTED_LINK_ADDITIONS["$fixture"]="$link_additions"
  CANDIDATE_EXPECTED_LEAF_CONSTRUCTIONS["$fixture"]="$leaf_constructions"
  CANDIDATE_EXPECTED_PARENT_CONSTRUCTIONS["$fixture"]="$parent_constructions"
done

if [[ "$GO_BACKEND" == "candidate" ]]; then
	GO_BUILD_TAGS="gts_parsercorephase0"
	GO_BENCHMARK="BenchmarkParserCoreFreshFullCanonical"
	GO_PREFLIGHT="TestParserCoreFreshFullRunnerRepeatedCanonicalLifecycle"
	GO_LIFECYCLE="parserCoreFreshFullRunner.parse+shallow_completeness+Tree.Release"
	GO_FALLBACK="none_fail_closed"
elif [[ "$GO_BACKEND" == "selected-store" ]]; then
	GO_BUILD_TAGS="gts_parsercorephase0"
	GO_BENCHMARK="BenchmarkParserCoreFreshFullSelectedStoreCanonical"
	GO_PREFLIGHT="TestDiagnosticParserCoreSelectedStoreCanonicalAdmissions"
	GO_LIFECYCLE="parserCoreFreshFullRunner.parseSelectedStore+SelectedCursor traversal+SelectedStore.Release"
	GO_FALLBACK="none_fail_closed"
else
  GO_BUILD_TAGS="gts_no_parsercorephase0"
  GO_BENCHMARK="BenchmarkGoParseWarmRealDFA"
  GO_PREFLIGHT="TestGoFullParseBenchmarkFixturesParseClean"
  GO_LIFECYCLE="Parser.Parse+shallow_completeness+Tree.Release"
  GO_FALLBACK="compact_route_compiled_out"
fi

GO_BIN="$OUT_DIR/bin/gotreesitter.test"
(
  cd "$REPO_ROOT"
  env GOMAXPROCS=1 go test -tags "$GO_BUILD_TAGS" -c -o "$GO_BIN" .
) > "$OUT_DIR/preflight/go_build.txt" 2>&1
"$GO_BIN" -test.list "^${GO_BENCHMARK}$" > "$OUT_DIR/preflight/go_benchmark_list.txt"
grep -qx "$GO_BENCHMARK" "$OUT_DIR/preflight/go_benchmark_list.txt" || die "prebuilt Go test binary does not contain $GO_BENCHMARK"
"$GO_BIN" -test.run "^${GO_PREFLIGHT}$" -test.count=1 -test.v \
  > "$OUT_DIR/preflight/go_workload_identity.txt" 2>&1 || die "Go workload-identity preflight failed"
GO_BIN_SHA="$(sha256sum "$GO_BIN" | awk '{print $1}')"
go version -m "$GO_BIN" > "$OUT_DIR/preflight/go_binary_modules.txt"

if ((SKIP_CGO_ADMISSION == 0)); then
  CGO_BUILD_TAGS="treesitter_c_parity,$GO_BUILD_TAGS"
  (
    cd "$REPO_ROOT"
    bash cgo_harness/docker/run_parity_in_docker.sh -- \
      "cd /workspace && go test -tags '$GO_BUILD_TAGS' . -run '^${GO_PREFLIGHT}$' -count=1 -v && cd /workspace/cgo_harness && GOT_PARSE_PHASE_TIMING=1 go test . -tags '$CGO_BUILD_TAGS' -run '^(TestCanonicalGoBackendBuildTag|TestCanonicalGoBenchmarkPreflight|TestCOracleStaticDeepParity)$' -count=1 -v"
  ) > "$OUT_DIR/preflight/cgo_static_deep_admission.txt" 2>&1
  CGO_ADMISSION="PASS"
else
  CGO_ADMISSION="SKIPPED_NONPUBLICATION_DIAGNOSTIC"
  printf '%s\n' "$CGO_ADMISSION" > "$OUT_DIR/preflight/cgo_static_deep_admission.txt"
fi

C_ORACLE_CACHE="${GTS_CANONICAL_C_ORACLE_CACHE:-}"
if [[ -z "$C_ORACLE_CACHE" ]]; then
  ORACLE_TEMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/gts-canonical-oracle.XXXXXX")"
  chmod 0700 "$ORACLE_TEMP_ROOT"
  C_ORACLE_CACHE="$ORACLE_TEMP_ROOT/c_oracle"
fi
C_BIN=""
for fixture in "${FIXTURE_IDS[@]}"; do
  preflight_out="$OUT_DIR/preflight/static_${fixture}.txt"
  env \
    GTS_C_ORACLE_CACHE="$C_ORACLE_CACHE" \
    GTS_C_ORACLE_EXPECTED_DEEP_SHA256="${FIXTURE_DEEP_SHA[$fixture]}" \
    bash "$SCRIPT_DIR/run_go_benchmark.sh" 500 1 1 "$OUT_DIR/fixtures/$fixture.go" \
    > "$preflight_out" 2>&1
  grep -qx "oracle_source_sha256=${FIXTURE_SOURCE_SHA[$fixture]}" "$preflight_out" || die "$fixture static source identity failed"
  grep -qx "oracle_deep_sha256=${FIXTURE_DEEP_SHA[$fixture]}" "$preflight_out" || die "$fixture static deep identity failed"
  candidate_bin="$(awk -F= '$1 == "oracle_artifact_path" { print substr($0, index($0, "=") + 1); exit }' "$preflight_out")"
  [[ -n "$candidate_bin" && -x "$candidate_bin" ]] || die "$fixture static preflight did not produce an executable artifact"
  if [[ -z "$C_BIN" ]]; then
    C_BIN="$candidate_bin"
  elif [[ "$candidate_bin" != "$C_BIN" ]]; then
    die "static artifact changed across fixture preflights: $C_BIN vs $candidate_bin"
  fi
done
C_BIN_RECEIPT="$OUT_DIR/bin/tree-sitter-c-go-benchmark"
cp -- "$C_BIN" "$C_BIN_RECEIPT"
chmod 0500 "$C_BIN_RECEIPT"
C_BIN="$C_BIN_RECEIPT"
C_BIN_SHA="$(sha256sum "$C_BIN" | awk '{print $1}')"

expand_cpu_list() {
  local list="$1" part start end cpu
  IFS=',' read -ra parts <<< "$list"
  for part in "${parts[@]}"; do
    if [[ "$part" == *-* ]]; then
      start="${part%-*}"
      end="${part#*-}"
      for ((cpu = start; cpu <= end; cpu++)); do
        printf '%s\n' "$cpu"
      done
    else
      printf '%s\n' "$part"
    fi
  done
}

read_cpu_stats() {
  awk '
    /^cpu[0-9]+ / {
      cpu = substr($1, 4)
      total = 0
      for (i = 2; i <= NF; i++) total += $i
      idle = $5 + $6
      print cpu, total, idle
    }
  ' /proc/stat
}

ALLOWED_CPU_LIST="$(awk '/^Cpus_allowed_list:/ { print $2 }' /proc/self/status)"
[[ -n "$ALLOWED_CPU_LIST" ]] || die "cannot determine allowed CPUs"
mapfile -t ALLOWED_CPUS < <(expand_cpu_list "$ALLOWED_CPU_LIST")
if [[ -z "$CORE" ]]; then
  declare -A CPU_TOTAL_BEFORE=()
  declare -A CPU_IDLE_BEFORE=()
  while read -r cpu total idle; do
    CPU_TOTAL_BEFORE["$cpu"]="$total"
    CPU_IDLE_BEFORE["$cpu"]="$idle"
  done < <(read_cpu_stats)
  sleep 1
  best_busy=""
  while read -r cpu total idle; do
    allowed=0
    for candidate in "${ALLOWED_CPUS[@]}"; do
      if [[ "$candidate" == "$cpu" ]]; then
        allowed=1
        break
      fi
    done
    ((allowed == 1)) || continue
    before_total="${CPU_TOTAL_BEFORE[$cpu]:-}"
    before_idle="${CPU_IDLE_BEFORE[$cpu]:-}"
    [[ -n "$before_total" && -n "$before_idle" ]] || continue
    busy=$(((total - before_total) - (idle - before_idle)))
    if [[ -z "$best_busy" || "$busy" -lt "$best_busy" || ("$busy" -eq "$best_busy" && "$cpu" -lt "$CORE") ]]; then
      best_busy="$busy"
      CORE="$cpu"
    fi
  done < <(read_cpu_stats)
  [[ -n "$CORE" ]] || die "cannot select an allowed CPU"
  CORE_SELECTION="least_busy_1s_probe"
else
  require_uint "core" "$CORE"
  CORE_SELECTION="explicit"
fi
taskset -c "$CORE" true >/dev/null 2>&1 || die "CPU $CORE is not online and allowed (allowed: $ALLOWED_CPU_LIST)"

declare -A C_ITERS=()
printf 'fixture\tattempt\titers\tns_op\ttimed_ns\ttarget_ns\n' > "$OUT_DIR/calibration/c_iterations.tsv"
for fixture in "${FIXTURE_IDS[@]}"; do
  iters=1
  calibrated=0
  for attempt in 1 2 3 4 5 6 7 8; do
    calibration_raw="$OUT_DIR/calibration/${fixture}_attempt_${attempt}.txt"
    taskset -c "$CORE" "$C_BIN" 500 "$iters" 1 "$OUT_DIR/fixtures/$fixture.go" > "$calibration_raw" 2>&1
    ns_op="$(awk -F= '$1 == "pure_c_full_ns_op" { print $2; exit }' "$calibration_raw")"
    [[ "$ns_op" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "$fixture C calibration did not report ns/op"
    timed_ns="$(awk -v ns="$ns_op" -v n="$iters" 'BEGIN { printf "%.0f\n", ns * n }')"
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$fixture" "$attempt" "$iters" "$ns_op" "$timed_ns" "$BENCHTIME_NS" >> "$OUT_DIR/calibration/c_iterations.tsv"
    if awk -v got="$timed_ns" -v want="$BENCHTIME_NS" 'BEGIN { exit !(got >= want) }'; then
      calibrated=1
      break
    fi
    next_iters="$(awk -v ns="$ns_op" -v want="$BENCHTIME_NS" 'BEGIN { n = int((want * 1.10) / ns) + 1; if (n < 2) n = 2; print n }')"
    if ((next_iters <= iters)); then
      next_iters=$((iters * 2))
    fi
    iters="$next_iters"
  done
  ((calibrated == 1)) || die "$fixture C calibration did not reach $BENCHTIME in 8 attempts"
  C_ITERS["$fixture"]="$iters"
done

read_host_telemetry() {
  local load1 io10 mem_available_kb
  load1="$(awk '{print $1}' /proc/loadavg)"
  if [[ -r /proc/pressure/io ]]; then
    io10="$(awk '/^some / { for (i = 1; i <= NF; i++) if ($i ~ /^avg10=/) { split($i, a, "="); print a[2] } }' /proc/pressure/io)"
  else
    io10="unavailable"
  fi
  mem_available_kb="$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)"
  printf '%s\t%s\t%s\n' "$load1" "$io10" "$mem_available_kb"
}

MEM_TOTAL_KB="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
QUIET_MIN_MEM_KB=$((MEM_TOTAL_KB / 4))
if ((QUIET_MIN_MEM_KB < 8388608)); then
  QUIET_MIN_MEM_KB=8388608
fi
printf 'timestamp_utc\tattempt\tload1\tio_avg10\tmem_available_kb\tresult\tconsecutive\n' > "$OUT_DIR/quiet_admission.tsv"
if ((SKIP_QUIET_ADMISSION == 0)); then
  consecutive=0
  for ((attempt = 1; attempt <= QUIET_MAX_ATTEMPTS; attempt++)); do
    IFS=$'\t' read -r load1 io10 mem_available_kb < <(read_host_telemetry)
    result="RETRY"
    if [[ "$io10" != "unavailable" ]] && awk \
      -v load_value="$load1" -v max_load="$QUIET_MAX_LOAD1" \
      -v io="$io10" -v max_io="$QUIET_MAX_IO_AVG10" \
      -v mem="$mem_available_kb" -v min_mem="$QUIET_MIN_MEM_KB" \
      'BEGIN { exit !(load_value < max_load && io <= max_io && mem >= min_mem) }'; then
      result="PASS"
      consecutive=$((consecutive + 1))
    else
      consecutive=0
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$(date -u +%FT%TZ)" "$attempt" "$load1" "$io10" "$mem_available_kb" "$result" "$consecutive" \
      | tee -a "$OUT_DIR/quiet_admission.tsv"
    if ((consecutive >= QUIET_REQUIRED_CONSECUTIVE)); then
      break
    fi
    if ((attempt < QUIET_MAX_ATTEMPTS)); then
      sleep "$QUIET_INTERVAL_SECONDS"
    fi
  done
  ((consecutive >= QUIET_REQUIRED_CONSECUTIVE)) || die "quiet admission failed after $QUIET_MAX_ATTEMPTS bounded attempts"
  QUIET_ADMISSION="PASS"
else
  QUIET_ADMISSION="SKIPPED_NONPUBLICATION_DIAGNOSTIC"
  printf '%s\t0\tNA\tNA\tNA\t%s\t0\n' "$(date -u +%FT%TZ)" "$QUIET_ADMISSION" >> "$OUT_DIR/quiet_admission.tsv"
fi

printf 'timestamp_utc\tphase\tfixture\tbackend\tsample\tload1\tio_avg10\tmem_available_kb\n' > "$OUT_DIR/telemetry.tsv"
printf 'sequence\tfixture\tcycle\tposition\tbackend\tsample\traw_file\ttime_file\n' > "$OUT_DIR/order.tsv"
sequence=0

record_telemetry() {
  local phase="$1" fixture="$2" backend="$3" sample="$4"
  local load1 io10 mem_available_kb
  IFS=$'\t' read -r load1 io10 mem_available_kb < <(read_host_telemetry)
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$(date -u +%FT%TZ)" "$phase" "$fixture" "$backend" "$sample" "$load1" "$io10" "$mem_available_kb" \
    >> "$OUT_DIR/telemetry.tsv"
}

run_go_sample() {
  local fixture="$1" cycle="$2" position="$3" sample="$4"
  local raw="$OUT_DIR/raw/${fixture}_go_${sample}.txt"
  local time_file="$OUT_DIR/time/${fixture}_go_${sample}.txt"
  sequence=$((sequence + 1))
  printf '%s\t%s\t%s\t%s\tgo\t%s\t%s\t%s\n' "$sequence" "$fixture" "$cycle" "$position" "$sample" "$raw" "$time_file" >> "$OUT_DIR/order.tsv"
  record_telemetry before "$fixture" go "$sample"
  /usr/bin/time -v -o "$time_file" \
    taskset -c "$CORE" env GOMAXPROCS=1 "$GO_BIN" \
      -test.run '^$' \
      -test.bench "^${GO_BENCHMARK}/${fixture}$" \
      -test.benchmem -test.count=1 -test.benchtime="$BENCHTIME" \
      > "$raw" 2>&1
  record_telemetry after "$fixture" go "$sample"
  grep -q "^${GO_BENCHMARK}/${fixture}" "$raw" || die "$fixture Go sample $sample did not emit the benchmark"
}

run_c_sample() {
  local fixture="$1" cycle="$2" position="$3" sample="$4"
  local raw="$OUT_DIR/raw/${fixture}_c_${sample}.txt"
  local time_file="$OUT_DIR/time/${fixture}_c_${sample}.txt"
  sequence=$((sequence + 1))
  printf '%s\t%s\t%s\t%s\tc\t%s\t%s\t%s\n' "$sequence" "$fixture" "$cycle" "$position" "$sample" "$raw" "$time_file" >> "$OUT_DIR/order.tsv"
  record_telemetry before "$fixture" c "$sample"
  /usr/bin/time -v -o "$time_file" \
    taskset -c "$CORE" "$C_BIN" 500 "${C_ITERS[$fixture]}" 1 "$OUT_DIR/fixtures/$fixture.go" \
    > "$raw" 2>&1
  record_telemetry after "$fixture" c "$sample"
  grep -q '^pure_c_full_ns_op=' "$raw" || die "$fixture C sample $sample did not emit ns/op"
}

for fixture in "${FIXTURE_IDS[@]}"; do
  go_sample=0
  c_sample=0
  for ((cycle = 1; cycle <= CYCLES; cycle++)); do
    go_sample=$((go_sample + 1)); run_go_sample "$fixture" "$cycle" 1 "$go_sample"
    c_sample=$((c_sample + 1)); run_c_sample "$fixture" "$cycle" 2 "$c_sample"
    c_sample=$((c_sample + 1)); run_c_sample "$fixture" "$cycle" 3 "$c_sample"
    go_sample=$((go_sample + 1)); run_go_sample "$fixture" "$cycle" 4 "$go_sample"
  done
done

[[ "$(sha256sum "$GO_BIN" | awk '{print $1}')" == "$GO_BIN_SHA" ]] || die "Go benchmark binary changed during sampling"
[[ "$(sha256sum "$C_BIN" | awk '{print $1}')" == "$C_BIN_SHA" ]] || die "static C benchmark binary changed during sampling"
printf '%s  %s\n' "$GO_BIN_SHA" "$GO_BIN" > "$OUT_DIR/final_inputs.sha256"
printf '%s  %s\n' "$C_BIN_SHA" "$C_BIN" >> "$OUT_DIR/final_inputs.sha256"
for fixture in "${FIXTURE_IDS[@]}"; do
  source_path="$OUT_DIR/fixtures/$fixture.go"
  [[ "$(sha256sum "$source_path" | awk '{print $1}')" == "${FIXTURE_SOURCE_SHA[$fixture]}" ]] || die "$fixture source changed during sampling"
  printf '%s  %s\n' "${FIXTURE_SOURCE_SHA[$fixture]}" "$source_path" >> "$OUT_DIR/final_inputs.sha256"
done

extract_rss_kb() {
  awk -F: '/Maximum resident set size/ { gsub(/^[[:space:]]+/, "", $2); print $2; exit }' "$1"
}

extract_go_metrics() {
  local fixture="$1" raw="$2"
  if [[ "$GO_BACKEND" != "production" ]]; then
    awk -v name="${GO_BENCHMARK}/${fixture}" '
      $1 == name || index($1, name "-") == 1 {
        for (i = 2; i < NF; i++) metric[$(i + 1)] = $i
        names[1] = "ns/op"; names[2] = "B/op"; names[3] = "allocs/op"; names[4] = "MB/s"
        names[5] = "compact_shifts/op"; names[6] = "compact_reductions/op"
        names[7] = "compact_pop_requests/op"; names[8] = "compact_pop_paths/op"
        names[9] = "compact_pop_payloads/op"; names[10] = "compact_link_additions/op"
        names[11] = "compact_leaf_constructions/op"; names[12] = "compact_parent_constructions/op"
        names[13] = "candidate_fallback/op"
        names[14] = "selected_retained_B/op"
        for (n = 1; n <= 14; n++) {
          value = metric[names[n]]
          if (value == "") value = "NA"
          printf "%s%s", value, (n == 14 ? ORS : OFS)
        }
        found = 1
        exit
      }
      END { if (!found) exit 2 }
    ' OFS='\t' "$raw"
    return
  fi
  awk -v name="${GO_BENCHMARK}/${fixture}" '
    $1 == name || index($1, name "-") == 1 {
      for (i = 2; i < NF; i++) metric[$(i + 1)] = $i
      names[1] = "ns/op"; names[2] = "B/op"; names[3] = "allocs/op"; names[4] = "MB/s"
      names[5] = "max_stacks"; names[6] = "peak_depth"; names[7] = "tokens/op"
      names[8] = "multi_iters/op"; names[9] = "multi_tokens/op"; names[10] = "nodes/op"
      names[11] = "arena_B/op"; names[12] = "normalization_runs/op"
      names[13] = "forest_fast_path"; names[14] = "constructed/final"
      for (n = 1; n <= 14; n++) {
        value = metric[names[n]]
        if (value == "") value = "NA"
        printf "%s%s", value, (n == 14 ? ORS : OFS)
      }
      found = 1
      exit
    }
    END { if (!found) exit 2 }
  ' OFS='\t' "$raw"
}

if [[ "$GO_BACKEND" != "production" ]]; then
  printf 'sequence\tfixture\tcycle\tposition\tbackend\tsample\tns_op\tB_op\tallocs_op\tMB_s\tcompact_shifts_op\tcompact_reductions_op\tcompact_pop_requests_op\tcompact_pop_paths_op\tcompact_pop_payloads_op\tcompact_link_additions_op\tcompact_leaf_constructions_op\tcompact_parent_constructions_op\tcandidate_fallback_op\tselected_retained_B_op\tmax_rss_kb\n' > "$OUT_DIR/samples.tsv"
  GO_METRIC_NA_COLUMNS=13
else
  printf 'sequence\tfixture\tcycle\tposition\tbackend\tsample\tns_op\tB_op\tallocs_op\tMB_s\tmax_stacks\tpeak_depth\ttokens_op\tmulti_iters_op\tmulti_tokens_op\tnodes_op\tarena_B_op\tnormalization_runs_op\tforest_fast_path\tconstructed_final\tmax_rss_kb\n' > "$OUT_DIR/samples.tsv"
  GO_METRIC_NA_COLUMNS=13
fi
while IFS=$'\t' read -r seq fixture cycle position backend sample raw time_file; do
  [[ "$seq" == "sequence" ]] && continue
  rss_kb="$(extract_rss_kb "$time_file")"
  [[ "$rss_kb" =~ ^[0-9]+$ ]] || die "cannot read max RSS from $time_file"
  if [[ "$backend" == "go" ]]; then
    metrics="$(extract_go_metrics "$fixture" "$raw")" || die "cannot parse Go metrics from $raw"
  else
    ns_op="$(awk -F= '$1 == "pure_c_full_ns_op" { print $2; exit }' "$raw")"
    [[ "$ns_op" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "cannot parse C ns/op from $raw"
    metrics="$ns_op"
    for ((column = 1; column <= GO_METRIC_NA_COLUMNS; column++)); do metrics+=$'\tNA'; done
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$seq" "$fixture" "$cycle" "$position" "$backend" "$sample" "$metrics" "$rss_kb" >> "$OUT_DIR/samples.tsv"
done < "$OUT_DIR/order.tsv"

expected_samples=$((4 * CYCLES * 4))
actual_samples=$(($(wc -l < "$OUT_DIR/samples.tsv") - 1))
[[ "$actual_samples" == "$expected_samples" ]] || die "sample rows=$actual_samples want=$expected_samples"

verify_go_workload_floor() {
  local fixture="$1"
  local min_max_stacks="${FIXTURE_MIN_MAX_STACKS[$fixture]}"
  local min_multi_iters="${FIXTURE_MIN_MULTI_ITERS[$fixture]}"
  local min_multi_tokens="${FIXTURE_MIN_MULTI_TOKENS[$fixture]}"
  awk -F'\t' \
    -v fixture="$fixture" \
    -v min_max_stacks="$min_max_stacks" \
    -v min_multi_iters="$min_multi_iters" \
    -v min_multi_tokens="$min_multi_tokens" '
      $2 == fixture && $5 == "go" {
        seen++
        if ($11 == "NA" || $11 + 0 < min_max_stacks ||
            $14 == "NA" || $14 + 0 < min_multi_iters ||
            $15 == "NA" || $15 + 0 < min_multi_tokens) {
          printf "fixture %s sample %s workload identity max_stacks=%s multi_iters=%s multi_tokens=%s want >=%s/>=%s/>=%s\n", fixture, $6, $11, $14, $15, min_max_stacks, min_multi_iters, min_multi_tokens > "/dev/stderr"
          failed = 1
        }
      }
      END { exit !(seen > 0 && !failed) }
    ' "$OUT_DIR/samples.tsv"
}

verify_candidate_work() {
  local fixture="$1"
  awk -F'\t' \
    -v fixture="$fixture" \
    -v shifts="${CANDIDATE_EXPECTED_SHIFTS[$fixture]}" \
    -v reductions="${CANDIDATE_EXPECTED_REDUCTIONS[$fixture]}" \
    -v pop_requests="${CANDIDATE_EXPECTED_POP_REQUESTS[$fixture]}" \
    -v pop_paths="${CANDIDATE_EXPECTED_POP_PATHS[$fixture]}" \
    -v pop_payloads="${CANDIDATE_EXPECTED_POP_PAYLOADS[$fixture]}" \
    -v link_additions="${CANDIDATE_EXPECTED_LINK_ADDITIONS[$fixture]}" \
    -v leaf_constructions="${CANDIDATE_EXPECTED_LEAF_CONSTRUCTIONS[$fixture]}" \
    -v parent_constructions="${CANDIDATE_EXPECTED_PARENT_CONSTRUCTIONS[$fixture]}" '
      $2 == fixture && $5 == "go" {
        seen++
        if ($11 == "NA" || $11 + 0 != shifts ||
            $12 == "NA" || $12 + 0 != reductions ||
            $13 == "NA" || $13 + 0 != pop_requests ||
            $14 == "NA" || $14 + 0 != pop_paths ||
            $15 == "NA" || $15 + 0 != pop_payloads ||
            $16 == "NA" || $16 + 0 != link_additions ||
            $17 == "NA" || $17 + 0 != leaf_constructions ||
            $18 == "NA" || $18 + 0 != parent_constructions ||
            $19 == "NA" || $19 + 0 != 0) {
          printf "fixture %s sample %s compact work=%s/%s/%s/%s/%s/%s/%s/%s fallback=%s want=%s/%s/%s/%s/%s/%s/%s/%s fallback=0\n", fixture, $6, $11, $12, $13, $14, $15, $16, $17, $18, $19, shifts, reductions, pop_requests, pop_paths, pop_payloads, link_additions, leaf_constructions, parent_constructions > "/dev/stderr"
          failed = 1
        }
      }
      END { exit !(seen > 0 && !failed) }
    ' "$OUT_DIR/samples.tsv"
}

for fixture in "${FIXTURE_IDS[@]}"; do
  if [[ "$GO_BACKEND" != "production" ]]; then
    verify_candidate_work "$fixture" || die "$fixture sampled compact work identity failed"
  else
    verify_go_workload_floor "$fixture" || die "$fixture sampled workload identity failed"
  fi
done

median_column() {
  local fixture="$1" backend="$2" column="$3"
  awk -F'\t' -v fixture="$fixture" -v backend="$backend" -v column="$column" \
    '$2 == fixture && $5 == backend && $column != "NA" { print $column }' "$OUT_DIR/samples.tsv" \
    | LC_ALL=C sort -n \
    | awk '{ value[NR] = $1 } END { if (NR == 0) { print "NA"; exit }; if (NR % 2) print value[(NR + 1) / 2]; else printf "%.6f\n", (value[NR / 2] + value[NR / 2 + 1]) / 2 }'
}

max_column() {
  local fixture="$1" backend="$2" column="$3"
  awk -F'\t' -v fixture="$fixture" -v backend="$backend" -v column="$column" \
    '$2 == fixture && $5 == backend && $column != "NA" { if (!seen || $column > max) max = $column; seen = 1 } END { if (!seen) exit 2; print max }' \
    "$OUT_DIR/samples.tsv"
}

if [[ "$GO_BACKEND" != "production" ]]; then
  printf 'fixture\tgo_median_ns_op\tc_median_ns_op\tgo_c_ratio\tgo_median_B_op\tgo_median_allocs_op\tgo_median_MB_s\tgo_median_compact_shifts_op\tgo_median_compact_reductions_op\tgo_median_compact_pop_requests_op\tgo_median_compact_pop_paths_op\tgo_median_compact_pop_payloads_op\tgo_median_compact_link_additions_op\tgo_median_compact_leaf_constructions_op\tgo_median_compact_parent_constructions_op\tgo_median_candidate_fallback_op\tgo_median_selected_retained_B_op\tc_fixed_iters\tgo_max_rss_kb\tc_max_rss_kb\n' > "$OUT_DIR/report.tsv"
else
  printf 'fixture\tgo_median_ns_op\tc_median_ns_op\tgo_c_ratio\tgo_median_B_op\tgo_median_allocs_op\tgo_median_MB_s\tgo_median_max_stacks\tgo_median_peak_depth\tgo_median_tokens_op\tgo_median_multi_iters_op\tgo_median_multi_tokens_op\tgo_median_nodes_op\tgo_median_arena_B_op\tgo_median_normalization_runs_op\tgo_median_forest_fast_path\tgo_median_constructed_final\tc_fixed_iters\tgo_max_rss_kb\tc_max_rss_kb\n' > "$OUT_DIR/report.tsv"
fi
ratio_file="$OUT_DIR/calibration/fixture_ratios.txt"
: > "$ratio_file"
go_sum=0
c_sum=0
MAX_FIXTURE_GO_C_RATIO=""
MAX_RATIO_FIXTURE=""
for fixture in "${FIXTURE_IDS[@]}"; do
  go_ns="$(median_column "$fixture" go 7)"
  c_ns="$(median_column "$fixture" c 7)"
  ratio="$(awk -v go="$go_ns" -v c="$c_ns" 'BEGIN { printf "%.6f\n", go / c }')"
  printf '%s\n' "$ratio" >> "$ratio_file"
  go_sum="$(awk -v sum="$go_sum" -v value="$go_ns" 'BEGIN { printf "%.6f\n", sum + value }')"
  c_sum="$(awk -v sum="$c_sum" -v value="$c_ns" 'BEGIN { printf "%.6f\n", sum + value }')"
  printf '%s\t%s\t%s\t%s' "$fixture" "$go_ns" "$c_ns" "$ratio" >> "$OUT_DIR/report.tsv"
  if [[ "$GO_BACKEND" != "production" ]]; then
    for column in 8 9 10 11 12 13 14 15 16 17 18 19 20; do
      printf '\t%s' "$(median_column "$fixture" go "$column")" >> "$OUT_DIR/report.tsv"
    done
    printf '\t%s\t%s\t%s\n' "${C_ITERS[$fixture]}" "$(max_column "$fixture" go 21)" "$(max_column "$fixture" c 21)" >> "$OUT_DIR/report.tsv"
    if [[ -z "$MAX_FIXTURE_GO_C_RATIO" ]] || awk -v candidate="$ratio" -v current="$MAX_FIXTURE_GO_C_RATIO" 'BEGIN { exit !(candidate > current) }'; then
      MAX_FIXTURE_GO_C_RATIO="$ratio"
      MAX_RATIO_FIXTURE="$fixture"
    fi
  else
    for column in 8 9 10 11 12 13 14 15 16 17 18 19 20; do
      printf '\t%s' "$(median_column "$fixture" go "$column")" >> "$OUT_DIR/report.tsv"
    done
    printf '\t%s\t%s\t%s\n' "${C_ITERS[$fixture]}" "$(max_column "$fixture" go 21)" "$(max_column "$fixture" c 21)" >> "$OUT_DIR/report.tsv"
  fi
done
EQUAL_FIXTURE_GEOMEAN_RATIO="$(awk '{ sum += log($1); n++ } END { if (!n) exit 2; printf "%.6f\n", exp(sum / n) }' "$ratio_file")"
SUITE_SUM_RATIO="$(awk -v go="$go_sum" -v c="$c_sum" 'BEGIN { printf "%.6f\n", go / c }')"
LARGEST_FIXTURE="grammargen_lr"
if [[ "$GO_BACKEND" != "production" ]]; then
  LARGEST_GO_MAX_RSS_KB="$(max_column "$LARGEST_FIXTURE" go 21)"
  LARGEST_C_MAX_RSS_KB="$(max_column "$LARGEST_FIXTURE" c 21)"
else
  LARGEST_GO_MAX_RSS_KB="$(max_column "$LARGEST_FIXTURE" go 21)"
  LARGEST_C_MAX_RSS_KB="$(max_column "$LARGEST_FIXTURE" c 21)"
fi

printf 'metric\tvalue\n' > "$OUT_DIR/summary.tsv"
printf 'equal_fixture_geomean_go_c_ratio\t%s\n' "$EQUAL_FIXTURE_GEOMEAN_RATIO" >> "$OUT_DIR/summary.tsv"
printf 'fixed_suite_go_sum_of_medians_ns\t%s\n' "$go_sum" >> "$OUT_DIR/summary.tsv"
printf 'fixed_suite_c_sum_of_medians_ns\t%s\n' "$c_sum" >> "$OUT_DIR/summary.tsv"
printf 'fixed_suite_sum_of_medians_go_c_ratio\t%s\n' "$SUITE_SUM_RATIO" >> "$OUT_DIR/summary.tsv"
if [[ "$GO_BACKEND" != "production" ]]; then
  printf 'max_fixture_go_c_ratio\t%s\n' "$MAX_FIXTURE_GO_C_RATIO" >> "$OUT_DIR/summary.tsv"
  printf 'max_ratio_fixture\t%s\n' "$MAX_RATIO_FIXTURE" >> "$OUT_DIR/summary.tsv"
fi
printf 'largest_fixture\t%s\n' "$LARGEST_FIXTURE" >> "$OUT_DIR/summary.tsv"
printf 'largest_fixture_go_max_rss_kb\t%s\n' "$LARGEST_GO_MAX_RSS_KB" >> "$OUT_DIR/summary.tsv"
printf 'largest_fixture_c_max_rss_kb\t%s\n' "$LARGEST_C_MAX_RSS_KB" >> "$OUT_DIR/summary.tsv"

git -C "$REPO_ROOT" status --porcelain=v1 --untracked-files=all > "$OUT_DIR/final_git_status.txt"
if [[ "$MODE" == "publication" && -s "$OUT_DIR/final_git_status.txt" ]]; then
  die "benchmark worktree changed during the publication run"
fi

FIRST_PREFLIGHT="$OUT_DIR/preflight/static_${FIXTURE_IDS[0]}.txt"
DRIVER_SHA="$(sha256sum "$SCRIPT_DIR/run_canonical_go_full_parse.sh" | awk '{print $1}')"
REPORT_SHA="$(sha256sum "$OUT_DIR/report.tsv" | awk '{print $1}')"
SUMMARY_SHA="$(sha256sum "$OUT_DIR/summary.tsv" | awk '{print $1}')"
FIXTURE_MANIFEST_SHA="$(sha256sum "$OUT_DIR/fixture_manifest.tsv" | awk '{print $1}')"
SAMPLES_SHA="$(sha256sum "$OUT_DIR/samples.tsv" | awk '{print $1}')"
if [[ "$GO_BACKEND" != "production" ]]; then
  RUNNER_SOURCE_SHA="$(sha256sum "$REPO_ROOT/parsercore_phase0_fresh_full_runner.go" | awk '{print $1}')"
  BENCHMARK_SOURCE_SHA="$(sha256sum "$REPO_ROOT/parsercore_phase0_fresh_full_runner_internal_test.go" | awk '{print $1}')"
fi
{
  if [[ "$GO_BACKEND" != "production" ]]; then
    printf 'receipt_version=canonical-go-full-parse-v2\n'
  else
    printf 'receipt_version=canonical-go-full-parse-v1\n'
  fi
  printf 'receipt_class=%s\n' "$RECEIPT_CLASS"
  printf 'go_backend=%s\n' "$GO_BACKEND"
  printf 'go_build_tags=%s\n' "$GO_BUILD_TAGS"
  printf 'go_benchmark=%s\n' "$GO_BENCHMARK"
  printf 'go_preflight=%s\n' "$GO_PREFLIGHT"
  printf 'go_measured_lifecycle=%s\n' "$GO_LIFECYCLE"
  printf 'go_fallback_policy=%s\n' "$GO_FALLBACK"
  if [[ "$GO_BACKEND" != "production" ]]; then
    printf 'candidate_runner_source_sha256=%s\n' "$RUNNER_SOURCE_SHA"
    printf 'candidate_benchmark_source_sha256=%s\n' "$BENCHMARK_SOURCE_SHA"
    printf 'max_fixture_go_c_ratio=%s\n' "$MAX_FIXTURE_GO_C_RATIO"
    printf 'max_ratio_fixture=%s\n' "$MAX_RATIO_FIXTURE"
  fi
  printf 'timestamp_utc=%s\n' "$(date -u +%FT%TZ)"
  printf 'git_head=%s\n' "$GIT_HEAD"
  printf 'git_initial_clean=%s\n' "$([[ -z "$GIT_STATUS" ]] && echo true || echo false)"
  printf 'git_final_clean=%s\n' "$([[ ! -s "$OUT_DIR/final_git_status.txt" ]] && echo true || echo false)"
  printf 'driver_sha256=%s\n' "$DRIVER_SHA"
  printf 'go_version=%s\n' "$(go version)"
  printf 'go_binary_sha256=%s\n' "$GO_BIN_SHA"
  printf 'receipt_c_artifact_path=%s\n' "$C_BIN"
  printf 'receipt_c_artifact_sha256=%s\n' "$C_BIN_SHA"
  printf 'core=%s\n' "$CORE"
  printf 'core_selection=%s\n' "$CORE_SELECTION"
  printf 'allowed_cpu_list=%s\n' "$ALLOWED_CPU_LIST"
  printf 'gomaxprocs=1\n'
  printf 'cycles=%s\n' "$CYCLES"
  printf 'sample_order=Go-C-C-Go\n'
  printf 'samples_per_backend_fixture=%s\n' "$((CYCLES * 2))"
  printf 'benchtime=%s\n' "$BENCHTIME"
  printf 'c_calibration_min_ns=%s\n' "$BENCHTIME_NS"
  printf 'cgo_static_deep_admission=%s\n' "$CGO_ADMISSION"
  printf 'cgo_admission_contract=root_and_cgo_same_backend_tag+treesitter_c_parity_canonical_static_deep\n'
  if [[ "$GO_BACKEND" != "production" ]]; then
    printf 'workload_identity=compact_deep_tree_and_exact_work_admission_v1\n'
  else
    printf 'workload_identity=runtime_and_node_kind_admission_v1\n'
  fi
  printf 'fixture_required_node_kinds=%s\n' "$FIXTURE_REQUIRED_NODE_KINDS"
  printf 'suite_required_node_kinds=%s\n' "$SUITE_REQUIRED_NODE_KINDS"
  printf 'suite_required_any_node_kinds=%s\n' "$SUITE_REQUIRED_ANY_NODE_KINDS"
  printf 'quiet_admission=%s\n' "$QUIET_ADMISSION"
  printf 'quiet_max_load1=%s\n' "$QUIET_MAX_LOAD1"
  printf 'quiet_max_io_avg10=%s\n' "$QUIET_MAX_IO_AVG10"
  printf 'quiet_min_mem_available_kb=%s\n' "$QUIET_MIN_MEM_KB"
  printf 'got_env_count=%s\n' "$(printf '%s\n' "$GOT_ENV" | awk 'NF { n++ } END { print n + 0 }')"
  printf 'inherited_go_override_count=%s\n' "$(printf '%s\n' "$GO_OVERRIDE_ENV" | awk 'NF { n++ } END { print n + 0 }')"
  printf 'inherited_go_overrides_sha256=%s\n' "$(sha256sum "$OUT_DIR/preflight/inherited_go_overrides.txt" | awk '{print $1}')"
  printf 'fixture_manifest_sha256=%s\n' "$FIXTURE_MANIFEST_SHA"
  printf 'samples_sha256=%s\n' "$SAMPLES_SHA"
  printf 'report_sha256=%s\n' "$REPORT_SHA"
  printf 'summary_sha256=%s\n' "$SUMMARY_SHA"
  printf 'equal_fixture_geomean_go_c_ratio=%s\n' "$EQUAL_FIXTURE_GEOMEAN_RATIO"
  printf 'fixed_suite_sum_of_medians_go_c_ratio=%s\n' "$SUITE_SUM_RATIO"
  printf 'largest_fixture_go_max_rss_kb=%s\n' "$LARGEST_GO_MAX_RSS_KB"
  printf 'largest_fixture_c_max_rss_kb=%s\n' "$LARGEST_C_MAX_RSS_KB"
  grep '^oracle_' "$FIRST_PREFLIGHT" | grep -v '^oracle_artifact_path='
} > "$OUT_DIR/manifest.txt"
MANIFEST_SHA="$(sha256sum "$OUT_DIR/manifest.txt" | awk '{print $1}')"

{
  printf 'hostname=%s\n' "$(hostname)"
  printf 'uname=%s\n' "$(uname -a)"
  printf 'go_version=%s\n' "$(go version)"
  printf 'core=%s\n' "$CORE"
  printf 'core_selection=%s\n' "$CORE_SELECTION"
  printf 'allowed_cpu_list=%s\n' "$ALLOWED_CPU_LIST"
  lscpu
  for path in \
    "/sys/devices/system/cpu/cpu$CORE/cpufreq/scaling_governor" \
    "/sys/devices/system/cpu/cpu$CORE/cpufreq/scaling_cur_freq"; do
    if [[ -r "$path" ]]; then
      printf '%s=%s\n' "$path" "$(<"$path")"
    fi
  done
} > "$OUT_DIR/environment.txt"

if [[ -n "$ORACLE_TEMP_ROOT" ]]; then
  rm -rf -- "$ORACLE_TEMP_ROOT"
  ORACLE_TEMP_ROOT=""
fi

tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
  --exclude='./receipt.tar' --exclude='./receipt.tar.sha256' --exclude='./run.status' \
  -C "$OUT_DIR" -cf "$OUT_DIR/receipt.tar" .
BUNDLE_SHA="$(sha256sum "$OUT_DIR/receipt.tar" | awk '{print $1}')"
printf '%s  receipt.tar\n' "$BUNDLE_SHA" > "$OUT_DIR/receipt.tar.sha256"
printf 'receipt_status=COMPLETE\nreceipt_class=%s\n' "$RECEIPT_CLASS" > "$OUT_DIR/run.status"
RUN_COMPLETE=1
trap - EXIT

echo "receipt_complete=true"
echo "receipt_class=$RECEIPT_CLASS"
echo "manifest_sha256=$MANIFEST_SHA"
echo "report_sha256=$REPORT_SHA"
echo "bundle_sha256=$BUNDLE_SHA"
echo "output_dir=$OUT_DIR"
