# R7 cliff fixtures

These files pin the seven inputs from the gotreesitter v1 design, Workstream R7.
The detector decompresses each archive and verifies the source SHA-256 before parsing.
The archives use gzip with no timestamp. The listed digests name the decompressed bytes.

| File | Source and revision | License | SHA-256 |
| --- | --- | --- | --- |
| `c_sharp_generated_32k.cs.gz` | `cmd/issue454bench` generator at `c3ae5dc09bb5990120f473c66e29b06f7125ea3a`; run `c_sharp 32 emit` | MIT, gotreesitter | `b712fa3408c0d0c7d8ebd1c7919b47fe4a68281c0e61f5a7619a43004d6cdca9` |
| `elixir_generated_32k.ex.gz` | The same generator; run `elixir 32 emit` | MIT, gotreesitter | `a8178ccc477384b9d71b6ab9df31fb12e306282abde68b7db1b5b32b494f76d8` |
| `python_generated_32k.py.gz` | The same generator; run `python 32 emit` | MIT, gotreesitter | `685ae2d152c1585e22fb875780b4831a5df6e162a0e5fdf55a3cd2dc7608d4f9` |
| `go_parser.go.gz` | [golang/go `src/go/parser/parser.go`](https://github.com/golang/go/blob/go1.25.1/src/go/parser/parser.go), tag `go1.25.1` | BSD-3-Clause | `ad0a0a8fce883ab86ec3d2bf1d8d8dcc1072534bff46ab26c0796795f7b3087a` |
| `php_run-tests.php.gz` | [php/php-src `run-tests.php`](https://github.com/php/php-src/blob/d26068059e83fe40de3430a512471d194119bee0/run-tests.php), tag `php-8.3.0` | PHP-3.01 | `f19d07587177c12822d604717f2474b91b57a7d7d3a4ca97227e824bc32a6084` |
| `markdown_volumes.md.gz` | [kubernetes/website `volumes.md`](https://github.com/kubernetes/website/blob/a746bbd76e9504ecdedafbda7f7abfdbdee5642e/content/en/docs/concepts/storage/volumes.md), commit `a746bbd76e9504ecdedafbda7f7abfdbdee5642e` | CC-BY-4.0, The Kubernetes Authors | `12f2cd6d66512b683ecd00d0b32cff41b70709068f691c063043e342c88ce193` |
| `html_go_mem.html.gz` | [golang/go `doc/go_mem.html`](https://github.com/golang/go/blob/go1.25.1/doc/go_mem.html), tag `go1.25.1` | BSD-3-Clause | `561b50bca58e07455e993d68cf49281ed80df2150d09a38c77c906eabee36b7a` |

The design names `golang/website` as the source for `go_mem.html`.
That file is absent from `golang/website` at commit `f2661d967b28530da480f0a1da9a4279026d34ca`.
The file is present in `golang/go` at tag `go1.25.1`, so this fixture uses that source.

The generated fixtures reach or exceed 32 KiB because the generator writes complete records.
The copied files have no changes. Read the upstream [Go license](https://github.com/golang/go/blob/go1.25.1/LICENSE), [PHP license](https://github.com/php/php-src/blob/d26068059e83fe40de3430a512471d194119bee0/LICENSE), and [Kubernetes license](https://github.com/kubernetes/website/blob/a746bbd76e9504ecdedafbda7f7abfdbdee5642e/LICENSE).
