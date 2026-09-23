# Third-party grammar licensing

gotreesitter's own source code uses the MIT license. See [LICENSE](../LICENSE).

gotreesitter also embeds grammar tables, and in a few cases hand-ported Go
scanners, from 206 upstream tree-sitter grammar repositories. This document
records each repository's confirmed license, explains the GPL-3.0 and
MPL-2.0 cases, and describes the `gotreesitter_no_copyleft` build tag.

This document is not legal advice. Ask a lawyer before you rely on it for a
compliance decision.

## The audit

[licenses/grammars.json](../licenses/grammars.json) is the source of truth.
It has one entry per grammar in
[grammars/languages.lock](../grammars/languages.lock): the upstream repository
URL, the pinned commit, the confirmed SPDX license identifier, the copyright
holder (where one is stated), the license file URL, and the confirmation
method.

[THIRD_PARTY_NOTICES](../THIRD_PARTY_NOTICES) is generated from that audit.
Run this command after you edit `licenses/grammars.json`:

```
go run ./cmd/gen_license_notices
```

Add `-check` to fail instead of writing, for use in CI.

### Confirmation method

Each entry names how gotreesitter confirmed its license:

- **`github_license_api`.** The GitHub REST API's license endpoint, called
  with the entry's pinned commit as the `ref` parameter. This method reads
  the actual file at that commit, not the repository's current default
  branch.
- **`license_file_content`** (and its `_subdir` and other suffixed
  variants). A human read the LICENSE, COPYING, or `SPDX-License-Identifier`
  file directly, because the GitHub API returned no result or an ambiguous
  one (`NOASSERTION`) at that ref.
- **`manifest_field_no_license_file`** or **`manifest_field_majority...`**.
  No LICENSE or COPYING file exists at the pinned ref. The license comes
  from a `"license"` field in `package.json` or `Cargo.toml` instead.
- **`notice_file_carveout`.** The repository's root LICENSE names one
  license, but its NOTICE file assigns a different license to the specific
  subdirectory gotreesitter vendors. See the elixir case below.

An entry's `note` field, where present, records exactly what a human found
and why. An entry's `copyright_holders` field is empty when no copyright
line could be found; in that case the entry also carries a
`repository_owner` field, labeled explicitly as unconfirmed, never as a
copyright statement.

### Two corrections this audit made

The task that produced this audit started from a GitHub-detected count of 4
GPL-3.0 repositories among gotreesitter's 206 grammars. Two of those
findings needed correction once checked against the actual file at the
pinned commit, illustrating why every entry needs individual confirmation:

1. **ebnf is MIT, not GPL-3.0.** `RubixDev/ebnf` is a Cargo workspace.
   Three of its four crates (`ebnf-parser`, `ebnf-fmt`,
   `dprint-plugin-ebnf`) are GPL-3.0-only, but gotreesitter vendors only
   `crates/tree-sitter-ebnf/src`, which carries its own MIT `LICENSE` file
   and its own `Cargo.toml` `license = "MIT"` field. The repository has no
   root-level LICENSE file, so GitHub's whole-repository license detection
   reported the workspace's dominant license, not the specific subcrate
   gotreesitter uses.

2. **elixir's vendored grammar source is MIT, not Apache-2.0.** The
   `elixir-lang/tree-sitter-elixir` root LICENSE is Apache-2.0 and covers
   the repository by default. Its NOTICE file, however, states that `src/`
   (what gotreesitter vendors) and `test/corpus/` fragments are MIT,
   copyright Max Brunsfeld and Anantha Kumaran respectively, and that only
   "all other files" (documentation, CI configuration; nothing gotreesitter
   vendors) stay Apache-2.0 under The Elixir Team's copyright.
   THIRD_PARTY_NOTICES reproduces the full upstream NOTICE file out of
   caution, since it governs both licenses.

Read each grammar's `note` field in `licenses/grammars.json` for the full
citation trail behind every other entry.

## License summary

Run `go run ./cmd/gen_license_notices` and read the Summary section of
THIRD_PARTY_NOTICES for the current counts. As of this audit:

