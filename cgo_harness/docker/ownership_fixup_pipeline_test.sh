#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/ownership-pipeline-test.XXXXXX")
trap 'rm -rf -- "$test_root"' EXIT
mkdir -p "$test_root/bin"
cat >"$test_root/bin/find" <<'MOCK'
#!/usr/bin/env bash
printf '%s\0' '/root-owned path'
if [[ "$MOCK_FIND_FAIL" == 1 ]]; then exit 71; fi
MOCK
cat >"$test_root/bin/chown" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$MOCK_CHOWN_LOG"
MOCK
chmod +x "$test_root/bin/find" "$test_root/bin/chown"
run_fixup() {
  local fail_find=$1
  PATH="$test_root/bin:$PATH" MOCK_FIND_FAIL="$fail_find" MOCK_CHOWN_LOG="$test_root/chown.log" \
    GTS_HOST_UID=1000 GTS_HOST_GID=1001 GTS_FIXUP_NEWER='' \
    bash -c '
      source "$1"
      docker() { while (($#)); do if [[ $1 == bash ]]; then "$@"; return; fi; shift; done; return 99; }
      GTS_FIXUP_PATHS=(/fixture)
      GTS_FIXUP_MOUNT_ARGS=()
      gts_fixup_ownership img 1000 1001 "$2" ""
    ' _ "$script_dir/ownership_fixup.sh" "$test_root/fixup.log"
}
run_fixup 0 || { echo 'valid ownership traversal failed' >&2; exit 1; }
printf '%s\n' '-h' '1000:1001' '/root-owned path' >"$test_root/expected"
cmp "$test_root/expected" "$test_root/chown.log"
rm "$test_root/chown.log"
if run_fixup 1; then
  echo 'failed find was incorrectly reported as success' >&2
  exit 1
fi
cmp "$test_root/expected" "$test_root/chown.log"
printf '%s\n' 'ok - ownership repair reports find failure after successful chown'
