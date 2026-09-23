//go:build (!grammar_subset || grammar_subset_caddy) && gotreesitter_no_copyleft

package grammarruntime

// RegisterCaddySupport is a no-op under gotreesitter_no_copyleft: it keeps
// the symbol builtin_scanners_gen.go's unconditional call site needs, but
// drops caddy_scanner.go (excluded by this same build tag; see its header)
// and never registers an external scanner for caddy. Loading the caddy
// grammar under this build still decodes the grammar blob, but every
// _newline/_indent/_dedent external token then goes unhandled, so caddy
// parses degrade instead of erroring. Combined with
// copyleft_language_set_no_copyleft.go (which drops caddy from the public
// registry entirely), that degraded path is reachable only by calling
// grammars.CaddyLanguage() (or the standalone grammars/caddy package)
// directly. See docs/licensing.md.
func RegisterCaddySupport() {}
