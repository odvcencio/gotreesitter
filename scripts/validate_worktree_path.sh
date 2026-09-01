#!/usr/bin/env bash
set -euo pipefail

usage() {
	printf 'Usage: %s PATH\n' "${0##*/}" >&2
}

reject() {
	local reason=$1
	local path=${2:-$candidate}
	printf 'invalid worktree path (%s): %q\n' "$reason" "$path" >&2
	exit 2
}

if (($# != 1)); then
	usage
	exit 2
fi

candidate=$1
if [[ -z "$candidate" ]]; then
	reject 'the path is empty'
fi

case "$candidate" in
*$'\n'*|*$'\r'*)
	reject 'the path contains a line break'
	;;
esac

if [[ ! -d "$candidate" ]]; then
	reject 'the directory does not exist'
fi

resolved=""
if ! IFS= read -r -d '' resolved < <(
	cd -P -- "$candidate" || exit
	printf '%s\0' "$PWD"
); then
	reject 'the path cannot be resolved'
fi
case "$resolved" in
*$'\n'*|*$'\r'*)
	reject 'the canonical path contains a line break' "$resolved"
	;;
esac

git_root=$(git -C "$resolved" rev-parse --show-toplevel 2>/dev/null || true)
if [[ -z "$git_root" ]]; then
	case "${resolved##*/}" in
	'Preparing worktree (detached HEAD '*)
		reject 'the path contains worktree status text' "$resolved"
		;;
	esac
	reject 'the path is not a Git worktree'
fi

canonical_git_root=""
if ! IFS= read -r -d '' canonical_git_root < <(
	cd -P -- "$git_root" || exit
	printf '%s\0' "$PWD"
); then
	reject 'the Git root cannot be resolved'
fi
git_root=$canonical_git_root
case "$git_root" in
*$'\n'*|*$'\r'*)
	reject 'the Git root contains a line break' "$git_root"
	;;
esac
if [[ "$git_root" != "$resolved" ]]; then
	reject 'the path is not the worktree root'
fi

printf '%s\n' "$resolved"
