# Contributing to gotreesitter

gotreesitter is a pure-Go tree-sitter runtime (module
`github.com/odvcencio/gotreesitter`). It needs Go 1.22 or newer (see `go.mod`)
and no C toolchain.

## Build

```sh
go build .
go vet ./grammars          # or the package you changed
```

Build and vet the package you touch. The root package is large, so do not run
`go test ./...` or repository-wide `go build ./...` sweeps on a laptop;
AGENTS.md forbids them on the host because they can run out of memory.

## Test

Run focused tests with an anchored `-run` regex. These two commands are safe on
a developer machine:

```sh
go test -count=1 ./internal/sched ./internal/incr ./internal/lexrun
go test ./grammars -run '^(TestAllHighlightQueriesCompile|TestAllTagsQueriesCompile)$' -count=1
```

Do not run root-package tests without `-run`, and do not run race sweeps on a
host. CI runs the broad suites.

### Parity runs (Docker)

Correctness and parity checks against the C runtime run in Docker, one
language per container:

```sh
bash cgo_harness/docker/run_single_grammar_parity.sh typescript
```

For the smallest regression check in a package:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  -- "cd /workspace && go test ./grammargen -run '^TestTypeScriptConditionalTypeParity$' -count=1"
```

Other focused lanes, each for one language:

```sh
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs typescript
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs typescript
```

`run_grammargen_focus_targets.sh` runs with one CPU and one process
(`GOMAXPROCS=1`, `GOFLAGS=-p=1`). Fortran gets tighter memory limits unless you
override them.

## Send a pull request

1. Fork the repository and branch from `main`.
2. Make one focused change. Keep commits small.
3. Run `gofmt -l .` and `git diff --check`, then the focused tests above.
4. Push your branch and open a pull request. Fill in
   [the template](.github/PULL_REQUEST_TEMPLATE.md): what changed, why, which
   tests ran, and what you did not verify.

Report a parser bug with the parser report issue form, with a minimal input.

## Evidence rule

Back every claim with numbers in the pull request body: test output, parity
results, or benchmark results before and after. For performance, use
`scripts/run_randomized_benchmarks.sh` (fixed-order runs are not evidence) and
report `ns/op`, `B/op`, and `allocs/op`. Do not commit evidence files, receipts,
reports, specs, or plans to the repository. Put the numbers in the pull request
body or attach the files to the pull request.

## Maintainer tools

The maintainers use canopy, buckley, and Hyphae for code analysis, commits and
pull requests, and project knowledge. Outside contributors do not need them;
plain `git` and the pull request template are enough. The maintainer workflow
(lanes, counter ledger, gate presets) is in [AGENTS.md](AGENTS.md).

## More

- [docs/architecture.md](docs/architecture.md): how the runtime fits together.
- [docs/build-tags.md](docs/build-tags.md): build tags and environment
  variables.
- [docs/authoring-languages.md](docs/authoring-languages.md): adding a grammar.
- [docs/repository-map.md](docs/repository-map.md): where files live.
