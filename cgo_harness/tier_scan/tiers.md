# Grammar tiers — unreleased

Generated 2026-07-09T05:26:24Z at `5d21c536`. Parity vs the
tree-sitter C oracle is the hard gate; performance is the sub-rank
(rules in `cgo_harness/tier_scan/README.md`).

| tier | count |
| --- | --- |
| I | 14 |
| II | 23 |
| III | 4 |
| IV | 165 |

## Tier I — parity-clean, fast (14)

`astro`, `gitcommit`, `javascript`, `llvm`, `lua`, `nickel`, `pkl`, `prisma`, `puppet`, `r`, `squirrel`, `thrift`, `tsx`, `yaml`

## Tier II — parity-clean, ok (23)

`arduino`, `beancount`, `cmake`, `devicetree`, `editorconfig`, `foam`, `fortran`, `git_config`, `git_rebase`, `gitignore`, `gn`, `janet`, `json`, `json5`, `pem`, `ql`, `ron`, `ruby`, `sparql`, `tablegen`, `todotxt`, `twig`, `vue`

## Tier III — parity-clean, poor perf (4)

`cobol`, `embedded_template`, `gomod`, `regex`

## Tier IV — not parity-clean (165)

| grammar | cause | parity |
| --- | --- | --- |
| `ada` | IV-unknown | 39/40 |
| `agda` | IV-scanner | 2/40 |
| `angular` | IV-recovery? | 22/40 |
| `apex` | IV-unknown | 39/40 |
| `asm` | IV-recovery | 0/40 |
| `authzed` | IV-unknown | 36/40 |
| `awk` | IV-unknown | 25/29 |
| `bash` | IV-recovery? | unmeasured |
| `bass` | IV-unknown | 39/40 |
| `bibtex` | IV-unknown | 38/40 |
| `bicep` | IV-unknown | 34/40 |
| `bitbake` | IV-unknown | 35/40 |
| `blade` | IV-recovery? | unmeasured |
| `brightscript` | IV-recovery? | 11/40 |
| `c` | IV-recovery | 22/40 |
| `c_sharp` | IV-recovery | unmeasured |
| `caddy` | IV-recovery? | 11/40 |
| `cairo` | IV-recovery? | 1/40 |
| `capnp` | IV-unknown | 14/21 |
| `chatito` | IV-unknown | 1/5 |
| `circom` | IV-shape? | 19/40 |
| `clojure` | IV-perf | unmeasured |
| `comment` | IV-perf | unmeasured |
| `commonlisp` | IV-recovery? | unmeasured |
| `cooklang` | IV-recovery | 0/3 |
| `corn` | IV-unknown | 22/23 |
| `cpon` | IV-unknown | 9/10 |
| `cpp` | IV-recovery | 9/40 |
| `crystal` | IV-shape? | unmeasured |
| `css` | IV-unknown | 37/40 |
| `csv` | IV-perf | unmeasured |
| `cuda` | IV-recovery? | 21/40 |
| `cue` | IV-unknown | 39/40 |
| `cylc` | IV-recovery? | unmeasured |
| `d` | IV-recovery? | unmeasured |
| `dart` | IV-recovery? | 0/40 |
| `desktop` | IV-perf | unmeasured |
| `dhall` | IV-unknown | 15/40 |
| `diff` | IV-perf | unmeasured |
| `disassembly` | IV-version | 0/40 |
| `djot` | IV-scanner? | unmeasured |
| `dockerfile` | IV-recovery? | unmeasured |
| `dot` | IV-perf | unmeasured |
| `doxygen` | IV-unknown | 20/40 |
| `dtd` | IV-unknown | 0/3 |
| `earthfile` | IV-recovery? | 0/40 |
| `ebnf` | IV-recovery? | 0/40 |
| `eds` | IV-unknown | 0/1 |
| `eex` | IV-unknown | 21/40 |
| `elisp` | IV-perf | unmeasured |
| `elixir` | IV-recovery | unmeasured |
| `elm` | IV-unknown | 0/8 |
| `elsa` | IV-recovery? | 0/0 |
| `enforce` | IV-unknown | 28/40 |
| `erlang` | IV-recovery | unmeasured |
| `facility` | IV-unknown | 1/4 |
| `faust` | IV-unknown | 39/40 |
| `fennel` | IV-recovery? | 15/40 |
| `fidl` | IV-unknown | 37/40 |
| `firrtl` | IV-recovery? | 12/27 |
| `fish` | IV-perf | unmeasured |
| `forth` | IV-perf | unmeasured |
| `fsharp` | IV-perf | unmeasured |
| `gdscript` | IV-unknown | 39/40 |
| `gitattributes` | IV-unknown | 9/10 |
| `gleam` | IV-unknown | 39/40 |
| `glsl` | IV-recovery | 7/40 |
| `go` | IV-unknown | 25/40 |
| `godot_resource` | IV-perf | unmeasured |
| `graphql` | IV-unknown | 0/1 |
| `groovy` | IV-recovery | 6/40 |
| `hack` | IV-unknown | 38/40 |
| `hare` | IV-recovery | 15/40 |
| `haskell` | IV-scanner | unmeasured |
| `haxe` | IV-recovery | 10/40 |
| `hcl` | IV-perf | unmeasured |
| `heex` | IV-unknown | 6/7 |
| `hlsl` | IV-unknown | 35/40 |
| `html` | IV-recovery | 0/40 |
| `http` | IV-unknown | 10/11 |
| `hurl` | IV-recovery | 14/40 |
| `hyprlang` | IV-unknown | 1/2 |
| `ini` | IV-unknown | 9/11 |
| `java` | IV-unknown | 22/40 |
| `jinja2` | IV-recovery | 3/40 |
| `jq` | IV-unknown | 7/8 |
| `jsdoc` | IV-unknown | 13/40 |
| `jsonnet` | IV-recovery? | 39/40 |
| `julia` | IV-recovery | unmeasured |
| `just` | IV-unknown | 2/8 |
| `kconfig` | IV-recovery? | 18/40 |
| `kdl` | IV-recovery | 11/40 |
| `kotlin` | IV-unknown | 22/40 |
| `ledger` | IV-unknown | 2/4 |
| `less` | IV-recovery? | 10/40 |
| `linkerscript` | IV-recovery | 1/40 |
| `liquid` | IV-recovery? | 11/36 |
| `luau` | IV-perf | unmeasured |
| `make` | IV-recovery? | unmeasured |
| `markdown` | IV-unknown | 0/40 |
| `markdown_inline` | IV-perf | unmeasured |
| `matlab` | IV-recovery? | 4/40 |
| `mermaid` | IV-recovery? | 0/40 |
| `meson` | IV-recovery? | 2/40 |
| `mojo` | IV-recovery? | 29/40 |
| `move` | IV-recovery? | 14/40 |
| `nginx` | IV-unknown | 0/1 |
| `nim` | IV-recovery? | unmeasured |
| `ninja` | IV-unknown | 3/5 |
| `nix` | IV-perf | unmeasured |
| `norg` | IV-scanner | 0/2 |
| `nushell` | IV-recovery? | 7/40 |
| `objc` | IV-recovery | unmeasured |
| `ocaml` | IV-unknown | 12/40 |
| `odin` | IV-recovery? | 1/40 |
| `org` | IV-recovery? | 1/6 |
| `pascal` | IV-recovery? | 0/40 |
| `perl` | IV-recovery? | unmeasured |
| `php` | IV-unknown | 36/40 |
| `powershell` | IV-recovery? | 22/40 |
| `prolog` | IV-recovery? | 4/40 |
| `promql` | IV-unknown | 0/4 |
| `properties` | IV-perf | unmeasured |
| `proto` | IV-recovery? | 24/40 |
| `pug` | IV-recovery? | 0/40 |
| `purescript` | IV-recovery? | 1/40 |
| `python` | IV-unknown | 6/40 |
| `racket` | IV-unknown | 2/40 |
| `rego` | IV-recovery? | 7/40 |
| `requirements` | IV-unknown | 8/9 |
| `rescript` | IV-recovery? | 24/40 |
| `robot` | IV-recovery? | 28/40 |
| `rst` | IV-perf | unmeasured |
| `rust` | IV-recovery? | 11/40 |
| `scala` | IV-recovery? | 5/40 |
| `scheme` | IV-perf | 23/40 |
| `scss` | IV-recovery? | 6/40 |
| `smithy` | IV-unknown | 34/40 |
| `solidity` | IV-unknown | 6/40 |
| `sql` | IV-recovery? | 8/40 |
| `ssh_config` | IV-unknown | 1/2 |
| `starlark` | IV-unknown | 34/40 |
| `svelte` | IV-unknown | 8/40 |
| `swift` | IV-recovery? | 0/40 |
| `tcl` | IV-recovery? | unmeasured |
| `teal` | IV-recovery? | 8/40 |
| `templ` | IV-recovery? | 31/40 |
| `textproto` | IV-perf | unmeasured |
| `tlaplus` | IV-perf | unmeasured |
| `tmux` | IV-recovery? | 0/1 |
| `toml` | IV-unknown | 8/11 |
| `turtle` | IV-unknown | 21/40 |
| `typescript` | IV-perf | unmeasured |
| `typst` | IV-recovery? | unmeasured |
| `uxntal` | IV-recovery? | 0/40 |
| `v` | IV-recovery? | 5/40 |
| `verilog` | IV-recovery? | unmeasured |
| `vhdl` | IV-recovery? | unmeasured |
| `vimdoc` | IV-recovery? | unmeasured |
| `wat` | IV-recovery? | 0/34 |
| `wgsl` | IV-recovery | 21/40 |
| `wolfram` | IV-unknown | 0/11 |
| `xml` | IV-unknown | 1/40 |
| `yuck` | IV-unknown | 1/2 |
| `zig` | IV-unknown | 39/40 |
