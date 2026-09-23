#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cache_path="$repo_root/.canopy/index.json"
cache_head_path="$repo_root/.canopy/index.git-head"
canopy_bin="${CANOPY_BIN:-$HOME/.local/bin/canopy}"

if [[ ! -x "$canopy_bin" ]]; then
	echo "Canopy is not executable: $canopy_bin" >&2
	exit 127
fi

for argument in "$@"; do
	if [[ "$argument" == "--no-cache" ]]; then
		exec "$canopy_bin" "$@"
	fi
done

cache_stale=false
stale_reason=""

if [[ ! -f "$cache_path" ]]; then
	cache_stale=true
	stale_reason="the cache does not exist"
else
	validate_status=0
	"$canopy_bin" index validate "$repo_root" \
		--cache "$cache_path" >/dev/null 2>&1 || validate_status=$?
	case "$validate_status" in
	0)
		;;
	2)
		cache_stale=true
		stale_reason="indexed files changed or disappeared"
		;;
	*)
		echo "Canopy cache validation failed with status $validate_status." >&2
		exit "$validate_status"
		;;
	esac
fi

if [[ "$cache_stale" == false ]]; then
	cache_head="$(tr -d '[:space:]' <"$cache_head_path" 2>/dev/null || true)"
	current_head="$(git -C "$repo_root" rev-parse HEAD 2>/dev/null || true)"
	if [[ -n "$current_head" && "$cache_head" != "$current_head" ]]; then
		cache_stale=true
		stale_reason="the cache belongs to a different commit"
	fi
fi

if [[ "$cache_stale" == false ]]; then
	declare -A indexed_paths=()
	declare -A indexed_extensions=()

	while IFS= read -r path; do
		indexed_paths["$path"]=1
		filename="${path##*/}"
		if [[ "$filename" == *.* ]]; then
			extension=".${filename##*.}"
			indexed_extensions["${extension,,}"]=1
		fi
	done < <(jq -r '.files[].path' "$cache_path")

	while IFS= read -r -d '' status_entry; do
		path="${status_entry:3}"
		filename="${path##*/}"
		if [[ "$filename" != *.* ]]; then
			continue
		fi
		extension=".${filename##*.}"
		extension="${extension,,}"
		if [[ -z "${indexed_extensions[$extension]:-}" ]]; then
			continue
		fi
		if [[ -n "${indexed_paths[$path]:-}" ]]; then
			continue
		fi
		if git -C "$repo_root" \
			-c "core.excludesFile=$repo_root/.canopyignore" \
			check-ignore --quiet --no-index -- "$path" 2>/dev/null; then
			continue
		fi
		cache_stale=true
		stale_reason="a new source file is not indexed"
		break
	done < <(git -C "$repo_root" status --porcelain=v1 -z --untracked-files=all)
fi

if [[ "$cache_stale" == true ]]; then
	echo "Canopy cache is stale: $stale_reason. Running a fresh scoped query." >&2
	exec "$canopy_bin" "$@" --no-cache
fi

exec "$canopy_bin" "$@"
