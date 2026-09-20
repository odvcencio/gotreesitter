//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// TestPowerShellCommandNameImmediateSingleQuoteMatchesCReference is the
// C-oracle regression twin for the languages.lock bump from da65ba3acc93 to
// e7bd348c49fd (airbus-cert/tree-sitter-powershell@1ffcd63, "Handle simple
// quote handler"). It compares gotreesitter's parse of a command name that
// immediately continues into a single-quoted run against a native C parser
// built from the pinned tree-sitter-powershell source, node-by-node.
//
// See TestPowerShellCommandNameImmediateSingleQuoteParsesCleanly (root
// package) for the host-side pin of the same construct.
func TestPowerShellCommandNameImmediateSingleQuoteMatchesCReference(t *testing.T) {
	src := []byte("foo'bar' baz\n")
	runParityCase(t, parityCase{name: "powershell", source: string(src)}, "command-name-immediate-quote", src)
}
