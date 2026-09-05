#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
selection='^Test(FullParseNodeCapacity.*|GoFullParseRepeatedParserPreservesLiveTree|Ensure.*NodeCapacity.*|AllocNodeUsesOverflowSlabsWhenPrimaryExhausted|Arena.*|NodeRetentionCapRespectsByteLimit|ChildSlabStalePointersAfterReset|ShouldNotRetryMemoryBudgetParse|EvictionGuardPreventsOversizedArenaReuse|OversizedFullArenaReleaseClearsAllDuplicateCheckoutSlots|ParseFullArena.*|ParserScratch.*Budget.*|GoFullParseBenchmarkFixturesParseClean|GoFullParseBenchmarkSourceStaysWithinDefaultNodeBudget|ParseIncrementalReleaseKeepsBorrowedNodesAlive|ParseReuseStateRetainsTransitiveBorrowedArenaLifecycle|NodeFieldMetadataArena.*)$'
for role in baseline candidate; do
  if [[ "$role" == baseline ]]; then cd /baseline; else cd /workspace; fi
  go test -tags gts_parsercorephase0 . -run "$selection" -v -count=1 -parallel=1 -timeout=3m > "/evidence/$role-correctness.txt" 2>&1
  printf 'Completed %s correctness\n' "$role"
done
