# Wave 4 External Lex-State Election Ledger

This ledger covers every grammar in `cgo_harness/tier_scan/exts.tsv`.
It tracks whether each grammar's external-scanner recovery path is
default-elected, staged, blocked on a missing precise ExternalLexStates
table, or not applicable because the grammar has no registered Go external
scanner.

## Summary

| metric | count |
| --- | ---: |
| grammars | 206 |
| registered external scanners | 119 |
| default precise ExternalLexStates tables | 90 |
| staged precise ExternalLexStates tables | 1 |
| C recovery default opt-outs | 3 |

| status | count |
| --- | ---: |
| default elected | 87 |
| staged precise ELS | 4 |
| blocked: missing precise ELS | 28 |
| not applicable: no external scanner | 87 |

## Verification Receipts

- `default_elected`: Docker: wave4-external-lex-election-inventory-test-v2; TestExternalLexStatesDefaultElectionInventory; Docker: wave4-cobol-default-precise-els; TestCobolExternalLexStatesDefaultElection
- `staged_precise_els`: Docker: wave4-javascript-precise-els-staged-test; TestJavascriptExternalLexStatesRegression (-tags javascript_precise_els); TestJavascriptExternalLexStatesRemainStagedByDefault; TestExternalLexStatesRecoveryElectionOptOutInventory
- `sample_c_oracle_smoke`: Docker: wave4-external-lex-smoke-20260707T1928; angular/python/yaml clean; scss/wgsl classified recovery/error-shape IV
- `wave3_inventory`: Docker: wave3-tier-plan-206; 206 visited; 202 planned files; 4 planned-empty

## Status Definitions

- `default_elected`: External scanner and precise ExternalLexStates table are registered by default; DiagnoseCRecoveryGate supports the language and C recovery cost competition is default-enabled.
- `staged_precise_els`: A precise ExternalLexStates table exists behind an opt-in build tag or explicit staging policy; default C recovery election is intentionally disabled.
- `blocked_missing_precise_els`: External scanner is registered, but no precise ExternalLexStates table is registered or staged yet. Wave 4 cannot default-elect C recovery for this grammar until the table is extracted and certified.
- `not_applicable_no_external_scanner`: No Go external scanner is registered for this grammar, so there is no external-lex-state election to perform. Tier/parity status remains governed by the ordinary C-oracle classification.

## Grammar Ledger

