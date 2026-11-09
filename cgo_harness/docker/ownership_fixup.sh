#!/usr/bin/env bash
# Shared ownership repair for the Docker harness scripts. Source this file.
#
# The harness containers run as root, so anything they create in a bind-mounted
# host folder belongs to root on the host. A later non-root build that writes
# into such a folder (a shared Go build cache, for example) fails with
# "permission denied". gts_fixup_ownership returns those entries to the host
# user.
#
# gts_fixup_ownership IMAGE UID GID LOG STAMP
#
#   IMAGE           image that provides bash, find, xargs and chown
#   UID, GID        host user and group
#   LOG             file that receives the repair output
#   STAMP           "" repairs every root-owned entry under the paths. A file
#                   path repairs only entries whose status changed after the
#                   file was modified: those created or chowned by the run.
#                   Use it for large shared mounts so the walk stays short.
#
# Set two arrays before the call:
#   GTS_FIXUP_MOUNT_ARGS  docker "--mount" arguments for the folders to repair
#   GTS_FIXUP_PATHS       the in-container paths to repair
#
# Returns 0 on success, 1 if the repair container failed, 3 if nothing was
# given to repair.

# gts_fixup_stamp FILE creates a stamp file dated slightly in the past, so the
# comparison is safe against coarse timestamp granularity.
gts_fixup_stamp() {
  : >"$1"
  touch -d '2 seconds ago' "$1"
}

gts_fixup_ownership() {
  local image=$1 uid=$2 gid=$3 log=$4 stamp=$5
  if [[ ${#GTS_FIXUP_PATHS[@]} -eq 0 ]]; then
    return 3
  fi
  local stamp_args=() newer=""
  if [[ -n "$stamp" ]]; then
    stamp_args=(--mount "type=bind,src=$stamp,dst=/gts-ownership-stamp,readonly")
    newer="-cnewer /gts-ownership-stamp"
  fi
  docker run --rm \
    "${GTS_FIXUP_MOUNT_ARGS[@]}" \
    "${stamp_args[@]}" \
    -e "GTS_HOST_UID=$uid" \
    -e "GTS_HOST_GID=$gid" \
    -e "GTS_FIXUP_NEWER=$newer" \
    "$image" \
    bash -c 'find "$@" -xdev \( -path "*/.git/objects" -o -path /workspace/.claude \) -prune -o -user 0 $GTS_FIXUP_NEWER -print0 | xargs -0r chown -h "$GTS_HOST_UID:$GTS_HOST_GID"' _ "${GTS_FIXUP_PATHS[@]}" \
    >"$log" 2>&1
}
