//go:build (!grammar_subset || grammar_subset_disassembly) && gotreesitter_no_copyleft

package grammarruntime

// RegisterDisassemblySupport is a no-op under gotreesitter_no_copyleft: it
// keeps the symbol builtin_scanners_gen.go's unconditional call site needs,
// but drops disassembly_scanner.go (excluded by this same build tag; see its
// header) and never registers an external scanner for disassembly. See
// caddy_no_copyleft_stub.go for the shared rationale and
// docs/licensing.md for the full picture.
func RegisterDisassemblySupport() {}