| SPDX identifier | Count |
| --- | --- |
| MIT | 178 |
| Apache-2.0 | 13 |
| GPL-3.0 | 3 |
| CC0-1.0 | 3 |
| ISC | 3 |
| WTFPL | 2 |
| Unlicense | 1 |
| MPL-2.0 | 1 |
| Apache-2.0 OR MIT | 1 |
| Apache-2.0 WITH LLVM-exception | 1 |

No entry in this audit is UNKNOWN. See "Repositories GitHub could not
detect a license for" below for how the nine repositories the task
originally flagged this way all resolved to a confirmed license instead.

## Copyleft grammars: GPL-3.0 and MPL-2.0

Four grammars come from copyleft-licensed upstream repositories:

| Grammar | SPDX | Upstream repository |
| --- | --- | --- |
| caddy | GPL-3.0 | opa-oz/tree-sitter-caddy |
| disassembly | GPL-3.0 | ColinKennedy/tree-sitter-disassembly |
| jq | GPL-3.0 | nverno/tree-sitter-jq |
| nim | MPL-2.0 | alaviss/tree-sitter-nim |

For caddy, disassembly, and nim, gotreesitter also ships a hand-written Go
port of the upstream external scanner (`src/scanner.c`), because these
grammars need one. Each port carries an SPDX header naming its license and
copyright holder:

- [grammars/runtime/caddy_scanner.go](../grammars/runtime/caddy_scanner.go)
- [grammars/runtime/disassembly_scanner.go](../grammars/runtime/disassembly_scanner.go)
- [grammars/runtime/nim_scanner.go](../grammars/runtime/nim_scanner.go)

jq has no hand-ported scanner; its grammar defines no external tokens, so
gotreesitter carries only its grammar table.

### What this means for a consumer

gotreesitter's own code stays MIT throughout. A program that imports
gotreesitter and never touches the caddy, disassembly, jq, or nim grammar
carries no GPL-3.0 or MPL-2.0 obligation from this project.

A program that imports gotreesitter's aggregate `grammars` package (the
normal `import "github.com/odvcencio/gotreesitter/grammars"` path) compiles
and embeds all four of these grammars by default, along with everything
else. A program that then distributes a binary built that way is
distributing GPL-3.0-covered and MPL-2.0-covered material (the caddy,
disassembly, and nim scanner source in every case; the jq, caddy,
disassembly, and nim grammar tables as well), and should check its own
GPL-3.0 and MPL-2.0 obligations before it does. Consult a lawyer for what
those obligations are; the plain-language summary is that GPL-3.0 is the
stricter of the two and generally treats a work that closely integrates
GPL-3.0 code as needing to carry the same terms, while MPL-2.0 is
file-scoped and requires only that the covered files themselves stay
available under MPL-2.0 terms.

### The `gotreesitter_no_copyleft` build tag

Build with this tag to remove all four grammars from the default build:

```
go build -tags gotreesitter_no_copyleft ./...
```

This tag is additive and non-breaking: it changes nothing when absent, and
the default (no-tag) build keeps today's behavior exactly.

With the tag set:

- caddy, disassembly, jq, and nim disappear from the public grammar
  registry. `grammars.AllLanguages()`, `grammars.DetectLanguage()`, and
  `grammars.DetectLanguageByName()` never return them.
- The hand-ported scanner Go source for caddy, disassembly, and nim
  (`grammars/runtime/{caddy,disassembly,nim}_scanner.go`) does not compile
  into the binary at all. A stub file
  (`{caddy,disassembly,nim}_no_copyleft_stub.go`) keeps the small internal
  registration function each scanner file's absence would otherwise leave
  undefined, but that stub function does nothing.
- [`grammars/copyleft_language_set_test.go`](../grammars/copyleft_language_set_test.go)
  checks the registry-exclusion half of this contract under both build
  configurations.

The tag has two known limits, both a direct consequence of this task's
"do not move or delete grammars" scope:

