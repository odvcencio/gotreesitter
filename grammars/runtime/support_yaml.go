//go:build !grammar_subset || grammar_subset_yaml

package grammarruntime

// RegisterYamlSupport registers the scanner support for yaml.
// yaml_external_lex_states_gen.go registers the lex states in its own init,
// which is the shape cmd/ts2go emits for every other grammar.
func RegisterYamlSupport() {
	RegisterExternalScanner("yaml", YamlExternalScanner{})
}
