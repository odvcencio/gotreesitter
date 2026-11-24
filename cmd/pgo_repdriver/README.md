# Build-time PGO profile

`default.pgo` is the checked-in profile for binaries that opt into Go
profile-guided optimization with `-pgo=cmd/parity_report/default.pgo`. It is a deterministic
composition of two immutable evidence inputs:

- `inputs/production-v1.pgo` once: the established Go, C#, Bash, Python, and
  CMake production-corpus workload;
- `inputs/selected-clean-error-v1.pgo` twice: the authenticated compact
  selected-store fixtures plus clean and accepted-error parsing across eight
  grammars.

The `1:2` notation is profile multiplicity, not normalized sample time. The
component hashes and exact training inputs are recorded in
[`inputs/README.md`](inputs/README.md). The resulting `default.pgo` SHA-256 is
`1e5e9aea594f4fcbc3c0fb5d4064d2fb856b13b2faf0994f747520615eaa2ae2`.

## Measured scope

The composite was selected on revision
`54d9db2a063fc9ee738667c1723ff0c6733e9cf1` with Go 1.22.2 on the pinned
quiet host. Each strict comparison used one CPU, 750 ms benchmark windows,
ten samples per backend and fixture, a symmetric
off/current/candidate/candidate/current/off sequence, and quiet-host admission.

- On the four authenticated selected-store fixtures, the composite improved
  the equal-fixture geomean by **4.01% versus the previous profile** and 3.84%
  versus PGO-off. Every fixture improved versus the previous profile by
  3.30-5.03%, and exact selected-store admission stayed green.
- On the existing five-grammar `pgo_repdriver` workload, the composite was
  statistically indistinguishable from the previous profile (`p=0.971`, with a
  -0.14% median point estimate). It was **5.37% faster than PGO-off**. The
  candidate's production allocation medians were slightly higher than the
  previous profile: +0.43% B/op and +0.07% allocs/op. Off, previous, and
  composite builds produced the same 14-file digest
  (`e7b82899069a6cbb6e667266d214036e64beb114c2d8449de613bafa0ddb7286`).
- Median maximum RSS did not regress. It was 39,552 KiB versus 39,700 KiB on
  the selected-store board and 103,202 KiB versus 104,768 KiB on the
  production board.

These are scoped results for the two named workloads, not a fleet-wide or
universal Go performance claim. PGO changes generated machine code, not parse
semantics, but every replacement still requires explicit correctness and
performance gates.

## Reproducing the composition

Use the Go 1.22.2 toolchain used for the receipt and run from the repository
root:

```sh
GO=/path/to/go1.22.2/bin/go ./cmd/pgo_repdriver/compose_default.sh
```

The script:

- verifies both immutable input hashes;
- merges the production input once and the selected/clean/error input twice
  with `go tool pprof -proto`; and
- refuses to publish an output whose hash differs from the admitted artifact.

It may also write to a scratch destination:

```sh
GO=/path/to/go1.22.2/bin/go ./cmd/pgo_repdriver/compose_default.sh /tmp/default.pgo
cmp /tmp/default.pgo cmd/parity_report/default.pgo
```

Refreshing an input is a separate campaign, not a routine composition step.
For the production input, rebuild the lock-pinned corpus and collect the
profile with the existing driver:

```sh
cd cgo_harness
go run ./cmd/build_real_corpus -out corpus_real -langs go,c_sharp,bash,python,cmake
cd ..
go run ./cmd/pgo_repdriver -mode=profile \
  -cpuprofile=cmd/pgo_repdriver/inputs/production-v1.pgo -iterations=1500
```

After refreshing either input, update its provenance and hash, compose a new
candidate, and rerun both the selected-store admission/performance board and
the `BenchmarkParseCorpus` off/current/candidate board. The production digest
must remain byte-identical.

## Correctness check

With `cgo_harness/corpus_real` available:

```sh
go build -pgo=off -o /tmp/digest_nopgo ./cmd/pgo_repdriver
go build -pgo=cmd/parity_report/default.pgo -o /tmp/digest_pgo ./cmd/pgo_repdriver
diff <(/tmp/digest_nopgo -mode=digest) <(/tmp/digest_pgo -mode=digest)
```

## Where it is used

- `cmd/parity_report`, invoked by CI against all registered grammars, is built
  with `-pgo=cmd/parity_report/default.pgo`.
- gotreesitter ships as a Go module, not a compiled application. Downstream
  consumers receive this optimization only when their own main package is
  built with `-pgo=<path>` pointing to this profile (or a consumer-specific
  profile). Go auto-discovery does not search imported module dependencies.
- The profile sits in `cmd/parity_report/` under the name `default.pgo`, so Go's
  default `-pgo=auto` also applies it to plain `go build` and `go run` of that
  command. Other commands do not get it.
- Other repository commands are not automatically built with this profile.
  Pass `-pgo=cmd/parity_report/default.pgo` explicitly when a command's workload justifies
  it.