1. **The embedded grammar blob bytes stay in the binary.** gotreesitter
   embeds every grammar's compiled table (`grammars/grammar_blobs/*.bin`)
   through one shared `//go:embed grammar_blobs/*.bin` directive per build
   configuration. `go:embed` has no per-file exclude option, only
   include-globs and explicit file lists, so removing caddy.bin,
   disassembly.bin, jq.bin, and nim.bin from the compiled binary requires
   moving those four files to a separately embedded directory (see "Moving
   to opt-in modules" below for the effort that takes). Until then, the
   four grammars' table bytes still ship in every binary built from the
   aggregate `grammars` package, tagged or not; the tag controls whether
   they are registered and reachable through the public API, not whether
   they are present in the compiled artifact.
2. **A caller that bypasses the registry still reaches a degraded grammar.**
   `grammars.CaddyLanguage()` (and the equivalent function for the other
   three) still exists and still compiles under the tag, because it lives
   in a file the tag does not exclude. Calling it directly returns a
   `*gotreesitter.Language` decoded from the embedded blob, but with no
   external scanner attached for caddy, disassembly, or nim (since the tag
   excluded that scanner's registration), so every external token that
   grammar defines goes unhandled. The same applies to the standalone
   `grammars/caddy`, `grammars/disassembly`, `grammars/jq`, and
   `grammars/nim` packages, which a caller can still import directly
   regardless of this tag. The tag's guarantee is that
   `AllLanguages`/`DetectLanguage` (the paths essentially every consumer
   actually uses) never surface these four grammars, and that their scanner
   source never compiles in; it is not a guarantee that no code path can
   reach them at all.

## Moving to opt-in modules: what it would cost

This audit's build tag is a cheap, additive opt-out. A more complete answer
— fully separate, opt-in Go modules per grammar, so a consumer's own
`go.mod` controls exactly which grammars (and which licenses) it pulls in
— is a real option, but a much larger change. Estimated effort, for the
user's decision:

- **Blob relocation.** Every grammar's `.bin` file moves out of the single
  shared `grammars/grammar_blobs/` directory current `//go:embed` globs
  cover. This touches the ts2go pipeline that generates
  `grammars/registry_builtin_gen.go`, `grammars/blob_source_embedded.go`,
  and `grammars/embedded_grammars_gen.go` (roughly 4,000 generated lines
  today, produced by `cmd/ts2go`, an ~8,900-line generator), plus the
  `grammar_set_core` and `grammar_subset` build variants, which use their
  own embed strategies already.
- **Module boundaries.** Each grammar (or each license class of grammars)
  needs its own `go.mod`, its own module path, and its own release
  versioning, replacing the current single-module, single-version release
  gate (`make release-gate`, `release-governed.yml`).
- **The standalone per-grammar packages already exist as a partial
  answer.** `grammars/<name>` (for example `grammars/python`,
  `grammars/caddy`) is already a self-contained package: it embeds only
  that grammar's blob and registers only that grammar's scanner, all
  without importing the aggregate `grammars` package. A consumer that
  imports only the specific `grammars/<name>` packages it wants, and never
  imports the aggregate `grammars` package, already avoids compiling in any
  grammar (including caddy, disassembly, jq, or nim) it did not ask for.
  What this does not yet give a consumer is separate go.mod boundaries or
  independent versioning; every `grammars/<name>` package still lives in
  gotreesitter's single module today.
- **Breaking impact.** Splitting into modules breaks
  `import "github.com/odvcencio/gotreesitter/grammars"` for every existing
  consumer of the aggregate package; each would need to switch to
  importing the specific per-grammar modules it needs. This is a major,
  coordinated migration, not a patch release.

Given the `grammars/<name>` packages already provide most of the practical
benefit (compile in only what you import) without any of this cost, this
audit recommends treating full module separation as a future option to
revisit only if a consumer specifically needs independent per-grammar
versioning, not as a near-term priority.

## Apache-2.0 NOTICE files

Two upstream repositories ship a NOTICE file: elixir and pkl.
THIRD_PARTY_NOTICES reproduces both in full, fetched at the pinned commit,
under "Upstream NOTICE files". elixir's case is also covered above, since
its NOTICE reassigns most of the repository's license away from Apache-2.0
for the part gotreesitter actually vendors.

## Repositories GitHub could not detect a license for

The task that produced this audit started from GitHub's repository-level
license detection, which flagged nine repositories with no detectable
license. This audit re-checked all ten repositories GitHub's API returned
`404` or `NOASSERTION` for at the exact pinned commit (one more than the
original nine, since ref-pinning surfaces a different set than a
whole-repository, default-branch check does), plus the `move`, `dhall`,
`ron`, and `wat` repositories, which returned `NOASSERTION` specifically.
Every one of them resolved to a confirmed SPDX license, using the license
file directly or, where no LICENSE file exists at all, the `"license"`
field in `package.json` or `Cargo.toml`:

| Grammar | Resolution | Method |
| --- | --- | --- |
| authzed | MIT | package.json and Cargo.toml agree |
| brightscript | MIT | Cargo.toml and pyproject.toml agree; package.json says ISC, flagged as likely unedited scaffold boilerplate (see note in the audit) |
| cooklang | MIT | Cargo.toml and pyproject.toml agree; package.json says ISC, same pattern as brightscript |
| corn | MIT | package.json and Cargo.toml agree |
| ebnf | MIT | LICENSE file inside the vendored subdirectory (see "Two corrections" above) |
| eds | MIT | Cargo.toml |
| eex | MIT | package.json and Cargo.toml agree |
| elsa | MIT | package.json and Cargo.toml agree |
| facility | MIT | package.json and Cargo.toml agree |
| tmux | MIT | package.json and Cargo.toml agree |
| move | Apache-2.0 | LICENSE file's own `SPDX-License-Identifier: Apache-2.0` header |
| dhall | MIT | LICENSE file text |
| ron | Apache-2.0 OR MIT | LICENSE-APACHE and LICENSE-MIT (dual license, standard for the Rust ecosystem) |
| wat | Apache-2.0 WITH LLVM-exception | LICENSE file's own SPDX-expression text |

**Recommended action:** none of these need upstream contact, removal, or a
standing risk note; every one now carries a confirmed, permissive license.
The two with a package.json/Cargo.toml mismatch (brightscript, cooklang)
are the only pair worth a low-priority upstream question, since a
maintainer could in principle confirm ISC was deliberate; both licenses are
permissive, so the practical risk from either answer is small.

## Recording a new grammar's license

When `grammars/languages.lock` gains a new entry:

1. Add a matching entry to `licenses/grammars.json`, following an existing
   entry's shape. Confirm the SPDX id from the actual file at the pinned
   commit — `gh api "repos/<owner>/<repo>/license?ref=<sha>"` is the fastest
   path; fall back to reading the LICENSE file directly, then to a
   `package.json`/`Cargo.toml`/`tree-sitter.json` `"license"` field, in that
   order. Never guess; use `"spdx": "UNKNOWN"` and explain why in `note` if
   nothing confirms a license.
2. If the SPDX id has no canonical text yet under `licenses/texts/`, add
   one (`licenses/texts/<SPDX-ID>.txt`) and register it in
   `cmd/gen_license_notices/main.go`'s `spdxTextFiles` map.
3. Run `go run ./cmd/gen_license_notices` to regenerate
   `THIRD_PARTY_NOTICES`.
4. If the new grammar is GPL, LGPL, AGPL, or MPL licensed, add its name to
   `copyleftGrammarNames` in
   [grammars/copyleft_language_set_no_copyleft.go](../grammars/copyleft_language_set_no_copyleft.go),
   and to `copyleftGrammarsUnderTest` in
   [grammars/copyleft_language_set_test.go](../grammars/copyleft_language_set_test.go).
   If it also needs a hand-ported external scanner, give that scanner file
   the same `&& !gotreesitter_no_copyleft` build-tag treatment as
   `caddy_scanner.go`, with a matching `_no_copyleft_stub.go` companion.

A CI check (`go run ./cmd/gen_license_notices -check`) fails the build when
`licenses/grammars.json` and `grammars/languages.lock` disagree, or when
`THIRD_PARTY_NOTICES` is stale relative to the audit.
