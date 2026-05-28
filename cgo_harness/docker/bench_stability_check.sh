#!/usr/bin/env bash
# bench_stability_check.sh — verify the host environment is in good shape
# for tight bench variance before running the ring matrix.
#
# Checks (informational; non-fatal unless --strict):
#   - CPU governor on the pinned CPU is "performance" (or close to)
#   - Turbo Boost / intel_pstate boost not actively varying frequency
#   - Pinned CPU is not currently busy (no other processes there)
#   - Docker is responsive
#   - No high system load (≥ N CPUs worth of pending work)
#
# Usage:
#   ./bench_stability_check.sh [--cpu N] [--strict]
#
# Exit codes:
#   0 — all checks passed (or non-strict and all warnings only)
#   1 — strict mode and at least one check failed

set -uo pipefail

CPU="${BENCH_PIN_CPU:-18}"
STRICT=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cpu) CPU="$2"; shift 2 ;;
    --strict) STRICT=1; shift ;;
    -h|--help)
      sed -n '2,18p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
done

PASS=0
WARN=0
FAIL=0

ok() { printf "  \033[32m✓\033[0m %s\n" "$1"; PASS=$((PASS+1)); }
warn() { printf "  \033[33m!\033[0m %s\n" "$1"; WARN=$((WARN+1)); }
fail() { printf "  \033[31m✗\033[0m %s\n" "$1"; FAIL=$((FAIL+1)); }

echo "Bench stability check (pinned CPU: $CPU)"
echo

# --- CPU governor check ---
GOV_FILE="/sys/devices/system/cpu/cpu${CPU}/cpufreq/scaling_governor"
if [[ -r "$GOV_FILE" ]]; then
  GOV="$(cat "$GOV_FILE" 2>/dev/null || echo unknown)"
  case "$GOV" in
    performance) ok "CPU $CPU governor = performance" ;;
    powersave) warn "CPU $CPU governor = powersave — frequency may vary; set governor to performance for tighter benches" ;;
    *) warn "CPU $CPU governor = $GOV — performance mode preferred" ;;
  esac
else
  warn "cannot read CPU governor at $GOV_FILE (probably WSL2 or restricted)"
fi

# --- Turbo / boost check ---
BOOST_FILE="/sys/devices/system/cpu/cpufreq/boost"
if [[ -r "$BOOST_FILE" ]]; then
  BOOST="$(cat "$BOOST_FILE")"
  if [[ "$BOOST" == "0" ]]; then
    ok "Turbo boost disabled (stable frequency)"
  else
    warn "Turbo boost enabled — frequency varies under thermal pressure"
  fi
else
  INTEL_NTB="/sys/devices/system/cpu/intel_pstate/no_turbo"
  if [[ -r "$INTEL_NTB" ]]; then
    NTB="$(cat "$INTEL_NTB")"
    if [[ "$NTB" == "1" ]]; then
      ok "intel_pstate Turbo disabled (stable frequency)"
    else
      warn "intel_pstate Turbo enabled — frequency varies under thermal pressure"
    fi
  else
    warn "Cannot read Turbo/boost setting (probably WSL2 — disable Turbo in BIOS for tightest variance)"
  fi
fi

# --- Current CPU activity ---
if command -v mpstat >/dev/null 2>&1; then
  CPU_USAGE="$(mpstat -P "$CPU" 1 1 2>/dev/null | awk -v cpu="$CPU" '$3==cpu {print 100-$NF}' | tail -1)"
  if [[ -n "$CPU_USAGE" ]]; then
    if awk -v u="$CPU_USAGE" 'BEGIN { exit !(u < 5) }'; then
      ok "CPU $CPU idle (${CPU_USAGE}% used)"
    else
      warn "CPU $CPU busy (${CPU_USAGE}% used) — kill background processes for cleaner benches"
    fi
  fi
else
  warn "mpstat not available — install sysstat package for CPU activity check"
fi

# --- System load ---
if [[ -r /proc/loadavg ]]; then
  LOAD="$(awk '{print $1}' /proc/loadavg)"
  NCPU="$(nproc 2>/dev/null || echo 1)"
  if awk -v l="$LOAD" -v n="$NCPU" 'BEGIN { exit !(l < n/4) }'; then
    ok "Load avg low ($LOAD with $NCPU CPUs)"
  else
    warn "Load avg high ($LOAD with $NCPU CPUs) — concurrent work will perturb benches"
  fi
fi

# --- Docker reachable ---
if command -v docker >/dev/null 2>&1; then
  if docker info >/dev/null 2>&1; then
    ok "Docker daemon responsive"
  else
    fail "Docker daemon not responding"
  fi
fi

# --- taskset available ---
if command -v taskset >/dev/null 2>&1; then
  ok "taskset available (for host-side pinned benches)"
else
  warn "taskset missing — needed for ab_pinned.sh"
fi

echo
echo "Summary: $PASS pass, $WARN warn, $FAIL fail"

if [[ "$STRICT" == "1" ]]; then
  if [[ "$FAIL" -gt 0 || "$WARN" -gt 0 ]]; then
    exit 1
  fi
fi
exit 0