| grammar | status | tier row | parity | scanner | default ELS | staged ELS | recovery opt-out | extensions |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `angular` | default elected | IV-recovery? | 22/40 | yes | yes | no | no | `.html` |
| `arduino` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.ino` |
| `astro` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.astro` |
| `awk` | default elected | IV-unknown | 25/29 | yes | yes | no | no | `.auk,.awk,.gawk,.mawk,.nawk` |
| `bash` | default elected | IV-recovery? | unmeasured | yes | yes | no | no | `.sh` |
| `beancount` | default elected | CLEAN | 3/3 | yes | yes | no | no | `.bean,.beancount` |
| `bicep` | default elected | IV-unknown | 34/40 | yes | yes | no | no | `.bicep` |
| `bitbake` | default elected | IV-unknown | 35/40 | yes | yes | no | no | `.bb,.bbappend,.bbclass` |
| `blade` | default elected | IV-recovery? | unmeasured | yes | yes | no | no | `.blade.php` |
| `c_sharp` | default elected | IV-recovery | unmeasured | yes | yes | no | no | `.cs` |
| `caddy` | default elected | IV-recovery? | 11/40 | yes | yes | no | no | `.caddy,caddyfile` |
| `cairo` | default elected | IV-recovery? | 1/40 | yes | yes | no | no | `.cairo` |
| `cmake` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.cmake,.cmake.in` |
| `cobol` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.cbl,.cob,.cpy` |
| `cooklang` | default elected | IV-recovery | 0/3 | yes | yes | no | no | `.cook` |
| `crystal` | default elected | IV-shape? | unmeasured | yes | yes | no | no | `.cr` |
| `css` | default elected | IV-unknown | 37/40 | yes | yes | no | no | `.css` |
| `cuda` | default elected | IV-recovery? | 21/40 | yes | yes | no | no | `.cu,.cuh` |
| `cue` | default elected | IV-unknown | 39/40 | yes | yes | no | no | `.cue` |
| `d` | default elected | IV-recovery? | unmeasured | yes | yes | no | no | `.d,.di` |
| `dart` | default elected | IV-recovery? | 0/40 | yes | yes | no | no | `.dart` |
| `disassembly` | default elected | IV-version | 0/40 | yes | yes | no | no | `.dis,.dump` |
| `dockerfile` | default elected | IV-recovery? | unmeasured | yes | yes | no | no | `.dockerfile,dockerfile` |
| `dtd` | default elected | IV-unknown | 0/3 | yes | yes | no | no | `.dtd` |
| `earthfile` | default elected | IV-recovery? | 0/40 | yes | yes | no | no | `earthfile` |
| `editorconfig` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.editorconfig` |
| `elixir` | default elected | IV-recovery | unmeasured | yes | yes | no | no | `.ex,.exs` |
| `elm` | default elected | IV-unknown | 0/8 | yes | yes | no | no | `.elm` |
| `erlang` | default elected | IV-recovery | unmeasured | yes | yes | no | no | `.app,.app.src,.erl,.escript,.hrl,.xrl,.yrl` |
| `fennel` | default elected | IV-recovery? | 15/40 | yes | yes | no | no | `.fnl` |
| `firrtl` | default elected | IV-recovery? | 12/27 | yes | yes | no | no | `.fir` |
| `foam` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.foam` |
| `fortran` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.f,.f03,.f08,.f90,.f95` |
| `gdscript` | default elected | IV-unknown | 39/40 | yes | yes | no | no | `.gd` |
| `gitcommit` | default elected | CLEAN | 13/13 | yes | yes | no | no | `commit_editmsg` |
| `gleam` | default elected | IV-unknown | 39/40 | yes | yes | no | no | `.gleam` |
| `gn` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.gn,.gni` |
| `hack` | default elected | IV-unknown | 38/40 | yes | yes | no | no | `.hack,.hh` |
| `haxe` | default elected | IV-recovery | 10/40 | yes | yes | no | no | `.hx` |
| `hlsl` | default elected | IV-unknown | 35/40 | yes | yes | no | no | `.fx,.hlsl` |
| `janet` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.janet` |
| `jsdoc` | default elected | IV-unknown | 13/40 | yes | yes | no | no | `.jsdoc` |
| `jsonnet` | default elected | IV-recovery? | 39/40 | yes | yes | no | no | `.jsonnet,.libsonnet` |
| `just` | default elected | IV-unknown | 2/8 | yes | yes | no | no | `.just,justfile` |
| `kconfig` | default elected | IV-recovery? | 18/40 | yes | yes | no | no | `kconfig` |
| `kdl` | default elected | IV-recovery | 11/40 | yes | yes | no | no | `.kdl` |
| `kotlin` | default elected | IV-unknown | 22/40 | yes | yes | no | no | `.kt,.kts` |
| `less` | default elected | IV-recovery? | 10/40 | yes | yes | no | no | `.less` |
| `liquid` | default elected | IV-recovery? | 11/36 | yes | yes | no | no | `.liquid` |
| `lua` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.lua` |
| `luau` | default elected | IV-perf | unmeasured | yes | yes | no | no | `.luau` |
| `matlab` | default elected | IV-recovery? | 4/40 | yes | yes | no | no | `.m,.mat` |
| `mojo` | default elected | IV-recovery? | 29/40 | yes | yes | no | no | `.mojo,.🔥` |
| `move` | default elected | IV-recovery? | 14/40 | yes | yes | no | no | `.move` |
| `nickel` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.ncl` |
| `nim` | default elected | IV-recovery? | unmeasured | yes | yes | no | no | `.nim,.nims` |
| `nushell` | default elected | IV-recovery? | 7/40 | yes | yes | no | no | `.nu` |
| `odin` | default elected | IV-recovery? | 1/40 | yes | yes | no | no | `.odin` |
| `org` | default elected | IV-recovery? | 1/6 | yes | yes | no | no | `.org` |
| `php` | default elected | IV-unknown | 36/40 | yes | yes | no | no | `.php` |
| `pkl` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.pkl` |
| `powershell` | default elected | IV-recovery? | 22/40 | yes | yes | no | no | `.ps1,.psd1,.psm1` |
| `pug` | default elected | IV-recovery? | 0/40 | yes | yes | no | no | `.jade,.pug` |
| `purescript` | default elected | IV-recovery? | 1/40 | yes | yes | no | no | `.purs` |
| `python` | default elected | IV-unknown | 6/40 | yes | yes | no | no | `.py` |
| `r` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.r` |
| `rescript` | default elected | IV-recovery? | 24/40 | yes | yes | no | no | `.res,.resi` |
| `ron` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.ron` |
| `ruby` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.rb` |
| `rust` | default elected | IV-recovery? | 11/40 | yes | yes | no | no | `.rs` |
| `scala` | default elected | IV-recovery? | 5/40 | yes | yes | no | no | `.scala` |
| `scss` | default elected | IV-recovery? | 6/40 | yes | yes | no | no | `.scss` |
| `sql` | default elected | IV-recovery? | 8/40 | yes | yes | no | no | `.sql` |
| `squirrel` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.nut` |
| `starlark` | default elected | IV-unknown | 34/40 | yes | yes | no | no | `.bzl,.star` |
| `svelte` | default elected | IV-unknown | 8/40 | yes | yes | no | no | `.svelte` |
| `tablegen` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.td` |
| `tcl` | default elected | IV-recovery? | unmeasured | yes | yes | no | no | `.tcl` |
| `teal` | default elected | IV-recovery? | 8/40 | yes | yes | no | no | `.tl` |
| `templ` | default elected | IV-recovery? | 31/40 | yes | yes | no | no | `.templ` |
| `tsx` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.tsx` |
| `typst` | default elected | IV-recovery? | unmeasured | yes | yes | no | no | `.typ` |
| `uxntal` | default elected | IV-recovery? | 0/40 | yes | yes | no | no | `.tal` |
| `vhdl` | default elected | IV-recovery? | unmeasured | yes | yes | no | no | `.vhd,.vhdl` |
| `vue` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.vue` |
| `wgsl` | default elected | IV-recovery | 21/40 | yes | yes | no | no | `.wgsl` |
| `yaml` | default elected | CLEAN | 40/40 | yes | yes | no | no | `.yaml,.yml` |
| `cpp` | staged precise ELS | IV-recovery | 9/40 | yes | yes | no | yes | `.cc,.cpp,.cxx,.h,.hh,.hpp,.hxx` |
| `html` | staged precise ELS | IV-recovery | 0/40 | yes | yes | no | yes | `.htm,.html` |
| `javascript` | staged precise ELS | CLEAN | 40/40 | yes | no | yes | no | `.cjs,.js,.mjs` |
| `julia` | staged precise ELS | IV-recovery | unmeasured | yes | yes | no | yes | `.jl` |
| `agda` | blocked: missing precise ELS | IV-scanner | 2/40 | yes | no | no | no | `.agda` |
| `comment` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.txt` |
| `dhall` | blocked: missing precise ELS | IV-unknown | 15/40 | yes | no | no | no | `.dhall` |
| `djot` | blocked: missing precise ELS | IV-scanner? | unmeasured | yes | no | no | no | `.dj,.djot` |
| `doxygen` | blocked: missing precise ELS | IV-unknown | 20/40 | yes | no | no | no | `.dox,.doxygen` |
| `fish` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.fish` |
| `fsharp` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.fs,.fsi,.fsx` |
| `go` | blocked: missing precise ELS | IV-unknown | 25/40 | yes | no | no | no | `.go` |
| `godot_resource` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.tres,.tscn` |
| `haskell` | blocked: missing precise ELS | IV-scanner | unmeasured | yes | no | no | no | `.hs,.lhs` |
| `hcl` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.hcl,.tf,.tfvars` |
| `markdown` | blocked: missing precise ELS | IV-unknown | 0/40 | yes | no | no | no | `.md` |
| `markdown_inline` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.md` |
| `nginx` | blocked: missing precise ELS | IV-unknown | 0/1 | yes | no | no | no | `.nginx,.nginxconf,.vhost,nginx.conf` |
| `nix` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.nix` |
| `norg` | blocked: missing precise ELS | IV-scanner | 0/2 | yes | no | no | no | `.norg` |
| `ocaml` | blocked: missing precise ELS | IV-unknown | 12/40 | yes | no | no | no | `.ml,.mli` |
| `perl` | blocked: missing precise ELS | IV-recovery? | unmeasured | yes | no | no | no | `.pl,.pm` |
| `properties` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.properties` |
| `racket` | blocked: missing precise ELS | IV-unknown | 2/40 | yes | no | no | no | `.rkt` |
| `rst` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.rst` |
| `swift` | blocked: missing precise ELS | IV-recovery? | 0/40 | yes | no | no | no | `.swift` |
| `tlaplus` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.tla` |
| `toml` | blocked: missing precise ELS | IV-unknown | 8/11 | yes | no | no | no | `.toml` |
| `typescript` | blocked: missing precise ELS | IV-perf | unmeasured | yes | no | no | no | `.ts` |
| `wolfram` | blocked: missing precise ELS | IV-unknown | 0/11 | yes | no | no | no | `.m,.nb,.wl` |
| `xml` | blocked: missing precise ELS | IV-unknown | 1/40 | yes | no | no | no | `.xml` |
| `yuck` | blocked: missing precise ELS | IV-unknown | 1/2 | yes | no | no | no | `.yuck` |
| `ada` | not applicable: no external scanner | IV-unknown | 39/40 | no | no | no | no | `.adb,.ads` |
| `apex` | not applicable: no external scanner | IV-unknown | 39/40 | no | no | no | no | `.cls,.trigger` |
| `asm` | not applicable: no external scanner | IV-recovery | 0/40 | no | no | no | no | `.asm,.s` |
| `authzed` | not applicable: no external scanner | IV-unknown | 36/40 | no | no | no | no | `.zed` |
| `bass` | not applicable: no external scanner | IV-unknown | 39/40 | no | no | no | no | `.bass` |
| `bibtex` | not applicable: no external scanner | IV-unknown | 38/40 | no | no | no | no | `.bib` |
| `brightscript` | not applicable: no external scanner | IV-recovery? | 11/40 | no | no | no | no | `.brs` |
| `c` | not applicable: no external scanner | IV-recovery | 22/40 | no | no | no | no | `.c,.h` |
| `capnp` | not applicable: no external scanner | IV-unknown | 14/21 | no | no | no | no | `.capnp` |
| `chatito` | not applicable: no external scanner | IV-unknown | 1/5 | no | no | no | no | `.chatito` |
| `circom` | not applicable: no external scanner | IV-shape? | 19/40 | no | no | no | no | `.circom` |
| `clojure` | not applicable: no external scanner | IV-perf | unmeasured | no | no | no | no | `.clj,.cljc,.cljs,.edn` |
| `commonlisp` | not applicable: no external scanner | IV-recovery? | unmeasured | no | no | no | no | `.asd,.cl,.lisp,.lsp` |
| `corn` | not applicable: no external scanner | IV-unknown | 22/23 | no | no | no | no | `.corn` |
| `cpon` | not applicable: no external scanner | IV-unknown | 9/10 | no | no | no | no | `.cpon` |
| `csv` | not applicable: no external scanner | IV-perf | unmeasured | no | no | no | no | `.csv,.tsv` |
| `cylc` | not applicable: no external scanner | IV-recovery? | unmeasured | no | no | no | no | `.cylc` |
| `desktop` | not applicable: no external scanner | IV-perf | unmeasured | no | no | no | no | `.desktop,.desktop.in,.service` |
| `devicetree` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.dts,.dtsi` |
| `diff` | not applicable: no external scanner | IV-perf | unmeasured | no | no | no | no | `.diff,.patch` |
| `dot` | not applicable: no external scanner | IV-perf | unmeasured | no | no | no | no | `.dot,.gv` |
| `ebnf` | not applicable: no external scanner | IV-recovery? | 0/40 | no | no | no | no | `.ebnf` |
| `eds` | not applicable: no external scanner | IV-unknown | 0/1 | no | no | no | no | `.eds` |
| `eex` | not applicable: no external scanner | IV-unknown | 21/40 | no | no | no | no | `.eex,.heex,.html.eex,.leex` |
| `elisp` | not applicable: no external scanner | IV-perf | unmeasured | no | no | no | no | `.el` |
| `elsa` | not applicable: no external scanner | IV-recovery? | 0/0 | no | no | no | no | `.elsa` |
| `embedded_template` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.ejs,.erb` |
| `enforce` | not applicable: no external scanner | IV-unknown | 28/40 | no | no | no | no | `.c,.enf` |
| `facility` | not applicable: no external scanner | IV-unknown | 1/4 | no | no | no | no | `.fac,.fsd` |
| `faust` | not applicable: no external scanner | IV-unknown | 39/40 | no | no | no | no | `.dsp` |
| `fidl` | not applicable: no external scanner | IV-unknown | 37/40 | no | no | no | no | `.fidl` |
| `forth` | not applicable: no external scanner | IV-perf | unmeasured | no | no | no | no | `.4th,.fs,.fth` |
| `git_config` | not applicable: no external scanner | CLEAN | 7/7 | no | no | no | no | `.gitconfig` |
| `git_rebase` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.git-rebase-todo` |
| `gitattributes` | not applicable: no external scanner | IV-unknown | 9/10 | no | no | no | no | `.gitattributes` |
| `gitignore` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.gitignore` |
| `glsl` | not applicable: no external scanner | IV-recovery | 7/40 | no | no | no | no | `.frag,.glsl,.vert` |
| `gomod` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.mod` |
| `graphql` | not applicable: no external scanner | IV-unknown | 0/1 | no | no | no | no | `.gql,.graphql` |
| `groovy` | not applicable: no external scanner | IV-recovery | 6/40 | no | no | no | no | `.groovy,.gvy` |
| `hare` | not applicable: no external scanner | IV-recovery | 15/40 | no | no | no | no | `.ha` |
| `heex` | not applicable: no external scanner | IV-unknown | 6/7 | no | no | no | no | `.heex` |
| `http` | not applicable: no external scanner | IV-unknown | 10/11 | no | no | no | no | `.http` |
| `hurl` | not applicable: no external scanner | IV-recovery | 14/40 | no | no | no | no | `.hurl` |
| `hyprlang` | not applicable: no external scanner | IV-unknown | 1/2 | no | no | no | no | `.conf` |
| `ini` | not applicable: no external scanner | IV-unknown | 9/11 | no | no | no | no | `.cfg,.conf,.ini` |
| `java` | not applicable: no external scanner | IV-unknown | 22/40 | no | no | no | no | `.java` |
| `jinja2` | not applicable: no external scanner | IV-recovery | 3/40 | no | no | no | no | `.j2,.jinja,.jinja2` |
| `jq` | not applicable: no external scanner | IV-unknown | 7/8 | no | no | no | no | `.jq` |
| `json` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.json` |
| `json5` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.json5` |
| `ledger` | not applicable: no external scanner | IV-unknown | 2/4 | no | no | no | no | `.journal,.ledger` |
| `linkerscript` | not applicable: no external scanner | IV-recovery | 1/40 | no | no | no | no | `.ld,.lds` |
| `llvm` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.ll` |
| `make` | not applicable: no external scanner | IV-recovery? | unmeasured | no | no | no | no | `.mak,.mk,gnumakefile,makefile` |
| `mermaid` | not applicable: no external scanner | IV-recovery? | 0/40 | no | no | no | no | `.mermaid,.mmd` |
| `meson` | not applicable: no external scanner | IV-recovery? | 2/40 | no | no | no | no | `meson.build` |
| `ninja` | not applicable: no external scanner | IV-unknown | 3/5 | no | no | no | no | `.ninja` |
| `objc` | not applicable: no external scanner | IV-recovery | unmeasured | no | no | no | no | `.m` |
| `pascal` | not applicable: no external scanner | IV-recovery? | 0/40 | no | no | no | no | `.inc,.pas,.pp` |
| `pem` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.pem` |
| `prisma` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.prisma` |
| `prolog` | not applicable: no external scanner | IV-recovery? | 4/40 | no | no | no | no | `.pl,.pro` |
| `promql` | not applicable: no external scanner | IV-unknown | 0/4 | no | no | no | no | `.promql` |
| `proto` | not applicable: no external scanner | IV-recovery? | 24/40 | no | no | no | no | `.proto` |
| `puppet` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.pp` |
| `ql` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.ql` |
| `regex` | not applicable: no external scanner | CLEAN | 1/1 | no | no | no | no | `.regex` |
| `rego` | not applicable: no external scanner | IV-recovery? | 7/40 | no | no | no | no | `.rego` |
| `requirements` | not applicable: no external scanner | IV-unknown | 8/9 | no | no | no | no | `requirements.txt` |
| `robot` | not applicable: no external scanner | IV-recovery? | 28/40 | no | no | no | no | `.robot` |
| `scheme` | not applicable: no external scanner | IV-perf | 23/40 | no | no | no | no | `.scm,.ss` |
| `smithy` | not applicable: no external scanner | IV-unknown | 34/40 | no | no | no | no | `.smithy` |
| `solidity` | not applicable: no external scanner | IV-unknown | 6/40 | no | no | no | no | `.sol` |
| `sparql` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.rq,.sparql` |
| `ssh_config` | not applicable: no external scanner | IV-unknown | 1/2 | no | no | no | no | `config,ssh_config,sshd_config` |
| `textproto` | not applicable: no external scanner | IV-perf | unmeasured | no | no | no | no | `.pbtxt,.textproto,.txtpb` |
| `thrift` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.thrift` |
| `tmux` | not applicable: no external scanner | IV-recovery? | 0/1 | no | no | no | no | `.tmux,tmux.conf` |
| `todotxt` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.txt` |
| `turtle` | not applicable: no external scanner | IV-unknown | 21/40 | no | no | no | no | `.ttl` |
| `twig` | not applicable: no external scanner | CLEAN | 40/40 | no | no | no | no | `.twig` |
| `v` | not applicable: no external scanner | IV-recovery? | 5/40 | no | no | no | no | `.v,.vsh` |
| `verilog` | not applicable: no external scanner | IV-recovery? | unmeasured | no | no | no | no | `.sv,.svh,.v` |
| `vimdoc` | not applicable: no external scanner | IV-recovery? | unmeasured | no | no | no | no | `.txt` |
| `wat` | not applicable: no external scanner | IV-recovery? | 0/34 | no | no | no | no | `.wast,.wat` |
| `zig` | not applicable: no external scanner | IV-unknown | 39/40 | no | no | no | no | `.zig` |
