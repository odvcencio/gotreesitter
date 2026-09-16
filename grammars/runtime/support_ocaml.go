//go:build !grammar_subset || grammar_subset_ocaml

package grammarruntime

// RegisterOcamlSupport registers the scanner support for ocaml.
func RegisterOcamlSupport() {
	RegisterExternalScanner("ocaml", OcamlExternalScanner{})
}
