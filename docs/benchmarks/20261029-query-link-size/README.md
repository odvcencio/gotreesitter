# Parse-only query linkage size

The private tree-view interface no longer mentions query types. Go's linker can omit query-only regular-expression compilation when the consumer parses but never uses queries. This does not remove the query API or change the aggregate grammar catalog.

The matched comparison used Go 1.25.14 on linux/amd64, CGO enabled, `GOMAXPROCS=1`, `GOFLAGS=-p=1`, and the existing `gotreesitter/cgo-harness:go1.25-local` Docker image. The in-module standalone fixture is [standalone.go.txt](standalone.go.txt). Both builds used checkout `a9ff1e7305bcb849c6b28309de20e49196546867` with `vcs.modified=true`; the before overlay restored the two files from that commit and the after build used the query interface split subsequently committed as `3254c609f`. No VCS stamping override was used. All reported binary sizes are bytes; one build per case, without timing claims.

| Standalone Go parser | Before | After | Difference |
| --- | ---: | ---: | ---: |
| Default linker flags | 15,893,646 | 15,599,041 | −294,605 (−1.85%) |
| `-ldflags='-s -w'` | 11,075,768 | 10,850,488 | −225,280 (−2.03%) |

Both binaries parsed `package main\n` and reported `false <nil>`. The baseline binary contained `regexp.(*Regexp).UnmarshalText`, `regexp.(*Regexp).MarshalText`, and `regexp.compile`; the candidate did not. Stripping is a separate optional build setting and is not the source of the comparison's code change.

For a separate consumer-path comparison at `36f6ff1bb` on the same Go and Docker settings, the [aggregate fixture](registry.go.txt) built to 35,314,144 bytes and the standalone fixture built to 15,893,662 bytes. Those command-line fixtures had no VCS metadata. This compares two existing APIs; it does not claim the aggregate binary became smaller. A [parse-capable root-only control](runtime-parse.go.txt) built to 14,504,268 bytes before and 14,212,941 after (unstripped); its empty language is never parsed unless an argument is supplied.

To reproduce the stamped standalone comparison, copy `standalone.go.txt` to an in-module temporary directory as `main.go`, then run `go build -o <output> ./<directory>` at each revision with the same flags. To reproduce this exact dirty-checkout comparison, use a Go `-overlay` restoring `tree.go` and `query.go` from `a9ff1e730` for the baseline, and build the candidate without that overlay in the same checkout. Inspect `go version -m <output>` for VCS metadata; preserve Git's safe-directory settings in containers rather than disabling VCS stamping.

Before and after, the tagged `TestNativeNodeViewsPublicAPI/go` and `TestNativeNodeViewsSparseQueryKeepsOutputCompact` passed. The candidate also passed `TestQueryEscapesSurviveCompilationAndExecution`. Tests ran under the bounded Docker parity runner, with `-tags gts_parsercorephase0` for the native-view tests. The first attempted run omitted the tag and selected no tests; it is not counted as validation. This is a focused Go/query check, not a full language-fleet or performance qualification.
