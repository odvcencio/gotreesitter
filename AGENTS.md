# GoTreeSitter contributor guidance

- Use `scripts/canopy_query.sh` for cached structural queries. It detects source changes and falls back to fresh scoped queries. Use direct `canopy ... --no-cache` for changed files and `rg` for literal text.
- Scope structural queries and set `--limit`. Do not run concurrent repository-wide Canopy searches. Refresh with `scripts/refresh_canopy_index.sh` before broad queries when the source set changed; it validates before promoting the cache.
- Never run repository-wide `go test ./...` or race sweeps on the host. Heavy correctness, parity, and race work requires Docker isolation, one language/grammar at a time. Narrow OOM investigations to one language and suite.
- Read [correctness/performance procedures](docs/agent-performance.md) before optimization, GLR/incremental changes, heavy correctness/parity/race work, benchmark comparisons, or releases. Keep correctness and performance gates separate; validate parity before performance for GLR/incremental changes.
- Preserve parser memory budgets. Crashes, OOMs, and runaway retention block releases. Release evidence must include the complete reproducible benchmark suite; directional speed targets do not block release.
- Use `scripts/run_randomized_benchmarks.sh` for every before/after comparison; fixed-order runs are not comparison evidence.
- Keep commits scoped and bisectable. Remove scratch/debug artifacts. Stage explicit paths and use `buckley commit --yes --minimal-output`.
- For prose-only edits, check the changed instructions and links. Do not start parser tests or benchmarks solely for documentation.

## Prose: ASD-STE100
All agent-written prose in this repo follows the ASD-STE100 rules
profile (decision 0011, hypha://m31labs/hyphae).

- Use the active voice and the imperative mood for instructions.
- Keep procedural sentences at or below 20 words. Keep descriptive
  sentences at or below 25 words.
- Give each word one meaning. Use it the same way through the
  document.
- Do not write noun clusters of more than three nouns. Do not drop
  articles.
- Do not use idioms, slang, or Latin abbreviations. Write "for
  example", not "e.g.".
- Define an abbreviation at first use.
- Use a vertical list for more than two items or steps.
- Use concrete verbs. Avoid "handle", "leverage", "deal with".

Scope: commit messages, PR titles and bodies, review output, and all
documentation prose that agents write. Code identifiers and quoted
tool output are out of scope.
