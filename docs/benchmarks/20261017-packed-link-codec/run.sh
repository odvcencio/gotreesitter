#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /workspace
for lang in go yaml;do
 go test . -tags gts_parsercorephase0,gts_link_storage_probe -run "^TestLinkStorageMetadataCensus$/^${lang}$" -count=1 -v -timeout=8m > /evidence/$lang-census.log 2>&1
 echo "$lang read-only link metadata census passed"
done
printf 'complete\n' > /evidence/census-complete.txt
