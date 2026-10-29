# gotreesitter command

A command-line tool for the gotreesitter parser. It is pure Go, so it needs no C compiler.

## Install

```
go install github.com/odvcencio/gotreesitter/cmd/gotreesitter@latest
```

This works from the first release that contains `cmd/gotreesitter`. The binary is about 34 MB because it embeds all grammars. Build with `-tags 'grammar_subset grammar_subset_go'` (or another grammar tag set) to embed fewer grammars and shrink it; `binsize_test.go` guards this.

## Commands

```
gotreesitter parse main.go                      # syntax tree as an S-expression
gotreesitter parse -json config.json            # same, as JSON
gotreesitter query -q '(function_declaration name: (identifier) @name)' main.go
gotreesitter highlight main.go                  # byte range, capture, text
gotreesitter tags main.go                       # symbol definitions and references
gotreesitter outline main.go                    # nested symbol outline
gotreesitter grep 'fmt.Println($X)' main.go     # match a code pattern
gotreesitter grep -r 'log.Print($X)' 'fmt.Println($X)' main.go   # rewrite (add -w to write)
gotreesitter test --root ~/gts-corpora/grammar_parity --langs json,go
gotreesitter languages
```

- The language comes from the file name or shebang. Pass `-l <name>` to choose one. Use `-` as the file to read standard input (then `-l` is required).
- Put flags before the file name.
- Positions are 1-based `line:column`.
- `tags` and `outline` need a tags query. About 84 of the 206 languages have one. Other languages exit with code 2 and say so.
- `test` runs upstream tree-sitter corpus files and compares each tree. It replaces the old `corpuscheck` command. It exits 2 if no language ran or a language named in `--langs` was skipped.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Syntax errors found (`parse`) or corpus cases failed (`test`) |
| 2 | Usage or input error (unknown command, unknown language, bad query, unreadable file) |
